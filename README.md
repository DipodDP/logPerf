# iperf2 GUI Utility

A cross-platform Go application with **GUI and CLI** that wraps iperf2 for network performance testing. Run tests locally, control remote servers via SSH, measure latency alongside throughput, and export results to CSV.

## Features

**Dual Mode**
- **GUI** (Fyne): Interactive testing with real-time output and history
- **CLI**: Headless automation and scripting

**iperf2 Wrapping**
- Local client tests with configurable parameters (TCP/UDP, IPv4/IPv6)
- Live interval reporting — real-time bandwidth, transfer, and retransmit data at each reporting interval
- Reverse (`-R`) and bidirectional (`--bidir`) modes
- Per-stream target bandwidth for UDP and rate-limited TCP
- UDP server-side log fallback when the client-side Server Report is unreliable

**Latency Measurement**
- Optional ping integration (`--ping`) to capture baseline and in-test RTT
- Cross-platform ping implementation (native `ping` on Linux/macOS/Windows)

**Remote Server Management**
- SSH connection to remote hosts (key or password auth)
- Automatic iperf2 installation (Windows, Linux, macOS)
- Start/stop remote iperf2 servers in daemon mode
- Privilege verification (sudoers/administrators)
- Automated Windows OpenSSH server setup helpers

**Formatted Output**
- Per-stream throughput breakdown for parallel tests
- Human-readable summary with stream totals verification

**Data Export**
- CSV output with append mode for continuous logging
- TXT log export alongside CSV with formatted results
- Per-interval log CSV (`<name>_log.csv`) for fine-grained analysis
- Date automatically appended to output base path
- Excel-compatible format

**Repeat / Continuous Monitoring**
- Built-in `--repeat` loop with optional iteration count (`--repeat-count`)

**Preferences Persistence**
- Form values saved between app restarts (Fyne Preferences API)

**Cross-Platform**
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
iperf-tool -s 192.168.1.1 -t 30 -P 4 -o results
```

Install iperf2 and start a remote server:
```bash
iperf-tool --ssh remote.host --user ubuntu --key ~/.ssh/id_rsa --install --start-server
```

Test against a remote server:
```bash
iperf-tool --ssh remote.host --user ubuntu --key ~/.ssh/id_rsa \
  --start-server -s remote.host -t 30 -o results
```

Continuous monitoring:
```bash
iperf-tool -s 192.168.1.1 -t 10 --repeat --repeat-count 60
```

See [docs/CLI.md](docs/CLI.md) for full reference.

## Requirements

### Local Testing
- iperf2 installed and in PATH (binary name: `iperf`)
- Go 1.22+ (to build)

### GUI Mode
- CGO enabled
- Windows: MinGW or TDM-GCC
- Linux: gcc (usually pre-installed)
- macOS: Xcode Command Line Tools

### Remote Server Management
- SSH key or password
- Target has or can install iperf2 (automatic)
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

A standalone CLI-only binary is also available under `cmd/cli`:
```bash
go build -o iperf-cli ./cmd/cli
```

## Usage

### GUI
1. Run `iperf-tool` with no arguments
2. Fill in server address and parameters (values persist between restarts)
3. Click "Start Test"
4. View live interval measurements in the "Live Output" tab (bandwidth, transfer, retransmits per interval), followed by the full summary on completion
5. Check "History" tab for past results
6. Click "Export CSV" to save results (also creates a `.txt` file alongside)

### Remote Server (GUI)
1. Fill SSH connection details (host, username, key/password)
2. Click "Connect"
3. Click "Install iperf2" (if needed)
4. Click "Start Server"
5. Run local tests against the remote server
6. Click "Stop Server" when done

### CLI
```bash
# Show help
iperf-tool help

# Local test (required: -s for server address)
iperf-tool -s SERVER -p PORT -P STREAMS -t DURATION -o OUTPUT

# Remote server (required: --ssh for host)
iperf-tool --ssh HOST --user USER --key KEYPATH \
  --install --start-server -s HOST -t DURATION -o OUTPUT
```

Common flags:
- `-s/--server`, `-p/--port`, `-P/--parallel`, `-t/--time`, `-i/--interval`
- `-u` / `--protocol udp`, `-l/--block-size`, `-b/--bandwidth` (per-stream)
- `-R/--reverse`, `--bidir`, `-V/--ipv6`
- `--ping` to capture latency alongside throughput
- `--repeat`, `--repeat-count N`
- `-o/--output`, `-v/--verbose`, `--debug`

See [docs/CLI.md](docs/CLI.md) for the comprehensive flag reference and examples.

## Documentation

- [**MODES.md**](docs/MODES.md) — Understand GUI vs CLI, when to use each
- [**CLI.md**](docs/CLI.md) — Complete command-line reference with examples
- [**INSTALLATION.md**](docs/INSTALLATION.md) — Remote iperf2 setup details

## Architecture

```
┌─────────────────────────┐
│      main.go            │
│  (mode detection)       │
└────────┬────────────────┘
         │
    ┌────┴─────┐
    │          │
