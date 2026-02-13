# Command-Line Interface (CLI)

The iperf3 Test Tool supports full command-line operation for scripting, automation, and headless environments. Run with no arguments to launch the GUI, or provide flags for CLI mode.

## Quick Start

### Run a local test
```bash
iperf-tool -c 192.168.1.1 -t 30 -P 4 -o results.csv
```

### Test against a remote server
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa \
  -start-server -c remote.host -t 30 -o results.csv
```

### Install iperf3 on remote host
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -install
```

## Usage

### No arguments → GUI mode
```bash
iperf-tool
```
Launches the Fyne graphical interface.

### Help
```bash
iperf-tool help
iperf-tool --help
iperf-tool -h
```

## Flags

### Local Test (required: `-c`)

| Flag | Long Form | Description | Default |
|------|-----------|-------------|---------|
| `-c` | `-connect` | Server address (IP or hostname) | - |
| `-p` | `-port` | Server port | 5201 |
| `-P` | `-parallel` | Number of parallel streams | 1 |
| `-t` | `-time` | Test duration in seconds | 10 |
| `-i` | `-interval` | Reporting interval in seconds | 1 |
| `-u` | - | Protocol mode (`tcp` or `udp`) | tcp |
| `-binary` | - | Path to iperf3 binary | iperf3 |

### Remote Server (required: `-ssh`)

| Flag | Description | Default |
|------|-------------|---------|
| `-ssh` | SSH host to manage remote iperf3 | - |
| `-user` | SSH username | `$USER` environment variable |
| `-key` | SSH private key path | - |
| `-password` | SSH password (insecure) | - |
| `-ssh-port` | SSH port | 22 |
| `-install` | Install iperf3 on remote host | false |
| `-start-server` | Start remote iperf3 server | false |
| `-stop-server` | Stop remote iperf3 server | false |

### Output

| Flag | Long Form | Description | Default |
|------|-----------|-------------|---------|
| `-o` | `-output` | Save results to CSV file | - |
| `-v` | `-verbose` | Verbose output (show live iperf3 output) | false |

## Examples

### 1. Simple local test
```bash
iperf-tool -c 192.168.1.1
```
Runs a 10-second test to 192.168.1.1:5201 with 1 parallel stream.

### 2. Extended test with multiple streams
```bash
iperf-tool -c server.example.com -t 60 -P 8 -o results.csv
```
60-second test with 8 parallel streams, saves results to CSV.

### 3. UDP test
```bash
iperf-tool -c 10.0.0.1 -u udp -t 20 -P 4
```
20-second UDP test with 4 parallel streams.

### 4. Test with verbose output
```bash
iperf-tool -c 192.168.1.1 -t 30 -v
```
Prints live iperf3 output as it runs.

### 5. Install iperf3 and start server
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa \
  -install -start-server -p 5201
```
Automatically installs iperf3 (if needed) and starts the server on port 5201.

### 6. Run test against remote server
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa \
  -start-server -c remote.host -t 30 -P 4 -o results.csv
```
Starts remote server, then runs a 30-second test with 4 streams from the local machine.

### 7. Stop remote server
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -stop-server
```
Stops the remote iperf3 server.

### 8. Combine install + start + test + stop
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa \
  -install -start-server -c remote.host -t 10 -v -o test.csv && \
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -stop-server
```
Full workflow: install, start, run test, output results. Stop in separate command to reuse connection.

## Authentication

### Using SSH Key (recommended)
```bash
iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -install
```

### Using SSH Password (not recommended)
```bash
iperf-tool -ssh remote.host -user ubuntu -password "mypass" -install
```
⚠️ **Warning**: Passing passwords on the command line is insecure and visible in process listings. Use SSH keys instead.

## Remote Installation

When you use `-install`, the tool automatically:

1. **Detects the remote OS** (Linux, macOS, or Windows)
2. **Finds available package manager** (apt, yum, dnf, apk, pacman, brew, chocolatey, winget)
3. **Verifies sudo/admin privileges** (requires passwordless sudo or UAC elevation)
4. **Installs iperf3** with appropriate commands
5. **Verifies installation** by checking if iperf3 is in PATH

