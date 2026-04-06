# Code Review Finding Report

## Summary

During live network testing of the `iperf-tool` CLI/GUI wrapper against a Windows remote host (rs4410, Tailscale) and a public Linux server (ipv6.dpdo.ru), **eight distinct bugs** were identified across the CLI runner, SSH server management, iperf2 output parser, and UI wiring layers. The most critical were: (1) the CLI never passed the SSH client to the iperf runner, making reverse/bidir tests always fail; (2) `remoteServerStartCmd()` hung indefinitely on Linux hosts; (3) the parser could not handle enhanced TCP server output.

---

## Context

- **Project:** `iperf-tool` — Go CLI + Fyne GUI wrapper around iperf2
- **Branch:** `multistream-iperf2`
- **Environment:**
  - macOS client (Tailscale VPN: 100.80.223.29, public NAT: behind SOCKS proxy)
  - Windows remote (100.89.230.34, iperf2 2.2.1) — Tailscale mesh, directly routable
  - Linux public server (81.7.17.12, iperf2 2.1.5) — behind SOCKS proxy from client
- **Test scope:** Forward TCP, Reverse TCP, Bidirectional TCP, Forward UDP, Parallel TCP, Repeat mode, Start/Stop server — across both hosts

---

## Detailed Findings

### Finding 1 — CLI runner passes `nil` SSH client to all iperf runner methods

**File:** `internal/cli/runner.go`, lines 158–163  
**File:** `main.go`, lines 131–136

`LocalTestRunner()` hardcoded `nil` for the `sshCli` parameter in all three dispatch calls (`RunForward`, `RunReverse`, `RunBidir`). The `runRemoteServer()` function in `main.go` established an SSH connection via `RemoteServerRunner` but never passed it to `LocalTestRunner`.

**Fact:** Reverse and bidirectional tests always failed with `"SSH client required"` when invoked via CLI with `--ssh`.

**Fix applied:** Added `SSHClient`, `IsWindows`, and `LocalAddr` fields to `RunnerConfig`. Wired `runner.Client()` into `cfg.SSHClient` in `runRemoteServer()` before calling `LocalTestRunner()`.

---

### Finding 2 — `remoteServerStartCmd()` hangs indefinitely on Linux

**File:** `internal/iperf/config.go`, line 311

The command template was:
```go
fmt.Sprintf("iperf %s &", strings.Join(args, " "))
```

The `&` backgrounds the process in the remote shell, but iperf's stdout remains connected to the SSH channel. `ssh.Session.CombinedOutput()` (called by `RunCommand`) blocks until the channel's stdout is closed — which never happens while iperf runs.

**Fact:** Directly observed — SSH command with `iperf ... &` hung for >30 seconds on ipv6.dpdo.ru (Linux). The same command with `nohup ... > /dev/null 2>&1 &` returned immediately and left the server listening.

**Note:** This did not affect Windows because `startWindows()` in `server.go` uses `schtasks`, which detaches the process differently. It also did not affect `server.go`'s Unix path, which uses `iperf -s -p <port> -D` (daemon flag).

**Fix applied:** Changed to `nohup iperf %s > /dev/null 2>&1 &`.

---

### Finding 3 — `schtasks` used nonexistent `C:\iperf2` directory path

**File:** `internal/ssh/server.go`, lines 95–98

The `startWindows()` method constructed a scheduled task with:
```
cmd.exe /c cd /d C:\iperf2 && iperf.exe -s -p %d
```

On the test host, iperf2 was installed at `C:\Users\Administrator\iperf.exe` and `C:\Program Files\iperf.exe` (on PATH), but `C:\iperf2` did not exist. The `cd /d C:\iperf2` failed with "The system cannot find the path specified", causing the server to never start. `isListening()` then correctly reported no LISTEN entry.

**Fact:** Observed in schtasks output: "The system cannot find the path specified" followed by `isListening` failure.

