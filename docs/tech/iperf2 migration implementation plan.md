# iperf2 Migration — Implementation Plan

## Context

Fully migrate iperf-tool from iperf3 to iperf2 as the primary measurement backend. iperf2 is chosen because:

- **UDP bidirectional on Windows works** — iperf3 `--bidir` + UDP on Windows produces EAGAIN errors (PR #1163)
- **UDP parallel on Windows works** — with the port-range workaround (see Finding 1 below)
- **Simpler text output** — no streaming JSON dependency, no `--json-stream` version gate (iperf3 3.17+)
- **Widely deployed** — iperf2 2.1.x+ available on all major platforms

The implementation creates a new `internal/iperf2/` package and modifies the CLI/GUI integration points. The existing `internal/iperf/` (iperf3) package remains for backward compatibility but is no longer the default.

### Key Findings Driving the Design

These findings from live testing (2026-03-12) are the architectural constraints for the implementation:

1. **`udp_accept` race on Windows** — `-P 2` on a single port drops one stream's Server Report ACK. Workaround: port range `-p start-end` on **both** server and client.
2. **NAT-dependent ACK loss** — The UDP Server Report ACK fails when the client is behind standard NAT. Works on directly routable networks (LAN, VPN, public IPs).
3. **Fabricated Server Report** — When the ACK fails, iperf2 prints a fake Server Report with `0.000 ms` jitter and `0%` loss. This is NOT real measurement data.
4. **`taskkill /F` prevents file flush** — Graceful kill (`taskkill /IM` without `/F`) required for server `-o` file to be written.
5. **Port collision in bidir** — Both directions on the same port produce identical (wrong) statistics. Non-overlapping port ranges per direction required.
6. **WARNING suppressed on SIGTERM** — Interrupted tests don't print the `WARNING: ack of last datagram failed` even though the ACK was not received.
7. **Server `-o` file always written** — On graceful server kill, the output file is written regardless of client state (interrupted, hard-killed, or completed normally).

---

## Architecture

### New package: `internal/iperf2/`

Five files: `config.go`, `parser.go`, `runner.go`, `probe.go`, `probe_test.go`.

### Test flow: Pre-flight Probe → Mode Selection → Test Execution

```
                    ┌─────────────────────┐
                    │  Pre-flight UDP     │
                    │  Reachability Probe │
                    └─────────┬───────────┘
                              │
                    ┌─────────▼───────────┐
                    │ Inbound UDP open?   │
                    └───┬─────────────┬───┘
                        │             │
                       YES            NO
                        │             │
              ┌─────────▼──┐  ┌──────▼──────────┐
              │ Direct Mode│  │ SSH Fallback Mode│
              └─────────┬──┘  └──────┬───────────┘
                        │            │
                        ▼            ▼
              Server Report      SSH-read server
              from client        -o output file
              stdout (valid)     (authoritative)
```

**Direct mode:** Server Report ACK arrives → parse from client stdout. No `-o` file, no extra SSH round-trips.

**SSH fallback mode:** Server started with `-o <file>` → after test, graceful kill → SSH-read file → parse. Authoritative server-side data regardless of NAT.

The probe runs once before the first reverse/bidir test. Forward-only tests skip the probe (the ACK travels the same path as data).

### Integration with existing codebase

The migration replaces iperf3 as the **default** backend:

- `internal/cli/runner.go` — `LocalTestRunner()` calls iperf2 runner by default, falls back to iperf3 only if `--iperf3` flag is set
- `main.go` — `runRemoteServer()` routes to iperf2 runner
- `ui/controls.go` — GUI test execution calls iperf2 runner
- All existing output formats (CSV, TXT, interval logs) remain unchanged — they consume `model.TestResult` which is backend-agnostic

---

## Files to Create

### `internal/iperf2/config.go`

Package `iperf2`. Imports: `fmt`, `regexp`, `strconv`, `strings`.

**Struct `Config`:**
```go
type Config struct {
    BinaryPath       string  // local iperf2 binary, default "iperf"
    ServerAddr       string  // remote host IP/hostname
    LocalAddr        string  // local IP (for remote client to connect back to)
    PortStart        int     // first port, e.g. 5201
    NumStreams       int     // port range size = stream count; ports PortStart..PortStart+NumStreams-1
    Duration         int     // -t seconds
    Interval         int     // -i seconds
    Bandwidth        string  // -b e.g. "7m"
    Protocol         string  // "udp" or "tcp"
    Enhanced         bool    // -e flag (enhanced output with latency, PPS, etc.)
    Reverse          bool    // reverse direction (remote→local only)
    Bidir            bool    // bidirectional (both directions simultaneously)

    // SSH fallback fields — populated when probe detects NAT
    SSHFallback      bool    // true = use SSH file fallback for server-side data
    RemoteOutputFile string  // file path on remote host for server output
    IsWindows        bool    // remote host is Windows (affects shell commands)

    // Probe configuration
    ProbeTimeout     time.Duration // UDP probe timeout, default 2s
    SkipProbe        bool          // skip pre-flight probe (force direct or SSH mode)

    // Timing
    KillWaitMs       int     // post-kill wait before reading file, default 500
}
```

**Method `Validate() error`:**
- ServerAddr non-empty
- PortStart 1..65000
- NumStreams 1..32; PortStart+NumStreams-1 <= 65535
- Duration >= 1
- Interval >= 1
- Bandwidth matches `^\d+[kmgKMG]?$` or empty
- Protocol is "udp" or "tcp"
- If Bidir: NumStreams*2 port range fits (PortStart+NumStreams*2-1 <= 65535) — forward and reverse need separate ranges
- If SSHFallback: RemoteOutputFile non-empty
- If Reverse || Bidir: LocalAddr non-empty (remote client needs to connect back)

**Method `PortRangeStr(offset int) string`:**
Returns port range string with offset. `PortRangeStr(0)` for forward, `PortRangeStr(NumStreams)` for reverse.
- NumStreams=1, offset=0: `"5201"`
- NumStreams=2, offset=0: `"5201-5202"`
- NumStreams=2, offset=2: `"5203-5204"`

**Method `fwdServerArgs() []string`:**
Server args for the forward direction (remote side receives).
```
["-s", "-u"/"-t" based on Protocol, "-p", PortRangeStr(0), "-f", "m", "-i", interval]
+ ["-e"] if Enhanced
+ ["-o", RemoteOutputFile] if SSHFallback
```

**Method `fwdClientArgs() []string`:**
Client args for forward direction (local side sends).
```
["-c", ServerAddr, "-u"/omit, "-p", PortRangeStr(0), "-t", dur, "-f", "m", "-i", interval]
+ ["-b", Bandwidth] if Bandwidth != "" && Protocol == "udp"
+ ["-e"] if Enhanced
```

**Method `revServerArgs() []string`:**
Server args for reverse direction (local side receives).
```
["-s", "-u"/"-t", "-p", PortRangeStr(NumStreams), "-f", "m", "-i", interval]
+ ["-e"] if Enhanced
```

**Method `revClientCmd() string`:**
Remote client command for reverse direction (remote side sends).
```
iperf -c <LocalAddr> -u -p <PortRangeStr(NumStreams)> -t <dur> -f m
+ -b <Bandwidth> if UDP
+ -e if Enhanced
```

**Method `remoteServerStartCmd() string`:**
Starts remote server for forward direction.
```
Windows: start /B iperf.exe <fwdServerArgs joined>
Unix:    iperf <fwdServerArgs joined> &
```
Note: `-o` is included only when SSHFallback=true (already in fwdServerArgs).

**Method `remoteServerKillCmd() string`:**
```
Windows: taskkill /IM iperf.exe    (NO /F — graceful kill for file flush)
Unix:    pkill -TERM iperf
```

**Method `remoteServerReadCmd() string`:**
Only called when SSHFallback=true.
```
Windows: type <RemoteOutputFile>
Unix:    cat <RemoteOutputFile>
```

---

### `internal/iperf2/probe.go`

Package `iperf2`. Implements the pre-flight UDP reachability probe.

**Function `ProbeUDPReachability(ctx context.Context, sshCli SSHClient, localAddr string, probeTimeout time.Duration, isWindows bool) (bool, error)`:**

```
1. Bind a local UDP socket on an ephemeral port
2. Via SSH, instruct remote to send a single UDP packet to localAddr:port
   - Linux:   echo -n PROBE | nc -u -w1 <localAddr> <port>
   - Windows: PowerShell -Command "$u=New-Object System.Net.Sockets.UdpClient; $b=[Text.Encoding]::ASCII.GetBytes('PROBE'); $u.Send($b,$b.Length,'<localAddr>',<port>); $u.Close()"
3. SetReadDeadline(probeTimeout) on local socket
4. Read from socket
5. If data received → return true, nil  (inbound UDP open)
6. If timeout      → return false, nil  (NAT blocking inbound)
7. If other error  → return false, err
```

The probe is topology-agnostic. It works on:
- LAN (both sides private IPs, no NAT) → returns true
- VPN (mesh or tunnel) → returns true
- Public IPs on both sides → returns true
- Client behind NAT → returns false
- Remote behind NAT → returns true (probe goes outbound from remote, which NAT allows)

**When to probe:**
| Direction | Probe needed | Why |
|---|---|---|
| Forward only | No | Server Report ACK travels remote→client, same direction as data |
| Reverse | Yes | ACK must travel client→remote against data direction |
| Bidirectional | Yes | Forward ACK goes remote→client (ok), reverse ACK goes client→remote (NAT may block) |

---

### `internal/iperf2/parser.go`

Package `iperf2`. Imports: `fmt`, `math`, `regexp`, `strconv`, `strings`, `iperf-tool/internal/model`.

**Package-level compiled regexes:**

```go
var (
    // Standard server-side interval: jitter and lost/total columns
    // [  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/511 (52%)
    reServerInterval = regexp.MustCompile(
        `^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+([\d.]+)\s+ms\s+(\d+)/(\d+)\s+\(([\d.]+)%\)`)

    // Enhanced server-side interval (-e mode): adds latency, PPS, NetPwr, etc.
    // [  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps
    reServerEnhanced = regexp.MustCompile(
        `^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+([\d.]+)\s+ms\s+(\d+)/(\d+)\s+\(([\d.]+)%\)\s+(-?[\d.]+)/\s*(-?[\d.]+)/\s*(-?[\d.]+)/\s*(-?[\d.]+)\s+ms\s+(\d+)\s+pps`)

    // Client-side interval: no jitter/loss columns
    // [  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec
    reClientInterval = regexp.MustCompile(
        `^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec`)

    // Enhanced client-side interval (-e mode): adds Write/Err/Timeo
    // [  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  125/0/0
    reClientEnhanced = regexp.MustCompile(
        `^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/(\d+)/(\d+)`)

    // TCP interval (both client and server): includes window size
    // [  1]  0.00-1.00 sec  1.12 MBytes  9.44 Mbits/sec
    reTCPInterval = regexp.MustCompile(
        `^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec`)

    // SUM line (server-side, with loss):
    // [SUM-2]  0.00-10.00 sec  9.77 MBytes  5.59 Mbits/sec  4942/11909 (41%)
    reSumServer = regexp.MustCompile(
        `^\[SUM-?\d*\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/(\d+)\s+\(([\d.]+)%\)`)

    // SUM line (client-side, no loss):
    // [SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec
    reSumClient = regexp.MustCompile(
        `^\[SUM(?:-\d+)?\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec`)

    // Server Report marker — client received server-side stats via ACK
    reServerReport = regexp.MustCompile(`(?i)server\s+report`)

    // WARNING: ack of last datagram failed — server report is fabricated
    reACKWarning = regexp.MustCompile(`WARNING.*ack.*last.*datagram`)
)
```

**Internal struct `parsedLine`:**
```go
type parsedLine struct {
    streamID     int
    timeStart    float64
    timeEnd      float64
    bytes        int64       // converted from MBytes
    bandwidthBps float64     // Mbits/sec * 1e6
    jitterMs     float64     // 0 if client-side
    lostPackets  int
    totalPackets int
    lostPct      float64
    isSum        bool
    hasUDP       bool        // true = server-side (jitter/loss present)
    // Enhanced fields
    latencyAvgMs float64
    latencyMinMs float64
    latencyMaxMs float64
    latencyStdev float64
    pps          int
    writeCount   int         // client-side -e
    errCount     int
    timeoCount   int
}
```

**Function `parseSingleLine(line string) (*parsedLine, bool)`:**
1. Try `reServerEnhanced` first (most specific). Parse all groups including latency/PPS.
2. Else try `reServerInterval`. Parse standard server fields.
3. Else try `reClientEnhanced`. Parse bandwidth + Write/Err/Timeo.
4. Else try `reClientInterval`. Parse bandwidth only.
5. Else return nil, false.

**Function `ParseOutput(text string, isServerSide bool) (*model.TestResult, error)`:**

Same algorithm as before (split, group by time bucket, aggregate), with additions:
- Detect `reServerReport` line → set `serverReportPresent = true`
- Detect `reACKWarning` → set `ackWarning = true`
- If `!isServerSide && serverReportPresent && ackWarning`:
  - The Server Report section in client output is **fabricated** — discard it
  - Set `result.ServerReportFabricated = true` (new field on TestResult or returned separately)
- If `!isServerSide && serverReportPresent && !ackWarning`:
  - The Server Report is valid — parse it as server-side data embedded in client output

**Function `ValidateServerReport(text string) ServerReportStatus`:**

Returns one of:
```go
type ServerReportStatus int
const (
    ServerReportValid      ServerReportStatus = iota  // summary + no WARNING + exit 0
    ServerReportFabricated                             // summary + WARNING present
    ServerReportMissing                                // no summary line at all
    ServerReportUnknown                                // interrupted (no WARNING but exit != 0)
)
```

This replaces ad-hoc WARNING checks. The caller uses this to decide whether to trust client-side server data or fall back to SSH file.

**Function `MergeBidirResults(fwdClient, fwdServer, revClient, revServer *model.TestResult) *model.TestResult`:**

Same as before:
```go
result := *fwdClient
result.Direction         = "Bidirectional"
result.FwdReceivedBps    = fwdServer.FwdReceivedBps
result.FwdLostPackets    = fwdServer.FwdLostPackets
result.FwdPackets        = fwdServer.FwdPackets
result.FwdJitterMs       = fwdServer.FwdJitterMs
result.FwdLostPercent    = fwdServer.FwdLostPercent
result.ReverseSentBps    = revClient.SentBps
result.ReverseReceivedBps = revServer.FwdReceivedBps
result.ReverseJitterMs    = revServer.FwdJitterMs
result.ReverseLostPackets = revServer.FwdLostPackets
result.ReversePackets     = revServer.FwdPackets
result.ReverseLostPercent = revServer.FwdLostPercent
result.ReverseIntervals   = revServer.Intervals
return &result
```

**Function `MergeUnidirResults(client, server *model.TestResult) *model.TestResult`:**

For forward-only or reverse-only tests — merges client send data with server receive data:
```go
result := *client
result.FwdReceivedBps  = server.FwdReceivedBps
result.FwdLostPackets  = server.FwdLostPackets
result.FwdPackets      = server.FwdPackets
result.FwdJitterMs     = server.FwdJitterMs
result.FwdLostPercent  = server.FwdLostPercent
return &result
```

---

### `internal/iperf2/runner.go`

Package `iperf2`. Imports: `bytes`, `context`, `fmt`, `os/exec`, `sync`, `syscall`, `time`, `iperf-tool/internal/model`.

**Interface `SSHClient`:**
```go
type SSHClient interface {
    RunCommand(cmd string) (string, error)
}
```
Satisfied by `*ssh.Client` from `internal/ssh/client.go`.

**Struct `Runner`:**
```go
type Runner struct {
    mu          sync.Mutex
    localSrvCmd *exec.Cmd
    fwdCmd      *exec.Cmd
}
func NewRunner() *Runner { return &Runner{} }
```

**Method `Stop()`:**
Sends SIGTERM to local server and forward client processes.

**Method `RunForward(ctx, cfg, sshCli, onInterval) (*model.TestResult, error)`:**

Forward-only test (local client → remote server). Steps:

1. If cfg.Protocol == "udp" && !cfg.SkipProbe: (probe not needed for forward, but check note below)
2. Kill any leftover remote iperf
3. Start remote server (with or without `-o` based on SSHFallback)
4. Run local client, capture output
5. If SSHFallback: graceful kill server → wait → SSH-read file → parse as server-side
6. Else: parse client output, validate Server Report via `ValidateServerReport()`
   - If `ServerReportValid`: use embedded server data
   - If `ServerReportFabricated` or `ServerReportUnknown`: log warning, return client-side data only (loss/jitter unavailable)
7. Merge client + server results
8. Return merged result

**Method `RunReverse(ctx, cfg, sshCli, onInterval) (*model.TestResult, error)`:**

Reverse-only test (remote client → local server). Steps:

1. Run pre-flight UDP probe (unless SkipProbe)
2. Based on probe result, set SSHFallback mode
3. Kill any leftover remote iperf
4. Start local server for reverse (port offset = NumStreams)
5. Start remote client via SSH
6. Wait for remote client to finish
7. Stop local server, capture output
8. Parse local server output (authoritative — this is the receiving side)
9. Parse remote client output (send-side stats)
10. Merge results

**Method `RunBidir(ctx, cfg, sshCli, onInterval) (*model.TestResult, error)`:**

Bidirectional test — both directions simultaneously. Steps:

1. **Pre-flight probe** (unless SkipProbe):
   ```
   reachable, err := ProbeUDPReachability(ctx, sshCli, cfg.LocalAddr, cfg.ProbeTimeout, cfg.IsWindows)
   if err != nil: log warning, default to SSHFallback=true
   cfg.SSHFallback = !reachable
   ```

2. **Kill any leftover remote iperf:**
   ```
   sshCli.RunCommand(cfg.remoteServerKillCmd())   // ignore error
   time.Sleep(300ms)
   ```

3. **Start remote server (forward direction receives):**
   ```
   sshCli.RunCommand(cfg.remoteServerStartCmd())
   time.Sleep(600ms)   // allow ports to bind
   ```

4. **Start local server (reverse direction receives):**
   ```
   localSrvCmd = exec.CommandContext(ctx, cfg.BinaryPath, cfg.revServerArgs()...)
   localSrvCmd.Stdout = &localSrvBuf
   localSrvCmd.Start()
   time.Sleep(400ms)
   ```

5. **Run both clients concurrently:**
   ```
   go func() { // Forward: local client → remote server
       cmd := exec.CommandContext(ctx, cfg.BinaryPath, cfg.fwdClientArgs()...)
       fwdCh <- result{buf.String(), cmd.Run()}
   }()
   go func() { // Reverse: remote client → local server
       out, err := sshCli.RunCommand(cfg.revClientCmd())
       revCh <- result{out, err}
   }()
   fwdOut := <-fwdCh
   revOut := <-revCh
   ```

6. **Stop local server:**
   ```
   localSrvCmd.Process.Signal(SIGTERM)
   localSrvCmd.Wait()
   localSrvOutput := localSrvBuf.String()
   ```

7. **Get remote server data:**
   ```
   if cfg.SSHFallback:
       sshCli.RunCommand(cfg.remoteServerKillCmd())   // graceful
       time.Sleep(cfg.KillWaitMs)
       remoteSrvOutput, err := sshCli.RunCommand(cfg.remoteServerReadCmd())
       // Assert non-empty before parsing
   else:
       sshCli.RunCommand(cfg.remoteServerKillCmd())
       // Server Report is in fwdOut (client stdout) — validate it
       status := ValidateServerReport(fwdOut.out)
       if status == ServerReportValid:
           // parse server data from client output
       else:
           // log warning: server data unavailable in direct mode
   ```

8. **Parse all outputs:**
   ```
   fwdClientResult  := ParseOutput(fwdOut.out, false)
   revClientResult  := ParseOutput(revOut.out, false)
   remoteSrvResult  := ParseOutput(remoteSrvOutput, true)   // from file or client stdout
   localSrvResult   := ParseOutput(localSrvOutput, true)
   ```

9. **Merge:**
   ```
   merged := MergeBidirResults(fwdClientResult, remoteSrvResult, revClientResult, localSrvResult)
   merged.Timestamp = time.Now()
   ```

10. **Fire onInterval callbacks (post-test replay):**
    ```
    if onInterval != nil:
        for i := 0; i < max(len(merged.Intervals), len(merged.ReverseIntervals)); i++:
            onInterval(fwd[i], rev[i])
    ```

**Method `RunTCP(ctx, cfg, sshCli, onInterval) (*model.TestResult, error)`:**

TCP test — simpler than UDP because no Server Report ACK issue, no probe needed.

1. Kill leftover remote iperf
2. Start remote server (with `-o` if SSHFallback for consistency)
3. Run local client
4. Parse client output — TCP has no loss/jitter, just throughput
5. If SSHFallback: read server file for server-side perspective
6. Return result

**Error policy:**
- Client run failures in bidir: collect both results, continue to parse. Partial data is useful.
- SSH failures: abort, clean up with best-effort kill.
- Empty server file: return error with diagnostic message, not silent zeros.

---

### `internal/iperf2/probe_test.go`

Tests for the UDP probe:
- `TestProbeUDPReachability_Open` — mock SSH sends packet, local socket receives → true
- `TestProbeUDPReachability_Blocked` — mock SSH succeeds but no packet arrives → false (timeout)
- `TestProbeUDPReachability_SSHError` — SSH command fails → false, error

---

## Files to Modify

### `internal/model/result.go`

**Add field to `TestResult`:**
```go
IperfVersion string // "iperf2" or "iperf3"
```

This field is already used in the CSV export but may not be in the struct yet. Verify and add if missing.

### `internal/cli/runner.go`

**1. Update `RunnerConfig`:**
```go
// iperf2 is now the default. Use --iperf3 to force iperf3.
Iperf3           bool   // force iperf3 runner instead of iperf2
Iperf2Binary     string // local iperf2 binary path (default "iperf")
LocalAddr        string // local IP for remote client to connect back to
RemoteOutputFile string // file path on remote for iperf2 server output
SkipProbe        bool   // skip pre-flight UDP probe
```

**2. Update `LocalTestRunner(cfg)` default path:**

Currently calls iperf3 runner. Change to:
```go
func LocalTestRunner(cfg RunnerConfig) (*model.TestResult, error) {
    if cfg.Iperf3 {
        return localTestRunnerIperf3(cfg)  // existing iperf3 code, moved
    }
    return localTestRunnerIperf2(cfg)      // new default
}
```

**3. Add `localTestRunnerIperf2(cfg)` function:**

Handles local-only iperf2 tests (no SSH). For simple forward TCP/UDP tests where both client and server are local or the server is remote but accessible without SSH.

**4. Add `RunIperf2Bidir(cfg, sshCli)` function:**

Orchestrates the full bidir flow including probe:
```go
func RunIperf2Bidir(cfg RunnerConfig, sshCli iperf2.SSHClient) (*model.TestResult, error) {
    iperf2Cfg := iperf2.Config{
        BinaryPath:       cfg.Iperf2Binary,
        ServerAddr:       cfg.ServerAddr,
        LocalAddr:        cfg.LocalAddr,
        PortStart:        cfg.Port,
        NumStreams:        cfg.Parallel,
        Duration:         cfg.Duration,
        Interval:         cfg.Interval,
        Bandwidth:        cfg.Bandwidth,
        Protocol:         cfg.Protocol,
        Enhanced:         true,
        Bidir:            true,
        RemoteOutputFile: cfg.RemoteOutputFile,
        IsWindows:        detectRemoteOS(sshCli),
        SkipProbe:        cfg.SkipProbe,
        ProbeTimeout:     2 * time.Second,
        KillWaitMs:       500,
    }
    if err := iperf2Cfg.Validate(); err != nil { return nil, err }

    // Print header
    isUDP := cfg.Protocol == "udp"
    header := "Time      " + format.FormatBidirIntervalHeader(isUDP)
    fmt.Println(header)
    fmt.Println(strings.Repeat("-", len(header)))
    testStart := time.Now()

    runner := iperf2.NewRunner()
    result, err := runner.RunBidir(context.Background(), iperf2Cfg, sshCli,
        func(fwd, rev *model.IntervalResult) {
            if fwd != nil {
                ts := testStart.Add(time.Duration(fwd.TimeStart * float64(time.Second))).Format("15:04:05")
                fmt.Println(ts + "  " + format.FormatBidirInterval(fwd, rev, isUDP))
            }
        })
    if err != nil { return nil, err }

    // Metadata
    result.Protocol = strings.ToUpper(cfg.Protocol)
    result.Direction = "Bidirectional"
    result.Parallel = cfg.Parallel
    result.IperfVersion = "iperf2"
    if h, _ := os.Hostname(); h != "" { result.LocalHostname = h }
    result.LocalIP = netutil.OutboundIP()
    result.SSHRemoteHost = cfg.SSHHost
    result.MeasurementID = export.NextMeasurementID(result.Timestamp)
    saveResults(result, cfg)
    return result, nil
}
```

**5. Add `RunIperf2Forward(cfg, sshCli)` and `RunIperf2Reverse(cfg, sshCli)` functions:**

Similar to RunIperf2Bidir but for single-direction tests.

**6. Add `detectRemoteOS(sshCli) bool` helper:**

```go
func detectRemoteOS(sshCli iperf2.SSHClient) bool {
    out, err := sshCli.RunCommand("ver")
    if err == nil && strings.Contains(strings.ToLower(out), "windows") {
        return true
    }
    return false
}
```

**7. Add `Client()` method to `RemoteServerRunner`:**
```go
func (r *RemoteServerRunner) Client() iperf2.SSHClient { return r.client }
```

### `internal/cli/flags.go`

**Replace iperf2-specific flags with iperf3 opt-in:**
```go
fs.BoolVar(&cfg.Iperf3, "iperf3", false, "Use iperf3 instead of iperf2 (legacy)")
fs.StringVar(&cfg.Iperf2Binary, "iperf2-binary", "iperf", "Path to local iperf2 binary")
fs.StringVar(&cfg.LocalAddr, "local-addr", "", "Local IP address for reverse/bidir connections")
fs.StringVar(&cfg.RemoteOutputFile, "remote-output-file", "", "Remote file path for iperf2 server output (auto-detected per OS)")
fs.BoolVar(&cfg.SkipProbe, "skip-probe", false, "Skip pre-flight UDP reachability probe")
```

**Update `PrintUsage()` with new sections:**
```
BACKEND SELECTION:
  --iperf3                 Use iperf3 instead of iperf2 (legacy mode)
  --iperf2-binary <path>   Path to local iperf2 binary (default: iperf)

IPERF2 OPTIONS:
  --local-addr <ip>        Local IP address for reverse/bidir connections
  --remote-output-file <p> Remote file path for server output (auto-detected per OS if omitted)
  --skip-probe             Skip pre-flight UDP reachability probe (force direct mode)
```

### `main.go`

**In `runRemoteServer()`**, update the test execution branch:

```go
if cfg.ServerAddr != "" {
    if cfg.Iperf3 {
        // Legacy iperf3 path
        cfg.RestartServerFunc = func(numInstances int) error {
            return runner.Restart(numInstances)
        }
        result, err := cli.LocalTestRunner(*cfg)
        ...
    } else {
        // Default: iperf2 path
        sshCli := runner.Client()
        var result *model.TestResult
        var err error
        if cfg.Bidir {
            result, err = cli.RunIperf2Bidir(*cfg, sshCli)
        } else if cfg.Reverse {
            result, err = cli.RunIperf2Reverse(*cfg, sshCli)
        } else {
            result, err = cli.RunIperf2Forward(*cfg, sshCli)
        }
        ...
    }
}
```

### `ui/controls.go`

**Update `startTest()` to use iperf2 runner by default.** The GUI test execution currently calls `LocalTestRunner()` which will be updated to route to iperf2 by default.

For SSH-connected tests in the GUI, add a code path that uses `RunIperf2Bidir` / `RunIperf2Forward` / `RunIperf2Reverse` when an SSH connection is active.

---

## Test Files to Create

### `internal/iperf2/config_test.go`

- `TestPortRangeStr`: NumStreams=1→`"5201"`, NumStreams=2→`"5201-5202"`, offset=2→`"5203-5204"`
- `TestValidate_OK`: valid config passes
- `TestValidate_Errors`: empty ServerAddr, zero PortStart, NumStreams=0, bad bandwidth, bidir port overflow
- `TestRemoteStartCmdWindows`: contains `start /B`, `-o` when SSHFallback
- `TestRemoteStartCmdUnix`: ends with `&`, `>` when SSHFallback
- `TestRemoteKillCmd`: Windows→`taskkill /IM`, Unix→`pkill -TERM`
- `TestFwdClientArgs_TCP`: protocol=tcp, no `-u`, no `-b`
- `TestFwdClientArgs_UDP`: protocol=udp, has `-u`, `-b`

### `internal/iperf2/parser_test.go`

Test fixtures from real iperf2 output captured during session.

**Fixture: 2-stream UDP server output (10s, `-e` enhanced, port range):**
```
------------------------------------------------------------
Server listening on UDP port 5201 to 5202
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.89.230.34 port 5201 connected with 100.80.223.29 port 52714
[  2] local 100.89.230.34 port 5202 connected with 100.80.223.29 port 52715
[ ID] Interval            Transfer     Bandwidth        Jitter   Lost/Total  Latency avg/min/max/stdev PPS
[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps
[  2]  0.00-1.00 sec  0.355 MBytes  2.98 Mbits/sec   5.422 ms  247/  500 (49%)  -0.411/ 0.388/ 1.187/ 0.799 ms  500 pps
[  1]  1.00-2.00 sec  0.437 MBytes  3.67 Mbits/sec  18.598 ms  197/  509 (39%)  -0.830/ 0.163/ 1.184/ 1.007 ms  509 pps
[  2]  1.00-2.00 sec  0.367 MBytes  3.08 Mbits/sec   7.630 ms  238/  500 (48%)  -0.509/ 0.332/ 1.177/ 0.854 ms  500 pps
[SUM-2]  0.00-10.03 sec  9.77 MBytes  8.17 Mbits/sec   6.405 ms 4942/11909 (41%)
```

**Fixture: 2-stream UDP client output (10s, `-e` enhanced):**
```
------------------------------------------------------------
Client connecting to 100.89.230.34, UDP port 5201 to 5202
Sending 1470 byte datagrams, IPG target: 1127.27 us (kalman adjust)
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.80.223.29 port 52714 connected with 100.89.230.34 port 5201
[  2] local 100.80.223.29 port 52715 connected with 100.89.230.34 port 5202
[ ID] Interval            Transfer     Bandwidth       Write/Err/Timeo
[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  2]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec  11907/0/0
[  1] Server Report:
[ ID] Interval            Transfer     Bandwidth        Jitter   Lost/Total  Datagrams
[  1]  0.00-10.03 sec  5.14 MBytes  4.30 Mbits/sec  10.088 ms 2461/6129 (40%)
[  2] Server Report:
[  2]  0.00-10.03 sec  4.63 MBytes  3.87 Mbits/sec   5.422 ms 2481/5780 (43%)
```

**Fixture: fabricated Server Report (NAT, no ACK):**
```
[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec
[  1] Server Report:
[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec   0.000 ms    0/6250 (0%)
WARNING: did not receive ack of last datagram after 10 tries.
```

**Fixture: TCP multistream output:**
```
[  1]  0.00-1.00 sec  1.12 MBytes  9.44 Mbits/sec
[  2]  0.00-1.00 sec  1.12 MBytes  9.40 Mbits/sec
[SUM]  0.00-10.02 sec  22.4 MBytes  18.7 Mbits/sec
```

Test functions:
- `TestParseClientOutput`: 2-stream client, 10 intervals. Assert len(Intervals)==10, SentBps≈14e6.
- `TestParseServerOutput`: 2-stream server with jitter/loss. Assert FwdJitterMs>0, FwdLostPackets≥0.
- `TestParseServerEnhanced`: `-e` mode output. Assert latency fields parsed, PPS > 0.
- `TestParseSumLineServerAuth`: SUM line present → total fields from SUM.
- `TestParseEmptyOutput`: empty string → error.
- `TestParseFabricatedServerReport`: detect 0.000ms jitter + WARNING → ServerReportFabricated.
- `TestValidateServerReport_Valid`: no WARNING, summary present → Valid.
- `TestValidateServerReport_Fabricated`: WARNING present → Fabricated.
- `TestValidateServerReport_Missing`: no summary line → Missing.
- `TestMergeBidirResults`: four stub results → all 10+ fields populated correctly.
- `TestMergeUnidirResults`: two stub results → client + server merged.
- `TestParseTCPOutput`: TCP interval parsing, no jitter/loss fields.

### `internal/iperf2/runner_test.go`

```go
type mockSSHClient struct {
    calls     []string
    responses map[string]string
    errors    map[string]error
}
func (m *mockSSHClient) RunCommand(cmd string) (string, error) {
    m.calls = append(m.calls, cmd)
    for k, v := range m.errors {
        if strings.Contains(cmd, k) { return "", v }
    }
    for k, v := range m.responses {
        if strings.Contains(cmd, k) { return v, nil }
    }
    return "", nil
}
```

- `TestRunBidirCallOrder`: verify SSH calls include start, client, kill, read in order.
- `TestRunBidirRemoteStartFails`: mock start returns error → RunBidir returns error.
- `TestRunBidirReadFileFails`: mock read returns error → error returned.
- `TestRunBidirOnIntervalCalled`: mock outputs with 3 intervals → onInterval called 3 times.
- `TestRunForwardDirect`: direct mode (no SSH fallback) → no read-file call.
- `TestRunForwardSSHFallback`: SSHFallback=true → read-file call present.
- `TestRunReverse`: reverse direction → local server started on offset ports.
- `TestRunTCP`: TCP mode → no `-u` flag, no probe.

---

## Reused Functions and Utilities

| Function | File | Used for |
|---|---|---|
| `format.FormatBidirIntervalHeader(isUDP)` | `internal/format/result.go` | Header row |
| `format.FormatBidirInterval(fwd, rev, isUDP)` | `internal/format/result.go` | Per-interval row |
| `format.FormatResult(result)` | `internal/format/result.go` | Final summary |
| `export.NextMeasurementID(ts)` | `internal/export/filename.go` | Result ID |
| `export.WriteCSV`, `WriteTXT`, `WriteIntervalLog` | `internal/export/` | File output (unchanged) |
| `netutil.OutboundIP()` | `internal/netutil/` | Local IP metadata |
| `saveResults(result, cfg)` | `internal/cli/runner.go` | CSV/TXT export (unchanged) |
| `ssh.Client.RunCommand(cmd)` | `internal/ssh/client.go` | Satisfies `iperf2.SSHClient` |

---

## Real Test Output Samples

### UDP 2-stream bidir (10s, port range, enhanced, VPN path)

**Forward client (local → remote):**
```
------------------------------------------------------------
Client connecting to 100.89.230.34, UDP port 5201 to 5202
Sending 1470 byte datagrams, IPG target: 1127.27 us (kalman adjust)
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.80.223.29 port 52714 connected with 100.89.230.34 port 5201
[  2] local 100.80.223.29 port 52715 connected with 100.89.230.34 port 5202
[ ID] Interval            Transfer     Bandwidth       Write/Err/Timeo
[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  2]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  1]  1.00-2.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  2]  1.00-2.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
...
[SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec  11907/0/0
[  1] Server Report:
[  1]  0.00-10.03 sec  5.14 MBytes  4.30 Mbits/sec  10.088 ms 2461/6129 (40%)
[  2] Server Report:
[  2]  0.00-10.03 sec  4.63 MBytes  3.87 Mbits/sec   5.422 ms 2481/5780 (43%)
```

**Forward server (remote, from `-o` file):**
```
------------------------------------------------------------
Server listening on UDP port 5201 to 5202
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.89.230.34 port 5201 connected with 100.80.223.29 port 52714
[  2] local 100.89.230.34 port 5202 connected with 100.80.223.29 port 52715
[ ID] Interval            Transfer     Bandwidth        Jitter   Lost/Total  Latency avg/min/max/stdev PPS
[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps
[  2]  0.00-1.00 sec  0.355 MBytes  2.98 Mbits/sec   5.422 ms  247/  500 (49%)  -0.411/ 0.388/ 1.187/ 0.799 ms  500 pps
...
[SUM-2]  0.00-10.03 sec  9.77 MBytes  8.17 Mbits/sec   6.405 ms 4942/11909 (41%)
```

**Reverse client (remote → local, via SSH):**
```
------------------------------------------------------------
Client connecting to 100.80.223.29, UDP port 5203 to 5204
Sending 1470 byte datagrams, IPG target: 1127.27 us (kalman adjust)
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec
[  2]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec
...
[SUM]  0.00-10.00 sec  16.6 MBytes  13.9 Mbits/sec
```

**Reverse server (local):**
```
------------------------------------------------------------
Server listening on UDP port 5203 to 5204
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1]  0.00-1.00 sec  0.197 MBytes  1.65 Mbits/sec   8.432 ms  359/  500 (72%)
[  2]  0.00-1.00 sec  0.225 MBytes  1.89 Mbits/sec   4.117 ms  339/  500 (68%)
...
[SUM-2]  0.00-10.07 sec  4.84 MBytes  4.03 Mbits/sec   6.275 ms 7566/11877 (64%)
```

### TCP 2-stream forward (10s, enhanced)

**Client output:**
```
------------------------------------------------------------
Client connecting to 100.89.230.34, TCP port 5201
TCP window size: 0.06 MByte (default)
------------------------------------------------------------
[  1] local 100.80.223.29 port 52800 connected with 100.89.230.34 port 5201
[  2] local 100.80.223.29 port 52801 connected with 100.89.230.34 port 5201
[ ID] Interval       Transfer     Bandwidth       Write/Err/Timeo  Rtry  Cwnd/RTT
[  1]  0.00-1.00 sec  1.12 MBytes  9.44 Mbits/sec  8/0/0          0    64K/65123 us
[  2]  0.00-1.00 sec  1.12 MBytes  9.40 Mbits/sec  8/0/0          0    64K/65289 us
...
[SUM]  0.00-10.02 sec  22.4 MBytes  18.7 Mbits/sec  160/0/0
```

### Fabricated Server Report (behind NAT)

**Client output (public server, client behind NAT):**
```
[  1]  0.00-5.00 sec  4.38 MBytes  7.34 Mbits/sec  625/0/0
[  1] Server Report:
[  1]  0.00-5.00 sec  4.38 MBytes  7.34 Mbits/sec   0.000 ms    0/ 3125 (0%)
WARNING: did not receive ack of last datagram after 10 tries.
```

Note: `0.000 ms` jitter and `0/3125 (0%)` loss — fabricated from client send data. The real server stats (from `-o` file) showed `0.231 ms` jitter and `52%` loss.

### Interrupted test (SIGTERM at 15s of 50s)

**Client output:**
```
[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec
...
[  1] 14.00-14.99 sec  0.875 MBytes  7.37 Mbits/sec
[  1]  0.00-14.99 sec  13.1 MBytes  7.34 Mbits/sec
[  1] Server Report:
[  1]  0.00-14.99 sec  13.1 MBytes  7.34 Mbits/sec   0.000 ms    0/ 9368 (0%)
```

Note: No `WARNING` line — SIGTERM killed the process before the ACK retry loop. The Server Report is fabricated (0.000 ms jitter). Exit code 124.

**Server `-o` file (graceful kill after client interrupt):**
```
[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)
...
[  1] 14.00-14.99 sec  0.302 MBytes  2.54 Mbits/sec   8.991 ms  284/  500 (57%)
[  1]  0.00-14.99 sec  4.77 MBytes  2.67 Mbits/sec   9.544 ms 3827/ 5584 (69%)
```

Valid partial results — server captured exactly how long data flowed.

---

## Implementation Order

### Phase 1 — Core Parser + Config (no network I/O)
1. Create `internal/iperf2/config.go` with `Config` struct and validation
2. Create `internal/iperf2/parser.go` with all regexes and `ParseOutput`
3. Create parser tests with real output fixtures
4. Create config tests

### Phase 2 — UDP Probe
1. Create `internal/iperf2/probe.go` with `ProbeUDPReachability`
2. Create probe tests

### Phase 3 — Runner
1. Create `internal/iperf2/runner.go` with `RunBidir`, `RunForward`, `RunReverse`, `RunTCP`
2. Create runner tests with mock SSH client

### Phase 4 — CLI Integration
1. Update `internal/cli/flags.go` — new flags, iperf3 opt-in
2. Update `internal/cli/runner.go` — add `RunIperf2Bidir` etc., update `LocalTestRunner` default
3. Update `main.go` — route to iperf2 in `runRemoteServer()`
4. Verify `go build ./...` and `go test ./...`

### Phase 5 — GUI Integration
1. Update `ui/controls.go` to use iperf2 runner by default
2. Add probe result indication in UI (direct mode vs SSH fallback)
3. Test GUI flow end-to-end

---

## Verification

```
go test ./internal/iperf2/... -v && go build ./...
```

End-to-end verification:
1. **Forward UDP** — `iperf-tool -s <host> --ssh <host> -u -t 10 -P 2`
2. **Reverse UDP** — `iperf-tool -s <host> --ssh <host> -u -t 10 -P 2 --reverse`
3. **Bidir UDP** — `iperf-tool -s <host> --ssh <host> -u -t 10 -P 2 --bidir`
4. **TCP forward** — `iperf-tool -s <host> --ssh <host> -t 10 -P 2`
5. **Behind NAT** — run from NAT'd client, verify probe detects NAT and uses SSH fallback
6. **Direct network** — run on LAN/VPN, verify probe passes and uses direct mode
7. **Interrupted test** — run 50s test, Ctrl-C at 15s, verify partial results via SSH file
8. **Legacy iperf3** — `iperf-tool -s <host> --iperf3 -t 10` works as before