Supported platforms:
- **Linux**: Debian/Ubuntu (apt), RHEL/CentOS (yum/dnf), Alpine (apk), Arch (pacman)
- **macOS**: Homebrew
- **Windows**: Chocolatey or WinGet

See [INSTALLATION.md](INSTALLATION.md) for details.

## Output

### Results Format

Test results are printed to stdout in human-readable format with per-stream breakdown (when using multiple parallel streams):

```
=== Test Results ===
Timestamp:       2024-01-15 14:30:45
Server:          192.168.1.1:5201
Protocol:        TCP
Parallel:        4 streams
Duration:        30 seconds

--- Per-Stream Results ---
Stream 1:  Sent: 235.12 Mbps  Received: 234.06 Mbps
Stream 2:  Sent: 235.13 Mbps  Received: 234.06 Mbps
Stream 3:  Sent: 235.12 Mbps  Received: 234.06 Mbps
Stream 4:  Sent: 235.13 Mbps  Received: 234.07 Mbps

--- Summary ---
Sent:            940.50 Mbps
Received:        936.25 Mbps
Retransmits:     42
====================
```

Per-stream section is only shown when using more than 1 parallel stream. A warning is displayed if per-stream totals don't match the summary values.

### CSV Export

With `-o results.csv`, results are appended to a CSV file:

```csv
Timestamp,Server,Port,Parallel,Duration,Protocol,Sent_Mbps,Received_Mbps,Retransmits,Error
2024-01-15 14:30:45,192.168.1.1,5201,4,30,TCP,940.50,936.25,42,
```

Multiple test runs append to the same file, creating a historical log.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (invalid flags, test failure, etc.) |

## Scripting Examples

### Batch testing multiple servers
```bash
#!/bin/bash
servers=("192.168.1.1" "192.168.1.2" "192.168.1.3")
for server in "${servers[@]}"; do
  iperf-tool -c "$server" -t 30 -o batch_results.csv
done
```

### Daily performance monitoring
```bash
#!/bin/bash
timestamp=$(date +%Y%m%d_%H%M%S)
iperf-tool -c production.server \
  -t 60 -P 8 \
  -o "perf_${timestamp}.csv" -v
```

### Remote server management
```bash
#!/bin/bash
host="test-server.internal"
user="ec2-user"
key="/path/to/key.pem"

# Install and prepare
iperf-tool -ssh "$host" -user "$user" -key "$key" -install

# Run periodic tests
for i in {1..5}; do
  echo "Test run $i..."
  iperf-tool -ssh "$host" -user "$user" -key "$key" \
    -start-server -c "$host" -t 30 -o results.csv
  sleep 60
done

# Cleanup
iperf-tool -ssh "$host" -user "$user" -key "$key" -stop-server
```

## Troubleshooting

### "must provide -c <server> or -ssh <host>"
You need either a server address for local test or SSH host for remote operations.

### "invalid config: port must be between 1 and 65535"
Check that `-p` value is a valid port number.

### "SSH connect: permission denied"
Verify SSH credentials (username, key path, password). Check if user has access to the host.

### "requires sudo/administrator privileges to install iperf3"
The remote user must be in sudoers (Linux/macOS) or have Administrator privileges (Windows).

### "iperf3 command not found"
Make sure iperf3 is installed on the local machine or remote server. Use `-install` flag for remote servers.

### "no supported package manager found"
The remote system's package manager is not supported. Install iperf3 manually or check supported OS list.

## Advanced

### Custom iperf3 binary
If you've compiled iperf3 from source or have a custom binary:
```bash
iperf-tool -c server -binary /opt/custom/iperf3
```

### Non-standard SSH port
```bash
iperf-tool -ssh remote.host -ssh-port 2222 -user ubuntu -key ~/.ssh/id_rsa -install
```

### Combining multiple remote operations
The tool allows chaining operations: install, then start server, then run test from same connection.
```bash
iperf-tool -ssh myhost -user root -key key.pem \
  -install -start-server -c myhost -t 30 -o results.csv
```
All operations happen in order, then the SSH connection is closed.

## See Also

- [INSTALLATION.md](INSTALLATION.md) — Remote iperf3 installation details
- [README.md](../README.md) — General application documentation
