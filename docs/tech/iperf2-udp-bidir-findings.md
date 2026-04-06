# Code Review Finding Report

## Summary

During live iperf2 UDP bidirectional testing (Mac ↔ Windows and Mac ↔ public Linux), five distinct issues were identified and verified with bare iperf2 commands: (1) a race condition in `udp_accept` causing parallel UDP streams started with `-P` to drop one stream's server report on Windows; (2) the UDP server report ACK not being delivered back to the client when the client is behind standard NAT; (3) graceful `taskkill` (without `/F`) unreliably leaves orphaned iperf processes on Windows — resolved by always using `/F` with schtasks one-shot mode; (4) port collision when running simultaneous bidirectional tests on the same port produces identical, incorrect loss figures in both directions; (5) persistent Windows UDP servers lose ACK ability after the first client due to Windows `WSAECONNRESET` (ICMP Port Unreachable) poisoning the server socket — iperf2 lacks the `SIO_UDP_CONNRESET` fix that Go, Rust, and other platforms apply. All findings were confirmed with direct iperf2 invocations (no wrapper code) during sessions 2026-03-12 and 2026-03-21.

---

## Context

- **Project:** `iperf-tool` — a Go CLI + GUI wrapper around iperf2
- **Environment:**
  - macOS client (public NAT: 185.126.130.226, VPN: 100.80.223.29)
  - Windows remote (100.89.230.34), iperf2 2.2.1
  - Linux public server eu6 (81.7.17.12, Ubuntu 22.04), iperf2 2.1.5 — used to isolate NAT behavior
- **Protocol:** UDP, bidirectional, 2 streams per direction
- **Language:** Go (implemented in `internal/iperf/`); shell commands run via `ssh.Client.RunCommand()`
- **Branch:** `multistream-iperf2`

---

## Recommended Approach: UDP Probe First, Then Test

Since the wrapper controls both sides of the connection via SSH, it can determine upfront whether inbound UDP is reachable before starting the actual measurement. This avoids running a full test only to discover at the end that the Server Report is fabricated.

### Pre-flight UDP Reachability Probe

Before starting any reverse or bidirectional test, the wrapper performs a short UDP echo probe:

```
Probe sequence (wrapper-implemented, not iperf2):
  1. Client binds a UDP socket on an ephemeral port and starts listening
  2. Via SSH, instruct the remote to send a single UDP packet to client-ip:port
  3. Wait up to 2s for the packet to arrive
  4. If received  → inbound UDP is open → use direct mode (no SSH fallback needed)
  5. If not received → NAT is blocking inbound UDP → use SSH fallback mode for the whole test
```

This probe is topology-agnostic — it works correctly on LAN (both sides private IPs, no NAT), public IPs, VPNs, and NAT deployments. The private/public IP of either side is irrelevant; only actual reachability matters.

**Why not use the `WARNING` at test end as the detection signal?**
- The WARNING appears only after the full test duration — wasting the entire measurement if fallback is needed
- On interrupted tests (SIGTERM), the WARNING is suppressed even though the ACK was not received
- The fabricated Server Report printed before the WARNING is easily mistaken for real data

### Test Execution Flow

```
Pre-flight:
  probe → inbound UDP reachable?
          │
          ├─ YES → Direct mode:
          │          Start remote server (no -o)
          │          Run test
          │          Parse Server Report from client stdout
          │
          └─ NO  → SSH fallback mode:
                     Start remote server with -o <file>
                     Run test
                     Graceful kill server, wait ≥500ms
                     SSH-read output file
                     Parse Server Report from file content
```

### Applicability by Direction

| Direction | Probe needed | Why |
|---|---|---|
| Forward only (client→remote) | No | Server Report ACK travels remote→client, same path as data; if data arrives, ACK likely will too |
| Reverse (remote→client) | Yes | ACK must travel client→remote against data direction; NAT on remote side blocks it |
| Bidirectional | Yes | Both directions have the ACK problem on one side |

**Note on forward direction:** even in forward-only tests the ACK can fail under NAT (confirmed with eu6). The probe can optionally be run for forward tests too, but the impact is lower since the fabricated Server Report in the forward case reflects the client's own send-side data which is partially accurate.

---

## Detailed Findings

### Finding 1 — `udp_accept` Race Condition with `-P` on Windows

