# Automatic iperf3 Installation

The application now includes automatic iperf3 installation on remote servers when the user is connected via SSH with sudo/administrator privileges.

## Features

### OS Detection
The application automatically detects the remote operating system:
- **Linux** (via `uname -s`)
- **macOS** (via `uname -s`)
- **Windows** (via `cmd.exe`)

### Package Manager Detection

#### Linux
Supports the following package managers (auto-detected):
- **apt-get** (Debian/Ubuntu) — `sudo apt-get install iperf3`
- **yum** (RHEL/CentOS 7) — `sudo yum install iperf3`
- **dnf** (Fedora/RHEL 8+) — `sudo dnf install iperf3`
- **apk** (Alpine) — `sudo apk add iperf3`
- **pacman** (Arch) — `sudo pacman -S iperf3`

#### macOS
- **Homebrew** — `brew install iperf3`
  - Homebrew must be installed

#### Windows
- **Chocolatey** — `choco install iperf3`
- **WinGet** (Windows 10+) — `winget install EricSilva.iPerf3`
- If neither is available, the user is directed to [iperf.fr](https://iperf.fr/iperf-download.php) for manual installation

## Requirements

### Privileges
- **Linux/macOS**: User must have `sudo` privilege (passwordless sudo preferred)
- **Windows**: User must be an Administrator (UAC elevation required)

### Verification
- The application checks if iperf3 is already installed before attempting installation
- If iperf3 is installed, "Install iperf3" button shows as unavailable after connection

## GUI Usage

1. **Connect to Remote Server**
   - Enter SSH host, username, and authentication (key or password)
   - Click "Connect"

2. **Install iperf3** (if not already installed)
   - Click "Install iperf3" button
   - Status updates show installation progress
   - For Windows, UAC may prompt for administrator approval

3. **Start Server**
   - Set desired port (default 5201)
   - Click "Start Server" to launch iperf3 in daemon mode

## Error Handling

If installation fails, the status label will show:
- "requires sudo/administrator privileges to install iperf3" — user lacks elevated privileges
- "no supported package manager found" — unsupported OS or missing all package managers
- Other specific error messages with details

## Implementation Details

### SSH Module Functions

- `Client.DetectOS()` — Determines remote operating system
- `Client.CheckIperf3Installed()` — Checks if iperf3 is available in PATH
- `Client.InstallIperf3()` — Detects OS and runs appropriate install command
- `Client.hasSudoPrivilege()` — Verifies sudo/admin access via `sudo -n true`
- `Client.installLinux()`, `Client.installMacOS()`, `Client.installWindows()` — OS-specific install logic

### UI Integration

The "Install iperf3" button is:
- **Enabled** after successful SSH connection
- **Disabled** during installation (non-blocking, runs in goroutine)
- **Status updates** reflected in the status label

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
- **Linux/macOS**: Run `sudo visudo` and ensure your user is in the sudoers list, preferably with NOPASSWD for iperf3 commands
- **Windows**: Ensure you have Administrator privileges; UAC may need to be confirmed

### "no supported package manager found"
- **Linux**: One of (apt-get, yum, dnf, apk, pacman) must be installed
- **macOS**: Homebrew must be installed (https://brew.sh)
- **Windows**: Install Chocolatey (https://chocolatey.org) or use Windows 10+ WinGet

### Connection timeout during installation
- Installation may take 1-2 minutes on slow connections
- Large package downloads are normal on first-time installation

## Future Enhancements

- [ ] Progress indication for long installations
- [ ] Support for more Windows installation methods (Scoop, direct download)
- [ ] Pre-check for package manager availability before attempting install
- [ ] Fallback to building iperf3 from source if no package manager available
