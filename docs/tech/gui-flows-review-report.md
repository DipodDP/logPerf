# Code Review Finding Report

## Summary

Multiple defects were discovered in the GUI and CLI layers of `iperf-tool` that caused reverse and bidirectional test modes to be broken or produce incomplete results. The issues fall into three categories:

1. **Missing config wiring** — GUI never set `LocalAddr`, `IsWindows`, or `RemoteOutputFile` on the iperf config, causing reverse/bidir tests to fail silently or produce wrong commands.
2. **Nil pointer dereference** — `FormatBidirInterval` dereferenced `fwd` unconditionally, panicking when the reverse direction had more intervals than forward.
3. **Missing byte-count propagation** — `MergeBidirResults` did not copy `BytesReceived` or `ReverseBytesReceived` from server-side parse results, causing "0.00 MB" in transferred summaries.
4. **Missing CLI dispatch** — Bidirectional mode without SSH always called `RunBidir` (which requires SSH) instead of the new `RunBidirDualtest`.

---

## Context

- **Project:** iperf-tool — a Go GUI/CLI wrapper around iperf2 for network performance testing.
- **Branch:** `multistream-iperf2`
- **Language:** Go 1.x, Fyne v2 GUI framework.
- **Scope:** The backend (`internal/iperf/`) had been updated for iperf2 migration including reverse, bidir, and UDP probe support. The GUI layer (`ui/`) and CLI layer (`internal/cli/`) had not been updated to pass new required config fields, and the result-merge logic had incomplete field propagation.

---

## Detailed Findings

### Finding 1: GUI does not set `LocalAddr`, `IsWindows`, or `RemoteOutputFile`

**File:** `ui/controls.go` — `onStart()` method, line ~145

The `ConfigForm.Config()` method builds an `IperfConfig` struct but never sets `LocalAddr`, `IsWindows`, or `RemoteOutputFile`. These fields are required by the runner for reverse and bidirectional tests:

- `LocalAddr` is used in `revClientCmd()` to build the remote client command (`iperf -c <LocalAddr>`). When empty, the command becomes `iperf -c  -p 5202` — the remote client has no target address.
- `IsWindows` determines whether to use `iperf.exe` vs `iperf` and `start /B` vs `nohup` in remote commands. Defaults to `false`, producing Unix commands on Windows hosts.
- `RemoteOutputFile` is required when `SSHFallback` is enabled (set dynamically by the UDP probe). The `Validate()` method rejects configs where `SSHFallback=true` and `RemoteOutputFile=""`.

**Observed behavior:** Reverse and bidir tests via the GUI silently produce invalid remote commands or fail validation after the probe enables SSH fallback.

### Finding 2: `FormatBidirInterval` nil `fwd` panic

**File:** `internal/format/result.go` — `FormatBidirInterval()`, line ~45

The function's doc comment states "rev may be nil" but the implementation unconditionally calls `fwd.BandwidthMbps()`, `fwd.TransferMB()`, and `fwd.Retransmits`. In `RunBidir`, the post-test interval replay iterates up to `max(len(fwdIvs), len(revIvs))`, passing `nil` for `fwd` when the reverse direction has more intervals than forward. This causes a nil pointer dereference panic.

**Observed behavior:** Bidir tests crash when the reverse direction produces more interval samples than the forward direction.

### Finding 3: `MergeBidirResults` does not propagate byte counts

**File:** `internal/iperf/parser.go` — `MergeBidirResults()`, lines ~598–664

The function copies bandwidth (`ReverseReceivedBps`), jitter, loss, and intervals from server-side results, but omits:

- `result.BytesReceived = fwdServer.BytesReceived` (forward server-measured bytes)
- `result.ReverseBytesReceived = revServer.BytesReceived` (reverse server-measured bytes)

The `formatBidirTransferred()` helper and `TotalRevMB()` method rely on these fields. When they are zero, the output reads `S→C transferred: 0.00 MB received` despite the reverse direction having real throughput data.

**Observed behavior:** Bidir test results display "0.00 MB" for transferred bytes in both C→S received and S→C received columns, even when actual data was transferred.

### Finding 4: CLI bidir dispatch missing for no-SSH dualtest

**File:** `internal/cli/runner.go` — `LocalTestRunner()`, line ~172

