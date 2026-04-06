# GUI vs CLI Modes

The iperf-tool automatically selects GUI or CLI mode based on command-line arguments.

## Mode Selection

### GUI Mode (default)
```bash
iperf-tool
```
No arguments → launches the Fyne graphical interface.

### CLI Mode
```bash
iperf-tool -s 192.168.1.1 -t 30
```
Any flags provided → runs headless in the terminal.

## Comparison

| Feature | GUI | CLI |
|---------|-----|-----|
| Interaction | Graphical forms | Command-line flags |
| Real-time output | Live interval widget | Interval lines to stdout |
| Test history | In-memory table | CSV file append |
| Persistent preferences | Yes (between restarts) | No (flags per invocation) |
| Remote server control | Connect/install/start/stop buttons | `--ssh` flags |
| UDP bidir / reverse | Supported | Supported |
| Pre-flight UDP probe | Automatic | Automatic |
| Batch / scripting | One test at a time | Loop-friendly |
| Headless environments | Not suitable | Fully supported |

## GUI Mode Features

- Configuration form with all iperf2 parameters (protocol, duration, streams, bandwidth, direction)
- Persistent preferences — form values saved between restarts
- Live interval display — bandwidth, loss, jitter updated each reporting interval
- Formatted summary on test completion (send/receive, jitter, loss per direction)
- History table of all past test results
- Remote panel — SSH connect, install iperf2, start/stop server
- Start/Stop test buttons with status feedback
- CSV + TXT export

## CLI Mode Features

- Flags for all test parameters
- SSH integration — install, start/stop remote iperf2, run remote client for reverse/bidir
- Pre-flight UDP probe automatically selects direct mode vs SSH fallback
- Repeat mode (`--repeat`, `--repeat-count`) for continuous monitoring
- Live interval output to stdout during test
- Verbose mode (`-v`) for probe status and extra messages
- Debug mode (`--debug`) logs raw iperf2 output to OS temp directory
- CSV + TXT export with `-o`

## Workflows

### Interactive testing (GUI)
1. Open app
2. Fill in server address, protocol, duration, streams
3. Optionally connect SSH for reverse/bidir tests
4. Click Start Test
5. Watch live intervals; review summary and history

### Automated testing (CLI)
```bash
# Single UDP bidir test
iperf-tool --ssh host --user ubuntu -s host -u --bidir -t 10 -b 20M -o results.csv

# Loop over multiple servers
for host in host1 host2 host3; do
  iperf-tool --ssh $host --user ubuntu -s $host -t 30 -o batch.csv
done

# Continuous monitoring with repeat
iperf-tool --ssh host --user ubuntu -s host -u --bidir -t 10 --repeat -o monitor.csv
```

## Technical Details

### Mode detection
1. Parse `os.Args`
2. No args or `help` → GUI mode (launches Fyne window)
3. Any flags → CLI mode

### CLI flag mapping
- Has `-s` only → local test (server must already be running)
- Has `--ssh` only → remote server management (install/start/stop)
- Has `--ssh` + `-s` → SSH connect, then run test (most common)
- Has `-R` or `--bidir` → requires `--ssh` to drive the remote client

### Shared components
Both modes use the same core engine:
- `internal/iperf` — `Runner`, `ParseOutput`, `ValidateServerReport`, `ProbeUDPReachability`
- `internal/ssh` — SSH connection and command execution
- `internal/format` — interval and result formatting
- `internal/model` — `TestResult`, `IntervalResult`
- `internal/export` — CSV/TXT writing

### UDP bidir mode selection
`Runner.RunBidir()` and `Runner.RunForward()` call `ProbeUDPReachability()` before starting the test:
- **Direct mode** (probe open): Server Report parsed from client stdout; `ValidateServerReport()` acts as secondary safety net
- **SSH fallback** (probe blocked/error): server output read from remote `-o` file via SSH after graceful kill + 500ms wait

In both modes, the remote server is started with `-o <file>` so that SSH file fallback is always available. If direct mode's Server Report is fabricated (0.000ms jitter — common under severe congestion, NAT, or Tailscale), the tool automatically falls back to reading the server output file via SSH.

## See Also

- [CLI.md](CLI.md) — Complete CLI flag reference and examples
- [INSTALLATION.md](INSTALLATION.md) — Remote iperf2 installation
- [docs/tech/iperf2-udp-bidir-findings.md](tech/iperf2-udp-bidir-findings.md) — UDP bidir implementation findings