┌───▼──┐   ┌───▼──┐
│ GUI  │   │ CLI  │
│(Fyne)│   │      │
└──┬───┘   └──┬───┘
   │          │
   └────┬─────┘
        │
   ┌────▼────────────────────────┐
   │   Shared Core Engine         │
   ├──────────────────────────────┤
   │ • internal/iperf (runner)    │
   │ • internal/ssh (remote)      │
   │ • internal/ping (latency)    │
   │ • internal/netutil (helpers) │
   │ • internal/format (output)   │
   │ • internal/export (CSV/TXT)  │
   │ • internal/model (types)     │
   └──────────────────────────────┘
```

## Project Structure

```
iperf-tool/
├── main.go                    # Mode detection & entry point
├── go.mod / go.sum
├── cmd/
│   └── cli/                   # Standalone CLI-only binary
├── internal/
│   ├── cli/                   # CLI command parsing and execution
│   ├── iperf/                 # iperf2 runner and output parser
│   ├── ssh/                   # Remote SSH client and server manager
│   ├── ping/                  # Cross-platform ping / latency
│   ├── netutil/               # Network utilities
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

```bash
go test ./internal/... -v
```

Coverage includes:
- Config validation and argument generation
- iperf2 output parsing (TCP, UDP, parallel streams, intervals)
- UDP server-side log fallback
- Stream event parsing (start, interval, end events)
- Interval formatting
- CSV export format (summary and interval logs)
- SSH connection and remote operations
- Ping / latency measurement
- CLI flag parsing
- Remote OS detection and package manager selection

## Examples

### One-time test
```bash
iperf-tool -s 192.168.1.1 -t 30
```

### Continuous monitoring with built-in repeat
```bash
iperf-tool -s server.example.com -t 60 --repeat -o perf
```

### Continuous monitoring (shell loop)
```bash
while true; do
  iperf-tool -s server.example.com -t 60 -o perf
  sleep 300  # Every 5 minutes
done
```

### Multi-server batch test
```bash
for server in 10.0.0.{1..10}; do
  iperf-tool -s $server -t 30 -P 4 -o batch
done
```

### Throughput with latency
```bash
iperf-tool -s 192.168.1.1 -t 30 --ping -o results
```

### Remote server automation
```bash
# Install and setup
iperf-tool --ssh test-server --user root --key key.pem \
  --install --start-server

# Run test
iperf-tool --ssh test-server --user root --key key.pem \
  -s test-server -t 30 -o remote_test

# Cleanup
iperf-tool --ssh test-server --user root --key key.pem --stop-server
```

## Security

- **SSH keys**: Primary auth method; passwords supported but discouraged
- **Input validation**: Whitelist characters for hostnames and ports
- **Privilege checking**: Verifies sudo/admin before attempting install
- **No credentials in code**: Keys loaded from files, never embedded
- **Host verification**: Uses `~/.ssh/known_hosts` when available

## Limitations

- **Single test at a time** (GUI locks during test execution)
- **iperf2 only** (not iperf3 — a prior migration rewrote the backend)
- **Installed binary required** (no built-in iperf2)
- **One remote server per connection** (separate SSH sessions needed for multiple)
- **UDP Server Report** can be unreliable on some platforms; the tool falls back to reading the server-side log file over SSH

## Future Enhancements

- [ ] Parallel test execution (CLI mode)
- [ ] Test scheduling/cron integration
- [ ] Graphical result analysis (charts, trends)
- [ ] Password-less sudo detection/setup
- [ ] Docker/container image

## Troubleshooting

### "iperf command not found"
Install iperf2 locally or use `--install` for remote servers.

### "SSH connect: permission denied"
Check username, key path, and host accessibility.

### "requires sudo/administrator privileges"
User must be in sudoers (Linux/macOS) or Administrator group (Windows).

### GUI doesn't open
Ensure CGO is enabled and build tools are installed.

### CLI test shows no output
Use `-v` for verbose output, or `--debug` to log raw iperf2 output to `/tmp/iperf-debug.log` (GUI: set `IPERF_DEBUG=1`).

### UDP results look wrong
The client-side Server Report for UDP can be fabricated on some iperf2 builds. The tool automatically starts the remote server with an output file and reads it back over SSH as a fallback.

## License

[Specify your license here]

## Contributing

[Contribution guidelines]

## Support

For issues, feature requests, or questions:
- Check [docs/](docs/) for detailed documentation
- Review [CLI.md](docs/CLI.md) for command reference
- See [INSTALLATION.md](docs/INSTALLATION.md) for setup help
