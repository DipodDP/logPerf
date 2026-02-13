# iperf3 GUI Utility

A cross-platform Go application with **GUI and CLI** that wraps iperf3 for network performance testing. Run tests locally, control remote servers via SSH, and export results to CSV.

## Features

✅ **Dual Mode**
- **GUI** (Fyne): Interactive testing with real-time output and history
- **CLI**: Headless automation and scripting

✅ **iperf3 Wrapping**
- Local client tests with configurable parameters
- JSON output parsing
- Live streaming of results

✅ **Remote Server Management**
- SSH connection to remote hosts
- Automatic iperf3 installation (Windows, Linux, macOS)
- Start/stop remote iperf3 servers in daemon mode
- Privilege verification (sudoers/administrators)

✅ **Formatted Output**
- Per-stream throughput breakdown for parallel tests
- Human-readable summary with stream totals verification

✅ **Data Export**
- CSV output with append mode for continuous logging
- TXT log export alongside CSV with formatted results
- Excel-compatible format

✅ **Preferences Persistence**
- Form values saved between app restarts (Fyne Preferences API)

✅ **Cross-Platform**
- Windows, Linux, macOS
- Native binaries (CGO required for GUI on Windows)

## Quick Start

### GUI Mode (Default)
```bash
iperf-tool
```
Opens interactive window with form, live output, and history table.

### CLI Mode (Examples)

Run a test:
```bash
iperf-tool -c 192.168.1.1 -t 30 -P 4 -o results.csv
```

Install iperf3 and start remote server:
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -install -start-server
```

Test against remote server:
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa \
  -start-server -c remote.host -t 30 -o results.csv
```

See [docs/CLI.md](docs/CLI.md) for full reference.

## Requirements

### Local Testing
- iperf3 installed and in PATH
- Go 1.22+ (to build)

### GUI Mode
- CGO enabled
- Windows: MinGW or TDM-GCC
- Linux: gcc (usually pre-installed)
- macOS: Xcode Command Line Tools

### Remote Server Management
- SSH key or password
- Target has or can install iperf3 (automatic)
- User with sudo/admin privileges (for install)

## Building

```bash
# Build binary
go build -o iperf-tool

# Run tests
go test ./internal/...

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o iperf-tool.exe
```

## Usage

### GUI
1. Run `iperf-tool` with no arguments
2. Fill in server address and parameters (values persist between restarts)
3. Click "Start Test"
4. View live output in "Live Output" tab — formatted with per-stream data on completion
5. Check "History" tab for past results
6. Click "Export CSV" to save results (also creates a `.txt` file alongside)

### Remote Server (GUI)
1. Fill SSH connection details (host, username, key/password)
2. Click "Connect"
3. Click "Install iperf3" (if needed)
4. Click "Start Server"
5. Run local tests against the remote server
6. Click "Stop Server" when done

### CLI
```bash
# Show help
iperf-tool help

# Local test (required: -c for server address)
iperf-tool -c SERVER -p PORT -P STREAMS -t DURATION -o OUTPUT.csv

# Remote server (required: -ssh for host)
iperf-tool -ssh HOST -user USER -key KEYPATH \
  -install -start-server -c HOST -t DURATION -o OUTPUT.csv
```

See [docs/CLI.md](docs/CLI.md) for comprehensive flag reference and examples.

## Documentation

- [**MODES.md**](docs/MODES.md) — Understand GUI vs CLI, when to use each
- [**CLI.md**](docs/CLI.md) — Complete command-line reference with examples
- [**INSTALLATION.md**](docs/INSTALLATION.md) — Remote iperf3 setup details

## Architecture