**Fix applied:** Removed `cd /d C:\iperf2 &&` prefix; `iperf.exe` is resolved via PATH.

---

### Finding 4 — Parser cannot handle enhanced TCP server output (`Reads=Dist` format)

**File:** `internal/iperf/parser.go`

Enhanced mode (`-e`) TCP server output uses a `Reads=Dist` histogram format:
```
[  1] 0.00-1.00 sec  2.51 MBytes  21.0 Mbits/sec  585=576:3:0:0:0:0:1:5
```

No regex matched this format. `reServerEnhanced` only matches UDP enhanced output (with jitter/ms/pps). `reServerInterval` requires jitter and loss columns. `reClientInterval` requires `$` at end-of-line which fails due to trailing histogram data.

**Fact:** Reverse TCP test returned `"parse local server output: no parseable iperf2 data found in output"` despite the buffer containing valid interval lines. Confirmed by dumping `localSrvOutput` which showed correct enhanced TCP server format with `Reads=Dist` columns.

**Fix applied:** Added `reServerEnhancedTCP` regex matching `\d+=` after bandwidth, and `parseServerEnhancedTCPMatch()` which extracts stream ID, interval, transfer, and bandwidth (histogram data is ignored).

---

### Finding 5 — `StopServer()` rejects calls when `m.running` is false

**File:** `internal/ssh/server.go`, lines 119–125

`StopServer()` checked `m.running` and returned an error if false. Since `ServerManager` is instantiated fresh on each CLI invocation, `m.running` is always false when `--stop-server` is used in a separate command from `--start-server`. The actual iperf processes are still running on the remote host.

**Fact:** `./iperf-cli --ssh <host> --stop-server` returned `"iperf2 server is not running"` while `netstat` confirmed ports 5201/5202 were LISTENING.

**Fix applied:** Removed the `m.running` guard. Added a nil-client guard instead (prevents panic from the existing test that calls `StopServer(nil)`).

---

### Finding 6 — Repeat mode ignored when `--ssh` is provided

**File:** `main.go`, lines 37–55

`runCLI()` dispatched to `runRemoteServer()` when `cfg.SSHHost != ""`, which ran a single test and returned. The `cfg.Repeat` check occurred before the SSH check, but only for the non-SSH path. `runRemoteServer()` had no repeat logic.

**Fact:** `./iperf-cli --ssh <host> -s <host> --repeat --repeat-count 3` ran exactly 1 iteration.

**Fix applied:** Added repeat dispatch inside `runRemoteServer()` after setting SSH client fields: `if cfg.Repeat { return runCLIRepeat(cfg) }`.

---

### Finding 7 — Bidir interval display falls back to single-direction format

**Files:** `ui/controls.go` lines 221–231, `internal/cli/runner.go` lines 150–157

The `onInterval` callback checked `fwd == nil` and returned early. In bidir mode, `RunBidir` replays intervals where `fwd` and `rev` arrays may differ in length — producing intervals where `fwd != nil && rev == nil` or `fwd == nil && rev != nil`. The former rendered as single-direction format (wrong columns); the latter was silently dropped.

**Fact:** Bidir test output showed 5 correct bidir lines followed by one line in single-direction format (`0.53 Mbps  0.12 MB  0 retransmits`).

**Fix applied:** Changed callback to: always use `FormatBidirInterval` when `cfg.Bidir` is true (passing nil for the missing side); derive timestamp from whichever side is non-nil.

---

### Finding 8 — `LocalAddr` not set for reverse/bidir CLI tests

**File:** `main.go`

`revClientCmd()` uses `cfg.LocalAddr` to construct `iperf.exe -c <LocalAddr>`. Without it, the remote client has no target address. Even with SSH wired (Finding 1), `LocalAddr` was empty because it was never populated from the SSH connection.

**Fact:** Inferred from the `revClientCmd()` code path — without `LocalAddr`, the generated command would be `iperf.exe -c  -p 5202 ...` (empty address).

