package ssh

import (
	"fmt"
	"strings"
)

// OSType represents the detected remote operating system.
type OSType string

const (
	OSLinux   OSType = "linux"
	OSMacOS   OSType = "macos"
	OSWindows OSType = "windows"
	OSUnknown OSType = "unknown"
)

// DetectOS probes the remote system to determine its operating system.
func (c *Client) DetectOS() (OSType, error) {
	// Try to detect via uname (works on Linux and macOS)
	out, err := c.RunCommand("uname -s")
	if err == nil {
		system := strings.TrimSpace(strings.ToLower(out))
		switch {
		case strings.Contains(system, "linux"):
			return OSLinux, nil
		case strings.Contains(system, "darwin"):
			return OSMacOS, nil
		}
	}

	// Fallback: check for Windows (cmd.exe exists)
	_, err = c.RunCommand("cmd /c echo test")
	if err == nil {
		return OSWindows, nil
	}

	return OSUnknown, fmt.Errorf("could not determine remote OS")
}

// CheckIperf3Installed checks if iperf3 is available on the remote system.
func (c *Client) CheckIperf3Installed() (bool, error) {
	// Linux/Mac check
	_, err := c.RunCommand("which iperf3")
	if err == nil {
		return true, nil
	}

	// Windows fallback using PowerShell
	// Also explicitly check the C:\iperf3 extracted path, 
	// since SSH shells sometimes don't reload new PATH variables instantly.
	winCheckCmd := `powershell -Command "if (Get-Command iperf3 -ErrorAction SilentlyContinue) { exit 0 } elseif (Test-Path \"C:\iperf3\iperf3.exe\") { exit 0 } else { exit 1 }"`
	_, err = c.RunCommand(winCheckCmd)
	return err == nil, nil
}

// InstallIperf3 attempts to install iperf3 on the remote system.
// It detects the OS and uses the appropriate package manager.
// Requires sudo/administrator privileges.
func (c *Client) InstallIperf3() error {
	// First check if already installed
	installed, err := c.CheckIperf3Installed()
	if err == nil && installed {
		return nil // Already installed
	}

	os, err := c.DetectOS()
	if err != nil {
		return fmt.Errorf("detect OS for installation: %w", err)
	}

	// Check for sudo/administrator privileges
	hasSudo, err := c.hasSudoPrivilege(os)
	if err != nil || !hasSudo {
		return fmt.Errorf("requires sudo/administrator privileges to install iperf3")
	}

	var installCmd string
	switch os {
	case OSLinux:
		installCmd, err = c.installLinux()
	case OSMacOS:
		installCmd, err = c.installMacOS()
	case OSWindows:
		installCmd, err = c.installWindows()
	default:
		return fmt.Errorf("unsupported operating system: %v", os)
	}

	if err != nil {
		return fmt.Errorf("build install command: %w", err)
	}

	if installCmd != "" {
		if _, err := c.RunCommand(installCmd); err != nil {
			return fmt.Errorf("install iperf3: %w", err)
		}
	}

	// Verify installation
	if installed, err := c.CheckIperf3Installed(); err != nil || !installed {
		return fmt.Errorf("iperf3 installation verification failed")
	}

	return nil
}

// hasSudoPrivilege checks if the user has sudo/administrator access.
func (c *Client) hasSudoPrivilege(osType OSType) (bool, error) {
	if osType == OSWindows {
		// Windows doesn't use sudo, Winget will handle elevation natively or install per-user
		return true, nil
	}

	// Try to run a simple sudo command without password
	_, err := c.RunCommand("sudo -n true")
	return err == nil, nil
}

// installLinux returns the command to install iperf3 on Linux.
// Detects the package manager (apt, yum, dnf, apk, pacman).
func (c *Client) installLinux() (string, error) {
	// Check which package manager is available
	managers := []struct {
		check  string
		install string
	}{
		{"which apt-get", "sudo apt-get update && sudo apt-get install -y iperf3"},
		{"which yum", "sudo yum install -y iperf3"},
		{"which dnf", "sudo dnf install -y iperf3"},
		{"which apk", "sudo apk add iperf3"},
		{"which pacman", "sudo pacman -S --noconfirm iperf3"},
	}

	for _, mgr := range managers {
		if _, err := c.RunCommand(mgr.check); err == nil {
			return mgr.install, nil
		}
	}

	return "", fmt.Errorf("no supported package manager found (apt, yum, dnf, apk, pacman)")
}

// installMacOS returns the command to install iperf3 on macOS.
// Assumes Homebrew is installed.
func (c *Client) installMacOS() (string, error) {
	// Check if Homebrew is available
	if _, err := c.RunCommand("which brew"); err != nil {
		return "", fmt.Errorf("homebrew not found; please install homebrew or iperf3 manually")
	}
	return "brew install iperf3", nil
}

// installWindows returns the command to install iperf3 on Windows.
// We explicitly bypass package managers like Chocolatey and Winget because
// their iperf3 packages are notoriously outdated (v3.1.3) and crash on UDP.
// Instead, we download and extract a modern, community-maintained build (v3.20).
func (c *Client) installWindows() (string, error) {
	psCmd := `powershell -Command "$ProgressPreference = 'SilentlyContinue'; $dir='C:\iperf3'; $zip=\"$env:TEMP\iperf3.zip\"; if (!(Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }; Invoke-WebRequest -Uri 'https://github.com/ar51an/iperf3-win-builds/releases/download/3.20/iperf-3.20-win64.zip' -OutFile $zip; Expand-Archive -Path $zip -DestinationPath $dir -Force; Remove-Item -Path $zip -Force; $path=[Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::Machine); if ($path -notmatch [regex]::Escape($dir)) { [Environment]::SetEnvironmentVariable('Path', $path + ';' + $dir, [EnvironmentVariableTarget]::Machine) }"`
	return psCmd, nil
}