**Observed behavior:** When the client used `-P 2` against a Windows iperf2 server on a single port, one of the two streams consistently failed to receive a Server Report ACK. The other stream received its report normally.

**Confirmed invocation (triggers bug):**
```
# Server (single port)
iperf.exe -s -u -p 5201

# Client
iperf -c <host> -u -p 5201 -P 2 -t 5 -b 10m -f m
```

**Observed output:**
```
[  1] Server Report: ... 5.5% loss
[  3] WARNING: did not receive ack of last datagram after 10 tries.
```
Stream `[1]` received its report; stream `[3]` (second server-side handler) got `Connection reset by peer` and failed.

**Confirmed workaround — port range on both server and client:**
```
# Server (port range, one listener per port)
iperf.exe -s -u -p 5201-5202

# Client (matching range + -P)
iperf -c <host> -u -p 5201-5202 -P 2 -t 5 -b 10m -f m
```
Both streams connected (`[1]` port 5201, `[2]` port 5202) and both received Server Reports. The port range must be specified on **both** server and client — server-only or client-only range is insufficient.

**Fact vs. assumption:**
- *Fact:* `-P 2` on a single port drops one stream's Server Report ACK on Windows 2.2.1; reproduced consistently.
- *Fact:* Port-range workaround with matching range on both sides produced correct per-stream reports for both streams.
- *Inference:* Root cause is the `udp_accept` Windows implementation calling `recvfrom` non-atomically across threads; race is deterministic with ≥2 UDP streams on the same port.

---

### Finding 2 — UDP Server Report ACK Not Delivered to Client (NAT-dependent)

**Observed behavior:** When the client is behind standard NAT, the client consistently receives `"did not receive ack of last datagram"` after a UDP test. The server computes and sends the report, but the ACK never arrives at the client.

**Confirmed with public server (client behind NAT):**
```
# Remote server
iperf -s -u -p 5201

# Client (behind NAT)
iperf -c <server-ip> -u -p 5201 -t 5 -b 10m -f m
# Result: WARNING: did not receive ack of last datagram after 10 tries.
```

**Confirmed NOT present on directly routable networks:**
```
iperf -c <peer-ip> -u -p 5201 -t 5 -b 10m -f m
# Result: Server Report received cleanly, no warning
```

**Root cause:** The server sends its report UDP packet back to the client's ephemeral source port. When the client is behind standard NAT, that NAT mapping expires or is directionally filtered after the data flow ends, making the return path unreachable. On directly routable networks (mesh VPNs, same LAN, public IPs on both sides) the return path stays open and the ACK arrives.

**Fact vs. assumption:**
- *Fact:* ACK lost when client is behind standard NAT; confirmed by `WARNING` in client output vs. valid stats in server `-o` file.
- *Fact:* When the ACK fails, iperf2 prints a **fabricated Server Report** synthesized from client-side send data — it always shows `0.000 ms` jitter and `0%` loss regardless of actual network conditions. This is not a real measurement.
- *Fact:* The `WARNING: ack of last datagram failed` appears at the very end of client output, after the fabricated Server Report. The runner must parse the complete output before deciding whether the Server Report is valid.
- *Fact:* ACK delivered successfully when both peers have directly routable addresses; confirmed by client receiving Server Report with non-zero jitter matching server-side measurements.
- *Fact:* The failure is on the client's NAT, not the server's — confirmed by testing against a public server with no NAT on its side.
- *Inference:* Any deployment where the client is behind NAT (home router, corporate firewall, cloud NAT gateway) may require the SSH-file fallback. Directly routable deployments do not.

---

### Finding 3 — `taskkill /F` Required for Reliable Process Cleanup

**Original observation (session 2026-03-12):** `taskkill /F` prevented the `-o` output file from being flushed. Graceful kill (`taskkill /IM` without `/F`) allowed flush but was itself unreliable — sometimes failing with `"could not be terminated — use /F option"`.

**Updated observation (session 2026-03-21):** Graceful kill without `/F` is unreliable on Windows — it frequently leaves orphaned iperf processes that pile up across test runs, causing port conflicts and stale socket state. Confirmed by observing 4 orphaned `iperf.exe` instances after a 5-run test loop.