After adding `RunBidirDualtest()` for the iperf2 `-d` flag mode, the CLI dispatch still called `runner.RunBidir(ctx, iperfCfg, cfg.SSHClient, onInterval)` unconditionally for bidir. When `SSHClient` is nil (no SSH), `RunBidir` returns the error "SSH connection required for SSH-controlled bidirectional tests".

**Observed behavior:** `iperf-cli -s <host> --bidir` (without `--ssh`) fails with an SSH-required error instead of using dualtest mode.

### Finding 5: Probe and status messages printed to stdout/stderr

**File:** `internal/iperf/runner.go` — `RunForward()`, `RunReverse()`, `RunBidir()`

Probe results and status messages used `fmt.Println` and `fmt.Fprintf(os.Stderr, ...)`. In CLI mode this is acceptable, but in GUI mode these messages are invisible to the user — they go to the process stdout/stderr, not the GUI output view.

**Observed behavior:** UDP probe results ("open" / "blocked") and status messages are not visible in the GUI.

---

## Root Cause Analysis

| Finding | Root Cause |
|---------|-----------|
| F1 | The GUI config builder (`ConfigForm.Config()`) was written before reverse/bidir support existed. The SSH-derived fields (`LocalAddr`, `IsWindows`) require access to the SSH client, which `ConfigForm` does not have — they must be injected by the dispatch layer (`controls.go`). |
| F2 | The `FormatBidirInterval` function was written assuming `fwd` is always present. The bidir replay loop correctly handles uneven interval counts by passing `nil`, but the formatter did not. |
| F3 | `MergeBidirResults` was modeled after `MergeUnidirResults`, which only merges bandwidth and UDP stats. Byte counts were added to `TestResult` later and were not included in the merge function. |
| F4 | `RunBidirDualtest` was a new method added to `runner.go`. The GUI dispatch in `controls.go` was updated, but the CLI dispatch in `cli/runner.go` was not. |
| F5 | The runner was originally CLI-only, where stdout/stderr is the natural output channel. GUI integration was added later without a callback mechanism for status messages. |

---

## Impact Assessment

| Finding | Severity | Impact |
|---------|----------|--------|
| F1: Missing config fields | **Critical** | Reverse and bidir tests completely broken in GUI. Remote client receives empty address, produces invalid command. Windows hosts get Unix-style commands. |
| F2: Nil fwd panic | **High** | Application crash during bidir test display. Occurs non-deterministically when network asymmetry causes uneven interval counts. |
| F3: Missing byte counts | **Medium** | Incorrect "0.00 MB" in result summaries. Data loss in CSV/TXT exports. Misleading test reports. |
| F4: CLI dispatch | **High** | `--bidir` without `--ssh` completely non-functional in CLI. Blocks a primary user flow. |
| F5: Invisible probe messages | **Low** | GUI users don't see probe results. Doesn't affect correctness, but reduces observability and trust. |

---

## Proposed Resolutions

### Recommended Solution (Applied)

**F1 — Wire SSH-derived config fields in `controls.go`:**

In `onStart()`, after `cfg := c.configForm.Config()` and before `cfg.Validate()`, inject SSH-derived fields:

```go
if c.remotePanel.IsConnected() {
    cfg.LocalAddr = c.remotePanel.LocalAddr()
    cfg.IsWindows = c.remotePanel.IsWindows()
    if cfg.RemoteOutputFile == "" {
        if cfg.IsWindows {
            cfg.RemoteOutputFile = `C:\iperf2_server_output.txt`
        } else {
            cfg.RemoteOutputFile = "/tmp/iperf2_server_output.txt"
        }
    }
}
```

New methods added to `RemotePanel`: `IsWindows() bool`, `LocalAddr() string`.

Also set `RemoteOutputFile` in the runner when the probe dynamically enables `SSHFallback`.

**F2 — Guard nil `fwd` in `FormatBidirInterval`:**

Extract `fwd` fields into local variables with zero defaults before use, mirroring the existing `rev` nil guard.

**F3 — Copy byte counts in `MergeBidirResults`:**

Add two lines:

```go
result.BytesReceived = fwdServer.BytesReceived       // forward server-measured
result.ReverseBytesReceived = revServer.BytesReceived // reverse server-measured
```

Same fix applied to `ParseDualtestOutput` for the dualtest path.

