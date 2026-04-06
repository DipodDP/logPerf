# Code Review Finding Report

## Summary

The SSH fallback mechanism for retrieving iperf2 server-side UDP statistics on Windows is broken. `config.go:remoteServerStartCmd()` starts the remote server using `start /B iperf.exe -o <file>`, which causes the iperf2 process to exit immediately after the client disconnects without flushing interval or summary data to the `-o` output file. The file contains only the 248-byte connection header. The correct approach — confirmed working in live testing — is to run the server **attached to the SSH session** with a shell-level stdout redirect (`cmd /c iperf.exe ... > C:\file.txt 2>&1`), which produces complete output on normal process exit.

---

## Context

- **Project:** `iperf-tool` — Go CLI + GUI wrapper around iperf2/iperf3
- **Branch:** `multistream-iperf2`
- **Language:** Go 1.x; remote host commands issued via `ssh.Client.RunCommand()`
- **Environment:** macOS client (Tailscale VPN: 100.80.223.29) → Windows remote (100.89.230.34, iperf2 2.2.1)
- **Relevant files:**
  - `internal/iperf/config.go` — `remoteServerStartCmd()`, `remoteServerReadCmd()`
  - `internal/iperf/runner.go` — `startRemoteServer()`, `readRemoteServerOutput()`, `RunForward()`, `RunBidir()`
- **Test date:** 2026-03-16 (live bare iperf2 commands, no wrapper code)

---

## Detailed Findings

### Finding A — `start /B` with `-o` Produces Header-Only Output File

**Observed behavior:** When the remote server is started via:

```
start /B iperf.exe -s -u -p 5201-5202 -f m -i 1 -e -o C:\iperf_srv.txt
```

the `-o` output file is created and written with only the 248-byte connection header:

```
------------------------------------------------------------
Server listening on UDP ports 5201-5202 with pid <N>
Read buffer size: 1470 Byte
UDP buffer size: 64.0 KByte (default)
------------------------------------------------------------
```

No interval lines, no summary lines, and no SUM lines are written — regardless of whether the process is killed gracefully (`taskkill /IM`) or exits normally after the client disconnects.

**Confirmed working alternative:** Running the server attached to the SSH session with a shell redirect:

```
cmd /c iperf.exe -s -u -p 5201-5202 -f m -i 1 -e > C:\iperf_srv.txt 2>&1
```

produced a complete 5,432-byte output file containing all per-stream interval lines, per-stream summary lines, `[SUM-2]` aggregate lines, and `WARNING: ack of last datagram failed` messages — exactly the data needed for server-side loss/jitter parsing.

**Fact vs. inference:**
- *Fact:* `-o` flag with `start /B` produced header-only file; directly observed in three separate test runs.
- *Fact:* Shell redirect with blocking SSH session produced complete output; directly observed.
- *Inference:* iperf2 on Windows 2.2.1 buffers its reporter-thread output and does not flush it to the `-o` file handle when the process exits via the end-of-test path triggered by client disconnection. The `-o` flag opens the file early (explaining the header) but the interval/summary data remains in an unflushed buffer at exit.

### Finding B — `start /B` Process Exits Before or Immediately After Client Disconnect

In testing with `PowerShell Start-Process -WindowStyle Hidden`, iperf2 was confirmed running (PID returned) at server start, but `taskkill /IM iperf.exe` at t=12s (mid-test, client running 30s) returned `ERROR: The process "iperf.exe" not found`. The server exited mid-test without any explicit kill signal.

- *Fact:* Process disappeared mid-test without being killed.
- *Inference:* `start /B` or `Start-Process -WindowStyle Hidden` detaches the process from its console session. When the iperf2 Windows process loses its console handle, it may exit on a `Ctrl+C`/console-close event that Windows sends to processes in a detached console group. This is a known Windows behavior for console applications started via `start /B` over SSH.

### Finding C — Server Reports Received in Client Output When Server Runs Attached

