package ssh

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ServerManager tracks and controls a remote iperf2 server process.
type ServerManager struct {
	mu           sync.Mutex
	running      bool
	port         int
	numInstances int // number of iperf2 server instances (default 2)
}

// NewServerManager creates a new ServerManager.
func NewServerManager() *ServerManager {
	return &ServerManager{}
}

// StartServer starts an iperf2 server on the remote host.
//
// Strategy:
//  1. Unix daemon mode: iperf -s -p <port> -D, verified with netstat.
//  2. Windows/schtasks: creates and immediately runs a scheduled task.
//     Works with iperf2 under OpenSSH for Windows and survives
//     SSH disconnect.
func (m *ServerManager) StartServer(client *Client, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("iperf2 server already running on port %d", m.port)
	}

	return m.start(client, port, 2)
}

// start does the actual work; must be called with m.mu held.
// It starts numInstances iperf2 instances on consecutive ports starting at port.
func (m *ServerManager) start(client *Client, port, numInstances int) error {
	if numInstances < 1 {
		numInstances = 2
	}

	// Pre-check: bail out early with a clear message if iperf is missing.
	if installed, _ := client.CheckIperfInstalled(); !installed {
		return fmt.Errorf("iperf2 is not installed on the remote host")
	}

	ports := make([]int, numInstances)
	for i := range ports {
		ports[i] = port + i
	}

	// 1. Check if we are on Windows (cmd.exe is present)
	if _, err := client.RunCommand("cmd.exe /c echo %OS%"); err == nil {
		for i, p := range ports {
			taskName := fmt.Sprintf("iperf2srv%d", i)
			if err := m.startWindows(client, p, taskName); err != nil {
				return fmt.Errorf("start server instance %d on port %d: %w", i, p, err)
			}
		}
		addWindowsFirewallRules(client, ports...)
		m.running = true
		m.port = port
		m.numInstances = numInstances
		return nil
	}

	// 2. Unix daemon mode.
	for _, p := range ports {
		if _, err := client.RunCommand(fmt.Sprintf("iperf -s -p %d -D", p)); err != nil {
			return fmt.Errorf("start remote iperf2 server on port %d: %w", p, err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for _, p := range ports {
		if err := isListening(client, p); err != nil {
			return fmt.Errorf("iperf2 server started but not listening on port %d: %w", p, err)
		}
	}

	m.running = true
	m.port = port
	m.numInstances = numInstances
	return nil
}

// startWindows starts a single iperf2 instance on the given port via schtasks
// (with WMI fallback). taskName must be unique per instance.
func (m *ServerManager) startWindows(client *Client, port int, taskName string) error {
	createRun := fmt.Sprintf(
		`schtasks /create /tn "%s" /tr "cmd.exe /c cd /d C:\iperf2 && iperf.exe -s -p %d" /sc once /st 00:00 /f && schtasks /run /tn "%s"`,
		taskName, port, taskName,
	)
	if _, err := client.RunCommand(createRun); err == nil {
		time.Sleep(1 * time.Second)
		if isListening(client, port) == nil {
			return nil
		}
	}

	// Fallback: WMI
	wmiCmd := fmt.Sprintf(`powershell -Command "Invoke-WmiMethod -Class Win32_Process -Name Create -ArgumentList 'cmd.exe /c cd /d C:\iperf2 && iperf.exe -s -p %d'"`, port)
	if _, err := client.RunCommand(wmiCmd); err != nil {
		return fmt.Errorf("start remote iperf2 server on port %d (Windows WMI fallback): %w", port, err)
	}
	time.Sleep(1 * time.Second)
	if err := isListening(client, port); err != nil {
		return fmt.Errorf("iperf2 server started but not listening on port %d: %w", port, err)
	}
	return nil
}

// StopServer stops the remote iperf2 server process.
func (m *ServerManager) StopServer(client *Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("iperf2 server is not running")
	}

	port := m.port
	n := m.numInstances
	if n < 2 {
		n = 2
	}

	// Unix
	if _, err := client.RunCommand("pkill -f 'iperf -s'"); err != nil {
		if _, err2 := client.RunCommand("killall iperf"); err2 != nil {
			// Windows
			client.RunCommand("taskkill /IM iperf.exe")
			for i := 0; i < n; i++ {
				client.RunCommand(fmt.Sprintf(`schtasks /delete /tn "iperf2srv%d" /f`, i))
			}
			ports := make([]int, n)
			for i := range ports {
				ports[i] = port + i
			}
			removeWindowsFirewallRules(client, ports...)
		}
	}

	m.running = false
	m.port = 0
	m.numInstances = 0
	return nil
}

// CheckStatus checks whether iperf2 is actually listening on the remote host.
func (m *ServerManager) CheckStatus(client *Client) (bool, error) {
	m.mu.Lock()
	port := m.port
	m.mu.Unlock()

	out, _ := client.RunCommand("netstat -an")
	if out != "" {
		needle := ":5201"
		if port != 0 {
			needle = fmt.Sprintf(":%d", port)
		}
		for _, line := range strings.Split(out, "\n") {
			if (strings.Contains(line, "LISTEN") || strings.Contains(line, "LISTENING")) &&
				strings.Contains(line, needle) {
				m.mu.Lock()
				m.running = true
				if m.port == 0 {
					m.port = port
				}
				m.mu.Unlock()
				return true, nil
			}
		}
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return false, nil
	}

	// netstat unavailable: fall back to process list
	out, err := client.RunCommand("pgrep -f 'iperf -s'")
	if err != nil {
		outWin, errWin := client.RunCommand("tasklist | findstr iperf.exe")
		if errWin != nil || strings.TrimSpace(outWin) == "" {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			return false, nil
		}
		out = outWin
	}

	isRunning := strings.TrimSpace(out) != ""
	m.mu.Lock()
	m.running = isRunning
	m.mu.Unlock()
	return isRunning, nil
}

// RestartServer kills any existing iperf2 processes and starts a fresh server.
// numInstances controls how many iperf2 server instances to start on consecutive
// ports. Pass 0 to use the default (2).
func (m *ServerManager) RestartServer(client *Client, port, numInstances int) error {
	if numInstances < 1 {
		numInstances = 2
	}

	m.mu.Lock()
	oldN := m.numInstances
	if oldN < 2 {
		oldN = 2
	}
	m.running = false
	m.port = 0
	m.numInstances = 0
	m.mu.Unlock()

	// Force-kill any stale processes and clean up old firewall rules.
	client.RunCommand("pkill -f 'iperf -s'")
	client.RunCommand("taskkill /IM iperf.exe")
	// Clean up schtasks for the maximum of old and new instance counts.
	cleanN := oldN
	if numInstances > cleanN {
		cleanN = numInstances
	}
	for i := 0; i < cleanN; i++ {
		client.RunCommand(fmt.Sprintf(`schtasks /delete /tn "iperf2srv%d" /f`, i))
	}
	oldPorts := make([]int, cleanN)
	for i := range oldPorts {
		oldPorts[i] = port + i
	}
	removeWindowsFirewallRules(client, oldPorts...)
	time.Sleep(300 * time.Millisecond)

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.start(client, port, numInstances)
}

// IsRunning returns the locally tracked state.
func (m *ServerManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// addWindowsFirewallRules opens TCP+UDP inbound for the given ports.
// Errors are ignored — the rules are best-effort (requires admin privileges).
func addWindowsFirewallRules(client *Client, ports ...int) {
	for _, p := range ports {
		rule := fmt.Sprintf("iperf2-%d", p)
		client.RunCommand(fmt.Sprintf(
			`netsh advfirewall firewall add rule name="%s" dir=in action=allow protocol=TCP localport=%d`, rule, p))
		client.RunCommand(fmt.Sprintf(
			`netsh advfirewall firewall add rule name="%s" dir=in action=allow protocol=UDP localport=%d`, rule, p))
	}
}

// removeWindowsFirewallRules removes the firewall rules created by addWindowsFirewallRules.
func removeWindowsFirewallRules(client *Client, ports ...int) {
	for _, p := range ports {
		rule := fmt.Sprintf("iperf2-%d", p)
		client.RunCommand(fmt.Sprintf(
			`netsh advfirewall firewall delete rule name="%s"`, rule))
	}
}

// isListening returns nil if something is listening on port (netstat works on
// both Unix and Windows).
func isListening(client *Client, port int) error {
	out, _ := client.RunCommand("netstat -an")
	needle := fmt.Sprintf(":%d", port)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) &&
			(strings.Contains(line, "LISTEN") || strings.Contains(line, "LISTENING")) {
			return nil
		}
	}
	return fmt.Errorf("no LISTEN entry found for port %d", port)
}