**F4 — CLI bidir dispatch:**

```go
if iperfCfg.Bidir {
    if cfg.SSHClient == nil {
        result, err = runner.RunBidirDualtest(ctx, iperfCfg, onInterval)
    } else {
        result, err = runner.RunBidir(ctx, iperfCfg, cfg.SSHClient, onInterval)
    }
}
```

**F5 — Status callback on Runner:**

Added `StatusCallback` type and `SetStatusCallback()` method. All `fmt.Println`/`fmt.Fprintf(os.Stderr)` probe messages replaced with `r.logStatus()` which invokes the callback (GUI) and falls back to stderr (CLI).

### Alternative Solutions

- **F1 alternative:** Pass the SSH client directly to `ConfigForm.Config()`. Rejected because it couples the form widget to the SSH layer.
- **F3 alternative:** Compute byte counts from bandwidth × duration in the formatter. Rejected because it's an approximation, not actual measured data.

---

## Verification Plan

### Automated Tests Added

| Test | File | Validates |
|------|------|-----------|
| `TestFormatBidirInterval_NilFwd_TCP` | `format/result_test.go` | F2: no panic when `fwd=nil` (TCP) |
| `TestFormatBidirInterval_NilFwd_UDP` | `format/result_test.go` | F2: no panic when `fwd=nil` (UDP) |
| `TestFormatResultUDPFabricatedServerReport` | `format/result_test.go` | U3: fabricated report shows "N/A" |
| `TestDualtestClientArgs_TCP` | `config_test.go` | N2: `-d` flag present in dualtest args |
| `TestDualtestClientArgs_UDP` | `config_test.go` | N2: `-d` + `-u` + `-b` for UDP dualtest |
| `TestParseDualtestOutput_TCP` | `parser_test.go` | N3: two-stream dualtest parsed correctly |
| `TestParseDualtestOutput_EmptyInput` | `parser_test.go` | N3: empty input returns error |
| `TestParseDualtestOutput_ForwardReverseInterleaved` | `parser_test.go` | N3: stream ID separation verified per-interval |

### Manual CLI Verification (Performed)

| Flow | Command | Result |
|------|---------|--------|
| Forward TCP + SSH | `--ssh <host> --user administrator -s <host> -t 3` | Pass — real-time intervals, summary |
| Reverse TCP + SSH | `--ssh <host> --user administrator -s <host> -t 3 -R` | Pass — LocalAddr wired, 20.6 Mbps received |
| Bidir TCP + SSH | `--ssh <host> --user administrator -s <host> -t 3 --bidir` | Pass — both directions, S→C bytes now correct |
| Bidir TCP dualtest | `-s <host> -t 3 --bidir` (no SSH) | Pass — `-d` flag, both directions shown |
| Forward UDP + SSH | `--ssh <host> --user administrator -s <host> -t 3 -u -b 10M` | Pass — probe message in output |
| Bidir UDP + SSH | `--ssh <host> --user administrator -s <host> -t 3 -u -b 10M --bidir` | Pass — jitter and loss for both directions |

### Recommended Post-Merge Monitoring

- Run bidir tests on Windows remote hosts to verify `IsWindows` correctness.
- Test bidir with `parallel > 1` to verify server instance count calculation (`parallel * 2`).
- Test repeat mode with reverse/bidir to verify multi-run stability.

---

## Preventive Recommendations

1. **Integration test for config wiring:** Add a test that constructs a `Controls` with a mock `RemotePanel` and verifies the config passed to the runner includes `LocalAddr`, `IsWindows`, and `RemoteOutputFile`.
2. **Nil-safety linting:** Consider `nilaway` or similar static analysis to catch nil dereferences in formatter functions.
3. **Field-completeness check in merge functions:** When new fields are added to `TestResult`, the merge functions (`MergeBidirResults`, `MergeUnidirResults`) must be audited. A comment listing the expected field mapping would help reviewers.
4. **Dispatch symmetry:** GUI and CLI dispatch blocks should mirror each other. When a new runner method is added, both dispatch sites must be updated. A shared dispatch function or table could eliminate this duplication.
5. **Status message routing:** All runner output should go through the `StatusCallback` mechanism, never directly to `fmt.Println`. A linting rule flagging `fmt.Print` in `runner.go` would catch regressions.
