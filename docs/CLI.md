# Command-Line Interface (CLI)

The iperf-tool supports full command-line operation for scripting, automation, and headless environments. Run with no arguments to launch the GUI, or provide flags for CLI mode.

## Quick Start

```bash
# Forward test (local → remote server)
iperf-tool --ssh remote.host --user ubuntu -s remote.host -t 30

# UDP bidirectional test
iperf-tool --ssh remote.host --user ubuntu -s remote.host -u --bidir -t 10 -P 2 -b 20M

# Reverse test (remote → local)
iperf-tool --ssh remote.host --user ubuntu -s remote.host -R -t 10
```

## Flags

### Local Test

| Flag | Long Form | Description | Default |
|------|-----------|-------------|---------|
| `-s` | `--server` | Server address (IP or hostname) | — |
| `-p` | `--port` | Server port | 5201 |
| `-P` | `--parallel` | Parallel streams (uses port range internally) | 1 |
| `-t` | `--time` | Test duration in seconds | 10 |
| `-i` | `--interval` | Reporting interval in seconds | 1 |
| `-u` | — | UDP mode (shorthand for `--protocol udp`) | — |
| `--protocol` | — | `tcp` or `udp` | tcp |
| `-l` | `--block-size` | Datagram/buffer size in bytes | iperf2 default |
| `-b` | `--bandwidth` | Target bandwidth, e.g. `100M`, `1G` (UDP only) | unlimited |
| `-R` | `--reverse` | Reverse mode — remote sends, local receives | false |
| `--bidir` | — | Bidirectional — both directions simultaneously | false |
| `-V` | `--ipv6` | Use IPv6 | false |
| `--ping` | — | Measure latency before and during test | false |
| `--binary` | — | Path to iperf2 binary | `iperf` (Windows: `iperf.exe`) |

### Remote Server (SSH)

| Flag | Description | Default |
|------|-------------|---------|
| `--ssh` | SSH host to manage remote iperf2 server/client | — |
| `--user` | SSH username | `$USER` / `%USERNAME%` |
| `--key` | SSH private key path | auto-discover |
| `--password` | SSH password (insecure, prefer `--key`) | — |
| `--ssh-port` | SSH port | 22 |
| `--install` | Install iperf2 on remote host | false |
| `--start-server` | Start remote iperf2 server | false |
| `--stop-server` | Stop remote iperf2 server | false |

### Repeat

| Flag | Description | Default |
|------|-------------|---------|
| `--repeat` | Repeat measurements in a loop until Ctrl-C | false |
| `--repeat-count` | Number of iterations (0 = infinite) | 0 |

### Output

| Flag | Long Form | Description | Default |
|------|-----------|-------------|---------|
| `-o` | `--output` | Output base path; date suffix added automatically | — |
| `-v` | `--verbose` | Show probe status and extra messages | false |
| `--debug` | — | Log raw iperf2 output to OS temp directory | false |

## Examples

### 1. Simple TCP forward test
```bash
iperf-tool --ssh server.example.com --user ubuntu -s server.example.com -t 30
```

### 2. UDP bidirectional test, 2 streams, 20 Mbps each direction
```bash
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -u --bidir -t 10 -P 2 -b 20M -v
```
The pre-flight UDP reachability probe runs automatically. If inbound UDP is open (VPN, LAN, public IPs), direct mode is used — no SSH file overhead. If NAT is blocking, SSH fallback is selected automatically.

### 3. Reverse test (remote → local)
```bash
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -R -t 10 -u -b 50M
```

### 4. Multiple parallel streams
```bash
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -P 4 -t 30 -o results.csv
```
For UDP, `-P 4` uses a port range (e.g. 5201-5204) on both server and client, bypassing the iperf2 Windows `udp_accept` race condition.

### 5. Repeat mode — continuous monitoring
```bash
# Loop forever until Ctrl-C
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -u --bidir -t 10 --repeat

# Exactly 5 runs
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -t 10 --repeat --repeat-count 5
```

### 6. Install iperf2 on remote host
```bash
iperf-tool --ssh server.example.com --user ubuntu --install
```

### 7. Full workflow: install, test, save results
```bash
iperf-tool --ssh server.example.com --user ubuntu \
  --install -s server.example.com -t 30 -P 2 -o results.csv -v
```