**Fix applied:** In `runRemoteServer()`, after connecting SSH, set `cfg.LocalAddr = lap.LocalAddr()` using the `LocalAddrProvider` interface.

---

## Root Cause Analysis

| Finding | Root Cause |
|---------|-----------|
| 1 — nil SSH client | CLI `LocalTestRunner` was designed for local-only tests; SSH integration was added in `main.go` but never threaded through |
| 2 — SSH hang on Linux | `iperf ... &` backgrounds the process but does not close stdout; SSH channel waits for EOF that never arrives |
| 3 — `C:\iperf2` path | Hardcoded install path assumption; installer adds `iperf.exe` to PATH, not necessarily to `C:\iperf2` |
| 4 — TCP server parse | Parser was built for UDP enhanced output; TCP enhanced format (`Reads=Dist`) was not encountered during initial development |
| 5 — `StopServer` guard | State machine assumed same process lifetime for start and stop; CLI creates fresh `ServerManager` per invocation |
| 6 — Repeat with SSH | `runCLI` routing logic was added incrementally; SSH and repeat paths were mutually exclusive |
| 7 — Bidir interval | Callback assumed `fwd` is always non-nil; `RunBidir` can produce intervals with only one direction |
| 8 — Empty LocalAddr | New field added to `Config` but never populated in the CLI path |

---

## Impact Assessment

| Finding | Severity | Impact |
|---------|----------|--------|
| 1 — nil SSH client | **Critical** | Reverse and bidir tests always fail via CLI with SSH; core functionality broken |
| 2 — SSH hang on Linux | **Critical** | Any test against a Linux remote host hangs indefinitely; requires process kill to recover |
| 3 — `C:\iperf2` path | **High** | Server start fails on Windows hosts where iperf2 is not in `C:\iperf2`; install puts it on PATH |
| 4 — TCP server parse | **High** | All reverse/bidir TCP tests fail to parse local server output; returns error instead of results |
| 5 — `StopServer` guard | **Medium** | `--stop-server` CLI command is non-functional across separate invocations |
| 6 — Repeat with SSH | **Medium** | `--repeat` silently runs once when combined with `--ssh` |
| 7 — Bidir interval | **Low** | Display-only: last bidir interval line uses wrong column format |
| 8 — Empty LocalAddr | **High** | Remote client in reverse/bidir connects to empty address; test fails or connects to wrong host |

---

## Proposed Resolutions

### Recommended Solutions (all applied)

| Finding | Resolution |
|---------|-----------|
| 1 | Added `SSHClient`, `IsWindows`, `LocalAddr` fields to `RunnerConfig`; wired from `RemoteServerRunner` in `main.go` |
| 2 | Changed `remoteServerStartCmd()` to `nohup iperf %s > /dev/null 2>&1 &` |
| 3 | Removed `cd /d C:\iperf2 &&`; rely on PATH resolution |
| 4 | Added `reServerEnhancedTCP` regex and `parseServerEnhancedTCPMatch()` |
| 5 | Removed `m.running` guard from `StopServer()`; added nil-client guard |
| 6 | Added `if cfg.Repeat { return runCLIRepeat(cfg) }` inside `runRemoteServer()` after SSH setup |
| 7 | Changed interval callback to always use `FormatBidirInterval` when `cfg.Bidir` is true |
| 8 | Set `cfg.LocalAddr` from `LocalAddrProvider` after SSH connect in `main.go` |

### Alternative Solutions

- **Finding 2:** Could use `iperf -s -D` (daemon flag) instead of `nohup ... &`. Trade-off: `-D` doesn't accept all flags (e.g., `-f m`, `-i 1`) on all iperf2 versions, and the runner needs these flags for server-side output format control.
- **Finding 3:** Could detect the iperf2 install path via `where iperf.exe` and use it explicitly. Trade-off: adds an extra SSH round-trip; PATH resolution is simpler and already works.
- **Finding 5:** Could persist `ServerManager` state to a file between CLI invocations. Trade-off: adds complexity for no practical benefit — `StopServer` should unconditionally attempt to kill regardless of local state.