When the server ran attached to the SSH session (`cmd /c iperf.exe ... > file 2>&1`), both client streams received valid Server Reports:

```
[  1] Server Report:
[  1] 0.00-10.12 sec  11.5 MBytes  9.50 Mbits/sec  1.866 ms 331/8506 (3.9%) ...
[  2] Server Report:
[  2] 0.00-10.12 sec  11.5 MBytes  9.50 Mbits/sec  1.865 ms 334/8506 (3.9%) ...
```

This contradicts the expectation from the findings document that the Server Report ACK is always lost from behind Tailscale VPN NAT. The Tailscale VPN (100.x.x.x addresses) appears to maintain the UDP mapping long enough for the ACK to return, unlike standard NAT. The blocking SSH session itself may keep the iperf process in a state where it sends the ACK immediately after the test rather than after a delay.

- *Fact:* Server Reports received in client output during the attached-session run.
- *Fact:* Server Reports were NOT received (WARNING appeared) in all runs using `start /B` / `Start-Process`.
- *Inference:* The difference is likely the process's console/signal handling state, not the network topology alone.

---

## Root Cause Analysis

| Finding | Root Cause |
|---|---|
| A — Header-only `-o` file | iperf2 Windows 2.2.1 reporter thread output is stdio-buffered; the `-o` file handle receives only header output (printed before buffering begins). Interval and summary data written to the reporter's FILE* buffer is not flushed on the exit path triggered by UDP end-of-test. This is not triggered by graceful vs. force kill — it is a structural iperf2 buffering issue on Windows with the `-o` flag. |
| B — Mid-test process death | `start /B` over SSH creates a process in a detached console group. Windows sends a console-close event to detached console processes when the SSH session context changes, causing iperf2 to exit prematurely. `PowerShell Start-Process -WindowStyle Hidden` has similar behavior. |
| C — ACK received on attached session | When iperf2 runs with an intact console (blocking SSH), its signal/exit handling follows the normal end-of-test path: server sends ACK, waits, then exits. In detached mode, the process may exit before completing the ACK send sequence. |

**Affected code:**

```go
// internal/iperf/config.go:333-339
func (c *Config) remoteServerStartCmd() string {
    args := c.fwdServerArgs()
    if c.IsWindows {
        return fmt.Sprintf("start /B iperf.exe %s", strings.Join(args, " "))
    }
    return fmt.Sprintf("nohup iperf %s > /dev/null 2>&1 &", strings.Join(args, " "))
}
```

The `start /B` invocation is the root of both Finding A and Finding B on Windows. The `-o` flag in `fwdServerArgs()` (`config.go:223-225`) is also ineffective as the primary capture mechanism on Windows.

```go
// internal/iperf/runner.go:544-549
func (r *Runner) readRemoteServerOutput(cfg Config, sshCli SSHClient) (string, error) {
    sshCli.RunCommand(cfg.remoteServerKillCmd()) // graceful kill first
    time.Sleep(time.Duration(cfg.KillWaitMs) * time.Millisecond)
    return sshCli.RunCommand(cfg.remoteServerReadCmd())
}
```

`readRemoteServerOutput` reads the file written by iperf's `-o` flag, which is empty/header-only on Windows. The 500ms `KillWaitMs` wait does not help because the data was never written to the file.

---

## Impact Assessment

| Issue | Severity | Consequence |
|---|---|---|
| A — Header-only `-o` file | **High** | SSH fallback for Windows UDP forward/bidir tests returns no server-side data. Loss, jitter, and per-stream stats are silently absent. `ParseOutput` receives a header-only string and returns no intervals — the caller gets a zero-result `TestResult` with no error, making the failure invisible to the user. |
| B — Premature server exit | **High** | Server exits mid-test. On a 10s test it may exit before gathering final-interval data. Results are incomplete and non-reproducible. |
| C — ACK reception difference | **Medium** (informational) | The probe-then-fallback logic in `RunForward`/`RunBidir` may route to direct mode (no SSH fallback) when running over Tailscale VPN, which is correct behavior. However if the server is started in detached mode (as currently coded), the ACK may be suppressed by the detached-process issue regardless of network topology. |

