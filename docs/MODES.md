# GUI vs CLI Modes

The iperf3 Test Tool automatically detects whether to run in GUI mode or CLI mode based on command-line arguments.

## Mode Selection

### GUI Mode (Default)
```bash
iperf-tool
```
No arguments → launches Fyne graphical interface with full feature set.

### CLI Mode
```bash
iperf-tool -c 192.168.1.1 -t 30
```
Any flags provided → runs in headless command-line mode.

## Comparison

| Feature | GUI | CLI |
|---------|-----|-----|
| **Interaction** | Graphical forms | Command-line flags |
| **Real-time output** | Scrollable text widget with live interval data | Live interval lines printed to stdout |
| **Test history** | In-memory table | CSV file append |
| **Preferences** | Persistent between restarts | N/A (flags per invocation) |
| **Per-stream data** | Formatted on completion | Formatted on completion |
| **Remote server** | Connect/install/start/stop buttons | Command composition |
| **Ease of use** | Intuitive for humans | Ideal for scripting/automation |
| **Batch testing** | One test at a time | Loop-friendly |
| **Cross-platform** | Requires CGO (Windows needs MinGW) | Pure CLI, CGO-free possible |
| **Headless environments** | Not suitable | Fully supported |

## GUI Mode Features

- **Configuration form** with all iperf3 parameters
- **Persistent preferences** — form values saved between app restarts
- **Live interval display** — real-time bandwidth, transfer, and retransmit data at each reporting interval (iperf3 3.17+)
- **Formatted results** — per-stream throughput breakdown appended on test completion
- **History table** displaying all past test results
- **Remote server panel** for SSH management (install, start, stop)
- **One-click operations** (Start Test, Stop Test, Export CSV)
- **Dual export** — CSV and TXT files created together
- **Visual status indicators**

→ See [GUI usage](../README.md) for details

## CLI Mode Features

- **Flags for all parameters** (server, port, protocol, duration, etc.)
- **SSH integration** (install, start, stop remote servers)
- **Batch automation** (run multiple tests in loops)
- **Scripting-friendly** (exit codes, CSV output, no UI blocking)
- **Live interval output** (bandwidth/transfer/retransmits per interval with iperf3 3.17+)
- **Verbose mode** (additional status messages with `-v`)
- **Remote operations** (install iperf3, manage servers)

→ See [CLI.md](CLI.md) for complete flag reference and examples

## Workflows

### Interactive Testing (GUI)
1. Open app
2. Fill form with test parameters
3. Click "Start Test"
4. View live output and historical results
5. Export results to CSV

### Automated Testing (CLI)
```bash
# Single test
iperf-tool -c server -t 30 -o results.csv

# Loop over servers
for server in server1 server2 server3; do
  iperf-tool -c $server -t 30 -o results.csv
done

# Complex workflow
iperf-tool -ssh host1 -user root -key key.pem \
  -install -start-server -c host1 -t 30 -o test.csv
```

### Hybrid Approach
Run CLI tests to gather data, then open GUI to review results:
```bash
# Batch testing
iperf-tool -c 192.168.1.1 -t 30 -o batch.csv
iperf-tool -c 192.168.1.2 -t 30 -o batch.csv

# View results in GUI
iperf-tool  # Open GUI and import batch.csv
```

## Which Mode to Use?

| Scenario | Use |
|----------|-----|
| One-off testing | GUI |
| Repeated tests to same server | GUI (history) |
| Multiple servers/scheduling | CLI |
| Integration with other tools | CLI |
| Performance monitoring | CLI + cron/systemd |
| CI/CD pipelines | CLI |
| Network troubleshooting | GUI (visual feedback) |
| Large-scale testing | CLI (scriptable) |

## Technical Details

### Mode Detection Logic

1. Parse `os.Args`
2. If no args or `help` → GUI mode
3. If any flags detected → CLI mode
4. CLI flags determine operation type:
   - Has `-c` → local test
   - Has `-ssh` → remote server management
   - Both → start remote, then test locally

### shared Components

Both modes use the same core engine:
- `internal/iperf` — runner and parser (supports both `-J` and `--json-stream` modes)
- `internal/ssh` — remote control
- `internal/export` — CSV writing (summary + interval logs)
- `internal/format` — result and interval formatting
- `internal/model` — result structs (TestResult, IntervalResult, StreamResult)

### Independence

- **GUI** imports `ui/` package, Fyne library
- **CLI** imports `internal/cli/` package, standard library only
- Build supports both simultaneously

## Running in Container/Headless

For servers without X11 or Wayland:

```bash
# CLI only (no GUI dependencies)
iperf-tool -c server -t 30 -o results.csv

# Not available
iperf-tool  # Would fail without display
```

## Performance

- **GUI**: ~33 MB binary size, depends on Fyne/CGO
- **CLI**: Pure Go CLI subset could be optimized separately
- **Runtime**: Same core engine, no performance difference for tests

## See Also

- [CLI.md](CLI.md) — Comprehensive CLI flag reference
- [INSTALLATION.md](INSTALLATION.md) — Remote iperf3 setup
- [README.md](../README.md) — General overview