### 8. Debug raw iperf2 output
```bash
iperf-tool --ssh server.example.com --user ubuntu \
  -s server.example.com -u --bidir -t 5 --debug
# macOS/Linux: cat $TMPDIR/iperf-debug.log or /tmp/iperf-debug.log
# Windows: type %TEMP%\iperf-debug.log
```

## Output Format

### Interval display (during test)

**TCP / UDP forward:**
```
Time      Mbps         MB           Retransmits
--------------------------------------------------
14:30:01  245.30       29.13        2
14:30:02  248.10       29.48        0
```

**UDP bidirectional:**
```
Time      Fwd Mbps     Rev Mbps     Fwd MB     Rev MB     Rev Jitter   Rev Lost
------------------------------------------------------------------------------------
14:30:01  21.00        20.80        2.50       2.48       3.641 ms     0/1784 (0.0%)
```

### Summary

```
=== Test Results ===
Timestamp:       2026-03-21 14:30:00
Server:          server.example.com:5201
Protocol:        UDP
Direction:       Bidirectional
Bandwidth Target: 10.00 Mbps/stream
Parallel:        2 streams
Duration:        10 seconds

--- Summary ---
Client Send:     21.00 Mbps
Server Recv:     20.95 Mbps
Server Send:     20.80 Mbps
Client Recv:     20.75 Mbps
C→S Jitter:      4.56 ms
C→S Lost:        0/8920 (0.00%)
S→C Lost:        3/8880 (0.03%)
C→S transferred: 25.00 MB sent / 24.94 MB received
S→C transferred: 25.00 MB sent / 24.93 MB received
```

`Server Recv` and `C→S Jitter/Lost` come from the remote server's measurement (authoritative receive-side stats). When SSH is connected, the server output file is always available as a fallback — if the iperf2 Server Report is fabricated (e.g. severe congestion, NAT, Tailscale), the tool automatically reads server-side data via SSH.

### CSV export

With `-o results.csv`, two files are written:
- `results_<date>.csv` — per-interval log (bandwidth, loss, jitter per second)
- `results_log.csv` — cumulative summary log (one row per test run)

## Authentication

### SSH key (recommended)
```bash
iperf-tool --ssh remote.host --user ubuntu --key ~/.ssh/id_ed25519 -s remote.host -t 10
```
Keys in `~/.ssh/` (id_ed25519, id_rsa, id_ecdsa) are auto-discovered. SSH agent is also used automatically.

### SSH password (not recommended)
```bash
iperf-tool --ssh remote.host --user ubuntu --password "mypass" -s remote.host -t 10
```

## UDP Probe Behavior

Before every UDP reverse or bidirectional test, a pre-flight reachability probe runs:

1. Binds a local UDP socket on an ephemeral port
2. Instructs the remote via SSH to send a single UDP packet to `local-ip:port`
3. Waits up to 2 seconds

| Result | Mode selected |
|--------|--------------|
| Packet received | Direct mode — Server Report from client stdout |
| Timeout | SSH fallback — server output read from remote `-o` file |
| SSH error | SSH fallback (safe default) |

With `-v`, the probe result is printed: `UDP reachability probe: open — using direct mode`.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (invalid flags, SSH failure, test error) |

## Troubleshooting

### "must provide -s <server> or --ssh <host>"
Provide `-s <addr>` for a local test or `--ssh <host>` for remote operations.

### "SSH connect: permission denied"
Check SSH credentials. Key auto-discovery looks for `~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa` and the SSH agent.

### "Server Recv: N/A" in UDP results
The forward Server Report ACK did not arrive and SSH file fallback was unavailable (no SSH connection). When SSH is connected, server data is always retrieved via SSH file fallback automatically. Without SSH, try reducing `-b` bandwidth — the link may be saturated.

### "start remote server: remote command failed"
iperf2 (`iperf` or `iperf.exe`) must be in the remote PATH. Use `--install` to install it, or verify with `ssh user@host iperf --version`.

### High loss on bidirectional UDP
Bidirectional UDP doubles the offered load on the link. Reduce `-b` to stay within link capacity. Total load = `-b` × `-P` × 2 directions.

## See Also

- [MODES.md](MODES.md) — GUI vs CLI comparison
- [INSTALLATION.md](INSTALLATION.md) — Remote iperf2 installation details
- [docs/tech/iperf2-udp-bidir-findings.md](tech/iperf2-udp-bidir-findings.md) — UDP bidir implementation findings
