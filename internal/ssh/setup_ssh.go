package ssh

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed setup_windows_ssh.ps1
var setupWindowsSSHScript string

// DefaultPublicKey returns the contents of the user's default SSH public key,
// searching ~/.ssh for id_ed25519.pub, id_rsa.pub, id_ecdsa.pub in that order.
// Returns the path it loaded from and the trimmed key contents. If no key is
// found, returns an error.
func DefaultPublicKey() (path string, key string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("locate home dir: %w", err)
	}
	candidates := []string{"id_ed25519.pub", "id_rsa.pub", "id_ecdsa.pub"}
	for _, name := range candidates {
		p := filepath.Join(home, ".ssh", name)
		data, err := os.ReadFile(p)
		if err == nil {
			return p, strings.TrimSpace(string(data)), nil
		}
	}
	return "", "", fmt.Errorf("no default SSH public key found in %s/.ssh (tried %s)",
		home, strings.Join(candidates, ", "))
}

// SetupWindowsSSHLocal runs the embedded PowerShell script locally to install
// and configure OpenSSH Server on the current Windows machine.
// pubKey is optional; if non-empty it is passed to the script for key-based auth.
// Returns combined stdout/stderr output and any error.
func SetupWindowsSSHLocal(pubKey string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("local SSH setup is only supported on Windows")
	}

	// Check if running as Administrator
	checkCmd := exec.Command("powershell", "-Command",
		"([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	checkOut, _ := checkCmd.Output()
	if strings.TrimSpace(string(checkOut)) != "True" {
		return "", fmt.Errorf("administrator privileges required — please run this application as Administrator")
	}

	// Write script to temp file
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, "setup-windows-ssh.ps1")
	if err := os.WriteFile(scriptPath, []byte(setupWindowsSSHScript), 0600); err != nil {
		return "", fmt.Errorf("write temp script: %w", err)
	}
	defer os.Remove(scriptPath)

	args := []string{"-ExecutionPolicy", "Bypass", "-NoProfile", "-File", scriptPath}
	if pubKey != "" {
		args = append(args, "-PublicKey", pubKey)
	}

	cmd := exec.Command("powershell", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("setup script failed: %w\n%s", err, string(out))
	}
	return string(out), nil
}