---

## Proposed Resolutions

### Recommended Solution

Replace `start /B iperf.exe -o <file>` with a **blocking SSH command** that runs iperf attached to the session and redirects stdout to the output file via the shell:

```
cmd /c iperf.exe -s -u -p 5201-5202 -f m -i 1 -e > C:\iperf_srv.txt 2>&1
```

This requires the SSH client to issue the server start command **asynchronously** (in a goroutine), so the blocking SSH call does not prevent the test from running. The server output is then read from the file after the SSH goroutine returns.

**Implementation change in `config.go`:**

```go
func (c *Config) remoteServerStartCmd() string {
    args := c.fwdServerArgs() // NOTE: must NOT include -o flag; redirect is done at shell level
    if c.IsWindows {
        // Blocking cmd redirect — must be called in a goroutine by the runner
        return fmt.Sprintf(`cmd /c iperf.exe %s > %s 2>&1`,
            strings.Join(args, " "), c.RemoteOutputFile)
    }
    // Linux: nohup with shell redirect, non-blocking
    return fmt.Sprintf("nohup iperf %s > %s 2>&1 &",
        strings.Join(args, " "), c.RemoteOutputFile)
}
```

**Implementation change in `runner.go` — `startRemoteServer` for Windows SSH fallback:**

```go
// For Windows SSH fallback, start server in a goroutine (blocking SSH call)
go func() {
    sshCli.RunCommand(cfg.remoteServerStartCmd())
}()
time.Sleep(600 * time.Millisecond) // wait for server to bind
```

After the test completes, cancel the SSH goroutine context (or simply read the file — the SSH call will return once iperf exits after the client disconnects).

**Remove `-o` flag from `fwdServerArgs()`** since output capture is now done at the shell level:

```go
// config.go fwdServerArgs(): remove the -o block
// if c.SSHFallback && c.RemoteOutputFile != "" {
//     args = append(args, "-o", c.RemoteOutputFile)
// }
```

**How it resolves the root cause:** The shell redirect (`>`) captures iperf's stdout at the OS level as iperf writes to stdout. stdout is line-buffered in a terminal context and block-buffered when redirected — but iperf2 calls `fflush` on exit in the normal end-of-test path, so all data is written before the process exits and `cmd /c` returns.

**Risks and side effects:**
- The `startRemoteServer` goroutine must not block the test indefinitely. A context with the test duration + buffer should be used.
- The Linux path (`nohup ... &`) is unchanged and unaffected.
- `fwdServerArgs` removing the `-o` flag is a **breaking change** if any caller relies on it. Audit all callers before removing.

### Alternative Solution — Named Pipe / PowerShell Tee

Use PowerShell to run iperf and capture output:

```powershell
PowerShell -Command "iperf.exe -s -u -p 5201-5202 -f m -i 1 -e 2>&1 | Tee-Object -FilePath C:\iperf_srv.txt"
```

**Trade-off:** Requires PowerShell (available on Windows 10+/Server 2019+). Output is line-buffered through the pipe. More complex to kill cleanly; `Tee-Object` may hold the file open after iperf exits.

### Alternative Solution — Always-SSH (No `-o` File)

Run the server as a blocking SSH command, read its stdout directly from the SSH return value instead of a file:

```go
out, err := sshCli.RunCommand("iperf.exe -s -u -p 5201-5202 -f m -i 1 -e")
// out contains full server output
```

**Trade-off:** Requires the SSH client to support concurrent blocking commands (one for the server, one for the client). Eliminates the file entirely. Simpler but requires the `SSHClient` interface to support concurrent `RunCommand` calls safely. This is the cleanest approach if the SSH client supports it.

---

## Verification Plan

1. **Unit test:** Add a test in `runner_test.go` that mocks the SSH client and verifies `remoteServerStartCmd()` on Windows does NOT contain `start /B` and DOES contain `cmd /c` with a `>` redirect to the output file path.