```
┌─────────────────────────┐
│      main.go            │
│  (mode detection)       │
└────────┬────────────────┘
         │
    ┌────┴─────┐
    │           │
┌───▼──┐   ┌───▼──┐
│ GUI  │   │ CLI  │
│(Fyne)│   │      │
└──┬───┘   └──┬───┘
   │          │
   └────┬─────┘
        │
   ┌────▼──────────────────────┐
   │   Shared Core Engine       │
   ├────────────────────────────┤
   │ • internal/iperf (runner)  │
   │ • internal/ssh (remote)    │
   │ • internal/format (output) │
   │ • internal/export (CSV/TXT)│
   │ • internal/model (types)   │
   └────────────────────────────┘
```

## Project Structure

```
iperf-tool/
├── main.go                    # Mode detection & entry point
├── go.mod / go.sum
├── internal/
│   ├── cli/                   # CLI command parsing and execution
│   ├── iperf/                 # iperf3 runner and JSON parser
│   ├── ssh/                   # Remote SSH client and server manager
│   ├── format/                # Result formatter
│   ├── export/                # CSV and TXT writers
│   └── model/                 # Shared data types
├── ui/                        # Fyne GUI components
├── docs/
│   ├── MODES.md               # GUI vs CLI comparison
│   ├── CLI.md                 # CLI reference
│   └── INSTALLATION.md        # Remote setup details
└── README.md
```

## Testing

24 tests across all internal packages:
```bash
go test ./internal/... -v
```

Coverage includes:
- Config validation and argument generation
- iperf3 JSON parsing with fixtures
- CSV export format
- SSH connection and remote operations
- CLI flag parsing
- Remote OS detection and package manager selection

## Examples

### One-time test
```bash
iperf-tool -c 192.168.1.1 -t 30
```

### Continuous monitoring (loop)
```bash
while true; do
  iperf-tool -c server.example.com -t 60 -o perf.csv
  sleep 300  # Every 5 minutes
done
```

### Multi-server batch test
```bash
for server in 10.0.0.{1..10}; do
  iperf-tool -c $server -t 30 -P 4 -o batch.csv
done
```

### Remote server automation
```bash
# Install and setup
iperf-tool -ssh test-server -user root -key key.pem \
  -install -start-server

# Run test (in separate invocation to reuse connection if needed)
iperf-tool -ssh test-server -user root -key key.pem \
  -c test-server -t 30 -o remote_test.csv

# Cleanup
iperf-tool -ssh test-server -user root -key key.pem -stop-server
```

## Security

- **SSH keys**: Primary auth method, passwords supported but discouraged
- **Input validation**: Whitelist characters for hostnames and ports
- **Privilege checking**: Verifies sudo/admin before attempting install
- **No credentials in code**: Keys loaded from files, never embedded
- **Host verification**: Uses `~/.ssh/known_hosts` when available

## Limitations

- **Single test at a time** (GUI locks during test execution)
- **iperf3 only** (not iperf2)
- **Installed binary required** (no built-in iperf3)
- **One remote server per connection** (separate SSH sessions needed for multiple)

## Future Enhancements

- [ ] Parallel test execution (CLI mode)
- [ ] Test scheduling/cron integration
- [ ] Graphical result analysis (charts, trends)
- [ ] Support for other test tools (netperf, iperf2)
- [ ] Password-less sudo detection/setup
- [ ] Docker/container image

## Troubleshooting

### "iperf3 command not found"
Install iperf3 locally or use `-install` for remote servers.

### "SSH connect: permission denied"
Check username, key path, and host accessibility.

### "requires sudo/administrator privileges"
User must be in sudoers (Linux/macOS) or Administrator group (Windows).

### GUI doesn't open
Ensure CGO is enabled and build tools are installed.

### CLI test shows no output
Use `-v` flag for verbose output: `iperf-tool -c server -v`

## License

[Specify your license here]

## Contributing

[Contribution guidelines]

## Support

For issues, feature requests, or questions:
- Check [docs/](docs/) for detailed documentation
- Review [CLI.md](docs/CLI.md) for command reference
- See [INSTALLATION.md](docs/INSTALLATION.md) for setup help