// SetupWindowsSSHRemote executes SSH server setup commands on a remote Windows
// host via the given SSH client. Steps:
//  1. Install OpenSSH Server capability
//  2. Start and configure sshd service
//  3. Add firewall rules for SSH (22) and iperf2 (5201 TCP+UDP)
//  4. If pubKey non-empty, write to administrators_authorized_keys
func SetupWindowsSSHRemote(client *Client, pubKey string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("SSH client required")
	}

	var log strings.Builder

	// Step 1: Install OpenSSH Server
	log.WriteString("[1/4] Installing OpenSSH Server...\n")
	out, err := client.RunCommand(`powershell -Command "
		$cap = Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*'
		if ($cap.State -eq 'Installed') { Write-Output 'already installed' }
		else { Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0 | Out-Null; Write-Output 'installed' }
	"`)
	if err != nil {
		log.WriteString(fmt.Sprintf("  Failed: %v\n", err))
		return log.String(), fmt.Errorf("install OpenSSH Server: %w", err)
	}
	log.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(out)))

	// Step 2: Start and configure sshd
	log.WriteString("[2/4] Configuring sshd service...\n")
	out, err = client.RunCommand(`powershell -Command "Start-Service sshd; Set-Service -Name sshd -StartupType Automatic; Write-Output 'sshd started and set to Automatic'"`)
	if err != nil {
		log.WriteString(fmt.Sprintf("  Failed: %v\n", err))
		return log.String(), fmt.Errorf("configure sshd: %w", err)
	}
	log.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(out)))

	// Step 3: Firewall rules
	log.WriteString("[3/4] Configuring firewall rules...\n")
	firewallCmds := []struct {
		name, display, proto string
		port                 int
	}{
		{"sshd", "OpenSSH Server (sshd)", "TCP", 22},
		{"iperf2", "iperf2 Server (TCP)", "TCP", 5201},
		{"iperf2-udp", "iperf2 Server (UDP)", "UDP", 5201},
	}
	for _, fw := range firewallCmds {
		cmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="%s" dir=in action=allow protocol=%s localport=%d`,
			fw.name, fw.proto, fw.port)
		client.RunCommand(cmd) // best-effort, ignore errors
		log.WriteString(fmt.Sprintf("  Rule '%s' (%s %d) applied\n", fw.name, fw.proto, fw.port))
	}

	// Step 4: Public key (optional)
	if pubKey != "" {
		log.WriteString("[4/4] Configuring SSH public key...\n")
		// For admin users, write to ProgramData\ssh\administrators_authorized_keys
		// Compare by the base64 key body (field 2) to ignore comment/whitespace
		// differences, so re-running setup doesn't duplicate entries.
		fields := strings.Fields(pubKey)
		var keyBody string
		if len(fields) >= 2 {
			keyBody = fields[1]
		} else {
			keyBody = strings.TrimSpace(pubKey)
		}
		escapedKey := strings.ReplaceAll(pubKey, "'", "''")
		escapedBody := strings.ReplaceAll(keyBody, "'", "''")
		// Single-line PowerShell: multi-line scripts passed through SSH shell
		// escaping can be eaten silently by the remote parser, so we keep the
		// whole pipeline on one line separated by semicolons.
		psScript := fmt.Sprintf(
			`$sshDir=\"$env:ProgramData\ssh\"; `+
				`$authFile=\"$sshDir\administrators_authorized_keys\"; `+
				`if (!(Test-Path $sshDir)) { New-Item -ItemType Directory -Force -Path $sshDir | Out-Null }; `+
				`if (!(Test-Path $authFile)) { New-Item -ItemType File -Force -Path $authFile | Out-Null }; `+
				`$body='%s'; `+
				`$already = Select-String -Path $authFile -SimpleMatch -Pattern $body -Quiet; `+
				`if ($already) { Write-Output 'key already present, skipping' } `+
				`else { Add-Content -Path $authFile -Value '%s'; Write-Output 'key added to administrators_authorized_keys' }; `+
				`icacls.exe $authFile /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F' | Out-Null`,
			escapedBody, escapedKey)
		keyCmd := `powershell -NoProfile -Command "` + psScript + `"`
		out, err = client.RunCommand(keyCmd)
		if err != nil {
			log.WriteString(fmt.Sprintf("  Failed: %v\n", err))
			return log.String(), fmt.Errorf("configure SSH key: %w", err)
		}
		log.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(out)))
	} else {
		log.WriteString("[4/4] No public key provided, skipping.\n")
	}

	// Restart sshd
	client.RunCommand(`powershell -Command "Restart-Service sshd"`)
	log.WriteString("Setup complete.\n")

	return log.String(), nil
}

// CheckWindowsSSHStatus checks whether OpenSSH Server is installed and running
// on a remote Windows host.
func CheckWindowsSSHStatus(client *Client) (installed bool, running bool, err error) {
	if client == nil {
		return false, false, fmt.Errorf("SSH client required")
	}

	out, err := client.RunCommand(`powershell -Command "
		$cap = Get-WindowsCapability -Online | Where-Object Name -like 'OpenSSH.Server*'
		$svc = Get-Service sshd -ErrorAction SilentlyContinue
		$inst = $cap.State -eq 'Installed'
		$run = $svc -ne $null -and $svc.Status -eq 'Running'
		Write-Output \"$inst|$run\"
	"`)
	if err != nil {
		return false, false, fmt.Errorf("check SSH status: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) == 2 {
		installed = strings.EqualFold(parts[0], "true")
		running = strings.EqualFold(parts[1], "true")
	}
	return installed, running, nil
}