---

## Verification Plan

**Automated tests:**
- All existing unit tests pass: `go test ./...` — confirmed.
- `TestFwdClientArgs_IPv6` and `TestRevClientCmd_IPv6` added for IPv6 flag propagation.
- `TestServerManagerState` updated to verify nil-client error (existing test).

**Manual verification performed:**

| Test | Host | Result |
|------|------|--------|
| `--install` | rs4410 (Windows) | ✅ |
| `--start-server` | rs4410 (Windows) | ✅ |
| `--start-server` | ipv6.dpdo.ru (Linux) | ✅ |
| Forward TCP | rs4410 (no SSH) | ✅ |
| Forward TCP | ipv6.dpdo.ru (SSH) | ✅ |
| Reverse TCP | rs4410 (SSH) | ✅ |
| Bidir TCP | rs4410 (SSH) | ✅ |
| Forward UDP | rs4410 (SSH) | ✅ |
| Forward UDP | ipv6.dpdo.ru (SSH) | ✅ |
| Parallel TCP (P=4) | rs4410 (SSH) | ✅ |
| Repeat (3 iters) | rs4410 (SSH) | ✅ |
| `--stop-server` | rs4410 (Windows) | ✅ |
| `--stop-server` | ipv6.dpdo.ru (Linux) | ✅ |

---

## Preventive Recommendations

1. **Integration test with mock SSH:** Add a test that exercises `runRemoteServer()` end-to-end with a mock SSH client, verifying the SSH client is passed through to all runner methods.

2. **Parser test fixtures for all iperf2 output formats:** Add test cases for enhanced TCP server (`Reads=Dist`), enhanced UDP server (jitter/pps), standard client, enhanced client (`Write/Err/Timeo`), and TCP verbose (`Write/Err Rtry Cwnd`). These should be captured from real iperf2 output.

3. **SSH command smoke test:** Any command passed to `RunCommand()` that includes `&` without stdout redirection should be flagged in code review — it will hang on Linux.

4. **State-free server management:** CLI tools should not rely on in-memory state (`m.running`) across invocations. `StopServer` and `CheckStatus` should always query actual remote state.

5. **`FormatBidirInterval` nil-safety:** `FormatBidirInterval()` should gracefully handle nil `fwd` or `rev` arguments by displaying dashes or zeros, rather than relying on callers to handle this.

---

## Open Questions / Missing Information

| Question | Why It Matters |
|----------|---------------|
| Does `nohup iperf ... > /dev/null 2>&1 &` work correctly on all target Linux distributions (Alpine, BusyBox, etc.)? | Some minimal distributions may not have `nohup`. The `-D` daemon flag could be used as fallback. |
| What is the correct `LocalAddr` when the SSH connection passes through a SOCKS proxy? | Observed: `SSH_CLIENT` showed the proxy's address (`81.7.17.12`), not the actual client IP. `LocalAddr()` on the Go SSH client returns `127.0.0.1` (local end of the proxy socket). Reverse/bidir tests over proxied connections will fail because the remote client cannot route to `127.0.0.1`. |
| Should the `remoteServerStartCmd()` path in `config.go` use `-D` (daemon) instead of `nohup ... &`? | The `-D` flag cleanly daemonizes iperf2 without stdout issues, but may not support all the flags (`-f`, `-i`, `-e`) that the runner needs for output format control. Testing is needed. |
| Is `taskkill /IM iperf.exe /F` (force flag added in `StopServer`) safe? | Finding 3 from `iperf2-udp-bidir-findings.md` documents that `/F` prevents `-o` file flush. The `/F` was added to `StopServer()` during this fix but may cause data loss in SSH fallback scenarios. |