**Resolution:** `taskkill /F` is now always used. This is safe because the runner uses `schtasks` one-shot mode — each test run creates a fresh iperf server process that typically exits on its own after the client disconnects. The `/F` kill runs as cleanup to catch any stragglers. In SSH fallback mode (when `-o` file is needed), the server writes output during the test, not at exit — the file is populated before the kill command runs.

**Fact:** Orphaned processes accumulated without `/F`, causing port conflicts. With `/F`, 5/5 runs completed cleanly with zero orphaned processes (2026-03-21).

---

### Finding 5 — Persistent Windows UDP Server Loses ACK Ability After First Client (WSAECONNRESET)

**Observed behavior (session 2026-03-21):** A persistent iperf2 server on Windows (`-D` daemon mode) delivers Server Report ACKs to the first client, but all subsequent clients receive `"WARNING: did not receive ack of last datagram after 10 tries"`. The server-side output shows `"recvfrom failed: Connection reset by peer"` after each client disconnects.

**Confirmed across all parameter combinations:**
- `-P 4 -b 1M`, `-P 2 -b 2M`, `-P 1 -b 4M`, `-P 4 -b 500K`
- 12/12 runs after the first client: 0 Server Reports, all WARNINGs
- Independent of stream count, bandwidth, and port range

**Root cause — Windows `WSAECONNRESET` (error 10054) on UDP sockets:**
1. Client #1 finishes test and closes its ephemeral UDP port
2. Server sends Server Report ACK to that port
3. Client's OS responds with ICMP "Port Unreachable"
4. **Windows delivers the ICMP error as `WSAECONNRESET` on the server's UDP socket** — this is a documented Windows platform behavior (Microsoft KB 263823) that does not occur on Linux/macOS, where asynchronous ICMP replies on UDP sockets are silently ignored
5. On the next `recvfrom()` call (for client #2), the stale `WSAECONNRESET` is returned instead of data
6. iperf2's `FATALUDPREADERR` macro treats any error except `WSAEWOULDBLOCK` as fatal — the socket is permanently poisoned

**Known platform fix:** `WSAIoctl(sock, SIO_UDP_CONNRESET, &FALSE, ...)` disables ICMP error reporting on Windows UDP sockets. Go, Rust (tokio), pjsip, and Dart/Flutter all apply this fix. iperf2 does not — it has no `SIO_UDP_CONNRESET` handling anywhere in its source.

**Resolution:** The runner uses `schtasks` one-shot mode — each test run creates a fresh iperf server process with a clean socket. The server exits after the client disconnects, so the poisoned socket is never reused. This avoids the Windows WSAECONNRESET bug without patching iperf2.

**Fact vs. assumption:**
- *Fact:* Persistent `-D` daemon fails ACK delivery for all clients after the first — confirmed with 12 consecutive runs across 4 parameter sets (2026-03-21).
- *Fact:* One-shot `schtasks` mode (fresh server per test) delivers ACKs reliably — confirmed with 5/5 runs, zero failures (2026-03-21).
- *Fact:* Root cause is Windows `WSAECONNRESET` on UDP sockets (Microsoft KB 263823), not iperf2 application logic.
- *Fact:* iperf2 source has no `SIO_UDP_CONNRESET` handling.

---

### Finding 4 — Identical Measurements in Both Directions (Port Collision)

**Observed behavior:** When running simultaneous bidirectional UDP tests with both directions using the same port (e.g., 5201), both directions reported identical loss percentages, indicating cross-contamination of measurement data.

**Root cause:** Both the reverse client traffic and the forward server's ACK/report packets share the same local port, mixing stream identities.

**Resolution (confirmed working):** Using non-overlapping port ranges per direction eliminates the collision:
- Forward server (client→remote): ports N..N+1 (e.g., 5201-5202)
- Reverse server (remote→client): ports N+2..N+3 (e.g., 5203-5204)

Confirmed with full simultaneous bidir test — forward and reverse reported distinct loss figures, consistent with asymmetric link conditions.

---

## Interrupt / Partial Measurement Behavior

**Scenario tested:** 50s UDP test with 5s intervals, client interrupted at ~15s via SIGTERM (`timeout`).

**Client output on interrupt:**
- Complete interval lines printed normally up to the last full interval
- A **partial summary line** emitted covering total elapsed time: `0.00-14.99 sec`
- A **fabricated Server Report** is printed (e.g. `0.000 ms` jitter) — same as the NAT failure case
- No `WARNING: ack of last datagram failed` printed — SIGTERM kills the process before the ACK retry loop completes and prints the warning

**Server output on interrupt (SSH fallback, `-o` file):**
- Complete interval lines up to the last full interval
- A **partial final interval** covering exactly the remaining elapsed time
- A **full summary line** covering total elapsed

**Facts:**
- The absence of `WARNING` on interrupt does **not** mean the ACK was received — the process was killed before the retry loop could print it
- The Server Report shown in client output on interrupt is **fabricated** (client-side data only), identical to the NAT failure case — `0.000 ms` jitter is the giveaway
- The server `-o` file **is written** on graceful server kill even when the client was interrupted — the server flushes its stats for however long data actually flowed
- Partial interval is labeled with its actual elapsed duration, not rounded or dropped
- Exit code from SIGTERM interrupt is 124 (killed by `timeout`), not 0

**Implication for the runner implementation:**
- Do not use absence of `WARNING` as proof the ACK was received — it is unreliable on interrupt
- The only reliable signal for a valid Server Report in client output is: summary line present **and** no `WARNING` **and** process exited cleanly (exit code 0)
- The SSH fallback (server `-o` file) is the authoritative source for server-side stats in all interrupted and NAT cases
- The server file contains valid partial results for interrupted tests — `RunBidir()` can interrupt at any point and retrieve real stats via SSH

---

## Maximum Observable Metrics (2 streams, bidir, `-e` enhanced mode)

From a full bidirectional 2-stream test (`-P 2`, `-e`, `-i 1`, port-range workaround applied), the following metrics are available **per stream, per 1-second interval**, from each of the 4 perspectives (forward client, forward server, reverse client, reverse server):

| Metric | Source | Notes |
|---|---|---|
| Throughput (Mbits/sec) | client + server | client = sent; server = received |
| Loss (lost/total, %) | server-side report | authoritative; client-side report unreliable under NAT |
| Jitter (ms) | server-side report | running inter-packet delay variation |
| Latency avg/min/max/stdev (ms) | server `-e` | one-way; invalid if clocks not synced (negatives = unsynchronized clocks) |
| PPS (packets per second) | server `-e` | received PPS |
| Out-of-order datagrams | server `-e` | per interval |
| NetPwr | server `-e` | throughput/latency ratio; unreliable with unsynchronized clocks |
| Write/Err/Timeo | client `-e` | send-side errors and timeouts |
| Rx/inP | Linux server `-e` only | receive buffer depth; not present in Windows iperf2 output |
| SUM line | both | aggregate across all streams per interval |

**Observed in session (10s, 2×10 Mbits/sec each direction, ~65ms RTT):**

| Direction | Sent | Received BW | Loss | Jitter |
|---|---|---|---|---|
| Client → Remote | 20.0 Mbits/sec (2×10) | ~8.9 Mbits/sec | ~40% | ~1.2 ms |
| Remote → Client | 19.9 Mbits/sec (2×10) | ~5.7 Mbits/sec | ~67% | ~6.4 ms |

High loss is consistent with 40 Mbits/sec total offered load over a congested ~65ms tunnel.

---

## Root Cause Analysis

| Finding | Root Cause |
|---|---|
| 1 — `-P` race | iperf2 Windows `udp_accept` calls `recvfrom` non-atomically across threads; race is deterministic with ≥2 UDP streams on a single port |
| 2 — ACK loss | Client-side NAT mapping expires after UDP data flow ends; server ACK is sent to an unreachable ephemeral port. Not present on directly routable networks |
| 3 — Process cleanup | Graceful kill (`taskkill` without `/F`) unreliable on Windows — leaves orphaned processes. `/F` is required; safe because schtasks one-shot mode restarts the server each run |
| 4 — Port collision | Forward and reverse traffic share the same port number, mixing stream identities and measurement data |
| 5 — Persistent server ACK loss | Windows `WSAECONNRESET` (ICMP Port Unreachable) poisons the server's UDP socket after first client disconnects; iperf2 lacks `SIO_UDP_CONNRESET` fix |

---

## Impact Assessment

| Finding | Severity | Impact |
|---|---|---|
| 1 — `-P` race | **High** | One stream silently drops its Server Report; multi-stream UDP loss/jitter data is incomplete or absent on Windows |
| 2 — ACK loss | **High (NAT) / None (direct)** | Client never receives server-side loss/jitter over NAT; SSH-file fallback required. No impact on directly routable networks |
| 3 — Process cleanup | **Medium** | Orphaned processes cause port conflicts and stale socket state across test runs; resolved by always using `taskkill /F` |
| 4 — Port collision | **High** | Both directions report identical (wrong) statistics; false measurements may be mistaken for real data |
| 5 — Persistent server ACK loss | **High (Windows)** | All UDP tests after the first fail to return server-side stats; affects any persistent/daemon server deployment on Windows |

---

## Proposed Resolutions

### Recommended Solution

**Workaround for Finding 1:** Replace `-P <n>` with a port range `-p <start>-<end>` where range size equals stream count. Both server and client must specify the same range. iperf2 spawns one listener per port, bypassing the `udp_accept` race entirely.

**Workaround for Finding 2 (implemented):** Use the pre-flight UDP probe described above to select mode before the test starts. `ProbeUDPReachability()` (`internal/iperf/probe.go`) binds a local UDP socket, instructs the remote via SSH to send a single packet back, and waits up to the configured timeout. The result selects direct mode or SSH fallback for the entire test run — no post-hoc WARNING detection required.

`ValidateServerReport()` (`internal/iperf/parser.go`) acts as a secondary safety net on the direct path: if the probe result was stale and the ACK failed, the fabricated Server Report (0.000 ms jitter, 0% loss) is detected and discarded rather than silently returned as valid data.

SSH fallback mode (started with `-o <file>`) is still used when:
- The probe times out (NAT blocking inbound UDP)
- The probe SSH command fails
- No local address is available for the probe

**Workaround for Finding 3:** Always use `taskkill /IM iperf.exe /F` to ensure reliable process cleanup. This is safe because schtasks one-shot mode creates a fresh server process per test run — the server typically exits on its own after the client disconnects, and `/F` catches any stragglers. In the SSH fallback path, wait `KillWaitMs` (default 500ms) before reading the `-o` file.

**Workaround for Finding 4:** Use non-overlapping port ranges per direction:
- Forward server: ports N..N+1
- Reverse server: ports N+2..N+3

This combined workaround is implemented in `Runner.RunBidir()` (`internal/iperf/runner.go`).

**Workaround for Finding 5:** Use schtasks one-shot mode (not `-D` daemon) to start a fresh iperf server process per test run. Each fresh process has a clean UDP socket that has never received a stale `WSAECONNRESET`. The proper fix would be adding `WSAIoctl(sock, SIO_UDP_CONNRESET, &FALSE, ...)` to iperf2's socket creation code, but that requires patching iperf2 itself.

### Alternative Solutions

| Alternative | Trade-off |
|---|---|
| Use `--no-udp-fin` | Suppresses server report entirely — no loss/jitter statistics. Unacceptable for measurement accuracy. |
| Use `-d` (dualtest) or `-r` (tradeoff) | Requires server to initiate reverse connection to client. Blocked by NAT on client side; not viable in most topologies. |
| Use iperf3 `--bidir` on Windows | iperf3 Windows UDP `--bidir` and `-P` officially unsupported; produce EAGAIN errors (confirmed via PR #1163). |
| Always use SSH-file fallback | Simpler code path, but adds unnecessary SSH round-trips on directly routable networks and requires pre-starting server with `-o`. |

---

## Verification Plan

1. **Finding 1 — Port range fix:** ✅ Implemented. `Config.PortRangeStr()` generates `-p 5201-5202` style ranges; server and client both receive matching ranges. Both streams receive Server Reports in live testing (2026-03-12).

2. **Finding 2 — Probe-first mode selection:** ✅ Implemented. `ProbeUDPReachability()` runs before every UDP reverse/bidir test. Confirmed on Tailscale VPN (macOS ↔ Windows, 100.80.223.29 ↔ 100.89.230.34): probe returns open, direct mode selected, Server Report parsed from client stdout with correct jitter/loss (2026-03-21). `ValidateServerReport()` confirmed to detect fabricated reports (0ms jitter heuristic) as secondary safety net.

3. **Finding 3 — Force kill:** ✅ Implemented. `remoteServerKillCmd()` uses `taskkill /IM iperf.exe /F` on Windows; safe because schtasks one-shot mode creates a fresh server per run. Confirmed 5/5 runs with zero orphaned processes (2026-03-21). SSH fallback path waits `KillWaitMs` (default 500ms) before reading file.

4. **Finding 4 — Port separation:** ✅ Implemented. `RunBidir()` uses `PortRangeStr(0)` for forward (ports N..N+P-1) and `PortRangeStr(Parallel)` for reverse (ports N+P..N+2P-1). Confirmed distinct forward/reverse loss figures in live testing.

5. **Finding 5 — One-shot server:** ✅ Implemented. `remoteServerStartCmd()` uses schtasks one-shot mode on Windows; `startRemoteServer()` kills and restarts before each test. Confirmed 5/5 runs with Server Report ACKs delivered vs 0/12 with persistent `-D` daemon (2026-03-21). Root cause documented: Windows `WSAECONNRESET` / missing `SIO_UDP_CONNRESET` in iperf2.

6. **Unit tests:** ✅ Complete. `internal/iperf/parser_test.go` covers `ParseOutput(isServer=true)` with real captured iperf2 enhanced output including `[SUM-2]` lines, `ValidateServerReport` with fabricated/valid/missing cases, and `MergeBidirResults`.

---

## Preventive Recommendations

1. **Validate port ranges in `Config.Validate()`:** Explicitly check `PortStart + NumStreams - 1 <= 65535` and `NumStreams >= 1`.

2. **Never expose `-P` flag for UDP tests:** The `iperf2.Config` struct deliberately omits `-P`, forcing the port-range pattern. Document this constraint in code comments.

3. **Pre-flight UDP probe before reverse/bidir tests:** Implement a UDP echo probe in the wrapper before starting any reverse or bidirectional test. Bind a local UDP socket, instruct the remote via SSH to send one packet back, wait up to 2s. Use the result to select direct mode or SSH fallback mode for the entire test upfront. Do not rely on `WARNING` detection at test end as the primary signal — it is too late, and is suppressed on interrupted tests. The WARNING check should remain as a secondary safety net in case the probe result was incorrect (e.g. NAT state changed between probe and test).

4. **Assert output file non-empty before parsing:** In the SSH fallback path, after reading the remote server output file, check the string is non-empty before calling `ParseOutput`. Return a clear error (not a silent zero-result) if empty; indicate whether graceful kill failed.

5. **Configurable kill wait time:** The 500ms post-kill wait should be a configurable constant rather than a magic number, to allow tuning on high-latency links.

6. **Code review gate:** Any change reading `-o` files without a preceding sleep must be flagged. The SSH fallback path must always wait `KillWaitMs` after kill before reading the output file.

---

## Open Questions / Missing Information

| Question | Why It Matters |
|---|---|
| What is the best remote command to send a single UDP packet back to the client for the pre-flight probe? | On Linux: `echo -n x \| nc -u -w1 <client-ip> <port>`. On Windows: requires PowerShell UDP socket code or a helper binary. The probe implementation differs per remote OS. |
| What timeout is appropriate for the UDP probe? | Too short (< RTT) gives false negatives; too long adds latency before every test. 2× RTT + 500ms margin is a reasonable starting point, but RTT must be known or estimated first. |
| ~~Does `taskkill /IM` (graceful) consistently flush the `-o` file?~~ | **Resolved (2026-03-21).** Graceful kill is unreliable — leaves orphaned processes. `/F` is always used now; safe because schtasks one-shot mode restarts the server each run. |
| ~~What causes the `taskkill /IM` failure on some process states?~~ | **Resolved (2026-03-21).** Root cause was Windows process state after SSH-launched schtask. Moot since `/F` is now always used. |
| ~~Is `IsWindows` always hardcoded?~~ | **Resolved.** `IsWindows` is auto-detected by the SSH client via `cmd.exe /c echo %OS%` check. |
| Negative latency values in Windows iperf2 server output (`-e`) | Clocks between peers are not synchronized. Latency values from server are unreliable for one-way delay measurement unless NTP or PTP sync is confirmed. |
| What happens if the remote iperf2 server crashes before writing the output file? | `RunCommand(cfg.remoteServerReadCmd())` returns empty string. Explicit error path with user-facing diagnostic is needed. |