2. **Integration test:** Run `RunForward` with `SSHFallback: true`, `IsWindows: true` against `rs4410-52465.tail566708.ts.net`. Assert:
   - `TestResult.Intervals` is non-empty
   - At least one interval has `LossPercent > 0` or `Jitter > 0` (confirming real server data, not zero-filled)
   - No error returned

3. **Manual verification:**
   ```bash
   # Start server via new cmd
   ssh administrator@rs4410-52465.tail566708.ts.net \
     'cmd /c iperf.exe -s -u -p 5201-5202 -f m -i 1 -e > C:\iperf_srv.txt 2>&1' &
   sleep 1
   iperf -c 100.89.230.34 -u -p 5201-5202 -P 2 -t 10 -b 10m -f m -i 1
   sleep 1
   ssh administrator@rs4410-52465.tail566708.ts.net 'type C:\iperf_srv.txt'
   # Assert: file > 1000 bytes, contains interval lines and SUM-2 summary
   ```

4. **Negative test (Finding B regression):** Confirm the server process does NOT die mid-test. Check `Get-Process iperf` at t=5s during a 10s test — process must still be present.

5. **Metrics to monitor:** After fix, all `RunForward`/`RunBidir` Windows SSH fallback tests should have `result.Intervals` length equal to `cfg.Duration / cfg.Interval`.

---

## Preventive Recommendations

1. **Assert file non-empty before parsing:** `readRemoteServerOutput` should check `len(strings.TrimSpace(output)) == 0` after reading and return a named error (not silently pass empty string to `ParseOutput`). This was already noted in the findings doc (`docs/tech/iperf2-udp-bidir-findings.md`, Recommendation 4) but is not yet implemented in `runner.go:544`.

2. **Add comment to `remoteServerStartCmd`:** Document why `cmd /c` blocking redirect is used instead of iperf's `-o` flag. Reference this report.

3. **Review gate:** Any future change that introduces `start /B iperf` or iperf's `-o` flag on Windows must be flagged in code review. Suggested inline comment:
   ```go
   // NOTE: Do NOT use 'start /B' or iperf's -o flag on Windows.
   // start /B detaches the console, causing premature exit and unflushed output.
   // Use 'cmd /c iperf.exe ... > file 2>&1' (blocking) in a goroutine instead.
   // See docs/tech/windows-ssh-server-output-finding.md
   ```

4. **Test the SSH fallback path in CI:** The SSH fallback is currently only exercised by manual tests. Add an integration test tag (`//go:build integration`) that can be run against a real Windows host when `IPERF_WINDOWS_HOST` env var is set.

---

## Open Questions / Missing Information

| Question | Why It Matters |
|---|---|
| Does `cmd /c iperf.exe ... > file 2>&1` (blocking) return when iperf exits normally (client disconnect) without needing an explicit kill? | If iperf exits after client disconnect and `cmd /c` returns, the goroutine terminates cleanly. If iperf hangs waiting for next connection, the goroutine blocks indefinitely and a kill is still needed. Must be tested. |
| Is the SSH client (`internal/ssh/client.go`) safe for concurrent `RunCommand` calls? | Required for the goroutine-based server start approach. If `RunCommand` uses a single channel/mutex, concurrent calls may deadlock. |
| Does `cmd /c` with `>` redirect on Windows flush all iperf output before returning? | The shell redirect is block-buffered. iperf2 calls `fflush(stdout)` in its normal exit path, which should be sufficient — but this must be confirmed on Windows 2.2.1. The live test confirmed it for a 10s run; behavior on abrupt client disconnect is untested. |
| What happens if the SSH goroutine for the blocking server command outlives the test context cancellation? | If `ctx` is cancelled (user stops test), the goroutine remains blocked on `sshCli.RunCommand`. A separate kill command must be issued first. The existing `killRemoteServer` / `defer` pattern must be audited for the new blocking-start approach. |
