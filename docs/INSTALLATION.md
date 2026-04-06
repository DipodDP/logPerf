# Automatic iperf2 Installation

The application includes automatic iperf2 installation on remote servers when the user is connected via SSH with sudo/administrator privileges.

## Features

### OS Detection
The application automatically detects the remote operating system:
- **Linux** (via `uname -s`)
- **macOS** (via `uname -s`)
- **Windows** (via `cmd.exe /c echo %OS%`)

### Package Manager Detection

#### Linux
Supports the following package managers (auto-detected):
- **apt-get** (Debian/Ubuntu) — `sudo apt-get install -y iperf`
- **yum** (RHEL/CentOS 7) — `sudo yum install -y iperf`
- **dnf** (Fedora/RHEL 8+) — `sudo dnf install -y iperf`
- **apk** (Alpine) — `sudo apk add iperf`
- **pacman** (Arch) — `sudo pacman -S --noconfirm iperf`

#### macOS
- **Homebrew** — `brew install iperf`
  - Homebrew must be installed

#### Windows
- Downloads `iperf-2.2.1-win64.zip` from SourceForge
- Extracts to `C:\iperf2`
- Updates system PATH

## Requirements

### Privileges
- **Linux/macOS**: User must have `sudo` privilege (passwordless sudo preferred)
- **Windows**: User must be an Administrator

### Verification
- The application checks if iperf2 is already installed before attempting installation
- If iperf2 is installed, the install step is skipped

## GUI Usage

1. **Connect to Remote Server**
   - Enter SSH host, username, and authentication (key or password)
   - Click "Connect"

2. **Install iperf2** (if not already installed)
   - Click "Install iperf" button
   - Status updates show installation progress

3. **Start Server**
   - Set desired port (default 5201)
   - Click "Start Server" to launch iperf2 server instances
   - On Unix: daemon mode (`iperf -s -p <port> -D`)
   - On Windows: WMI process creation (survives SSH disconnect)

## Error Handling

If installation fails, the status label will show:
- "iperf2 is not installed on the remote host" — binary not found in PATH
- "requires sudo/administrator privileges" — user lacks elevated privileges
- "no supported package manager found" — unsupported OS or missing all package managers

## Implementation Details

### SSH Module Functions

- `Client.DetectOS()` — Determines remote operating system (`OSLinux`, `OSMacOS`, `OSWindows`)
- `Client.CheckIperfInstalled()` — Checks if iperf/iperf.exe is available in PATH
- `Client.InstallIperf()` — Detects OS and runs appropriate install command
- `Client.installLinux()`, `Client.installMacOS()`, `Client.installWindows()` — OS-specific install logic

### Remote Server Management

- **Unix/Linux**: Daemon mode (`iperf -s -p <port> -D`), verified with `netstat`
- **Windows**: WMI process creation (`Invoke-WmiMethod Win32_Process`), firewall rules added via `netsh`
- **Server stop**: `pkill`/`killall` on Unix, `taskkill /IM iperf.exe /F` on Windows

## Testing

Unit tests cover:
- OS type detection constants
- Command selection for each package manager
- State management (mock client)

To run tests:
```bash
go test ./internal/ssh -v
```

## Troubleshooting

### "requires sudo/administrator privileges"
- **Linux/macOS**: Ensure your user has `sudo` access, preferably with NOPASSWD
- **Windows**: Ensure you have Administrator privileges

### "no supported package manager found"
- **Linux**: One of (apt-get, yum, dnf, apk, pacman) must be installed
- **macOS**: Homebrew must be installed (https://brew.sh)
- **Windows**: Auto-downloads from SourceForge; ensure internet access

### Connection timeout during installation
- Installation may take 1-2 minutes on slow connections
- Large package downloads are normal on first-time installation

## Cross-Platform Support

The tool runs natively on macOS, Linux, and Windows:

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| GUI | Fyne (native) | Fyne (X11/Wayland) | Fyne (native) |
| CLI | Full support | Full support | Full support |
| Ping measurement | `ping -c` / continuous | `ping -c` / continuous | `ping -n` / `ping -t` |
| Process management | SIGTERM | SIGTERM | Kill |
| SSH username default | `$USER` | `$USER` | `%USERNAME%` |
| Binary default | `iperf` | `iperf` | `iperf.exe` |
| Debug log path | `$TMPDIR/iperf-debug.log` | `/tmp/iperf-debug.log` | `%TEMP%\iperf-debug.log` |
| Remote server start | `nohup ... &` | `nohup ... &` | WMI (`Invoke-WmiMethod`) |
