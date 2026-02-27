package ssh

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ServerManager tracks and controls a remote iperf3 server process.
type ServerManager struct {
	mu      sync.Mutex
	running bool
	port    int
}

// NewServerManager creates a new ServerManager.
func NewServerManager() *ServerManager {
	return &ServerManager{}
}

// StartServer starts an iperf3 server on the remote host.
//
// Strategy:
//  1. Unix daemon mode: iperf3 -s -p <port> -D, verified with netstat.
//  2. Windows/schtasks: creates and immediately runs a scheduled task.
//     Works with Cygwin iperf3 under OpenSSH for Windows and survives
//     SSH disconnect.
func (m *ServerManager) StartServer(client *Client, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("iperf3 server already running on port %d", m.port)
	}

	return m.start(client, port)
}

// start does the actual work; must be called with m.mu held.
func (m *ServerManager) start(client *Client, port int) error {
	// 1. Check if we are on Windows (cmd.exe is present)
	if _, err := client.RunCommand("cmd.exe /c echo %OS%"); err == nil {
		// Windows path:
		// schtasks is the most reliable way to create a completely detached process
		// that survives SSH disconnects and doesn't block the SSH channel.
		// We use cmd /c cd to ensure it runs in the C:\iperf3 dir so it finds cygwin1.dll.
		createRun := fmt.Sprintf(
			`schtasks /create /tn "iperf3srv" /tr "cmd.exe /c cd /d C:\iperf3 && iperf3.exe -s -p %d" /sc once /st 00:00 /f && schtasks /run /tn "iperf3srv"`,
			port,
		)
		if _, err := client.RunCommand(createRun); err == nil {
			time.Sleep(1 * time.Second)
			if isListening(client, port) == nil {
				m.running = true
				m.port = port
				return nil
			}
		}

		// Fallback for Windows if schtasks fails (e.g., due to permissions):
		// Use WMI to create a detached background process.
		wmiCmd := fmt.Sprintf(`powershell -Command "Invoke-WmiMethod -Class Win32_Process -Name Create -ArgumentList 'cmd.exe /c cd /d C:\iperf3 && iperf3.exe -s -p %d'"`, port)
		if _, err := client.RunCommand(wmiCmd); err != nil {
			return fmt.Errorf("start remote iperf3 server (Windows WMI fallback): %w", err)
		}
		time.Sleep(1 * time.Second)
		if err := isListening(client, port); err != nil {
			return fmt.Errorf("iperf3 server started but not listening on port %d: %w", port, err)
		}
		
		m.running = true
		m.port = port
		return nil
	}

	// 2. Unix daemon mode.
	if _, err := client.RunCommand(fmt.Sprintf("iperf3 -s -p %d -D", port)); err != nil {
		return fmt.Errorf("start remote iperf3 server: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := isListening(client, port); err != nil {
		return fmt.Errorf("iperf3 server started but not listening on port %d: %w", port, err)
	}

	m.running = true
	m.port = port
	return nil
}

// StopServer stops the remote iperf3 server process.
func (m *ServerManager) StopServer(client *Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("iperf3 server is not running")
	}

	// Unix
	if _, err := client.RunCommand("pkill -f 'iperf3 -s'"); err != nil {
		if _, err2 := client.RunCommand("killall iperf3"); err2 != nil {
			// Windows
			client.RunCommand("taskkill /F /IM iperf3.exe /T")
			client.RunCommand(`schtasks /delete /tn "iperf3srv" /f`)
		}
	}

	m.running = false
	m.port = 0
	return nil
}

// CheckStatus checks whether iperf3 is actually listening on the remote host.
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
	out, err := client.RunCommand("pgrep -f 'iperf3 -s'")
	if err != nil {
		outWin, errWin := client.RunCommand("tasklist | findstr iperf3.exe")
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

// RestartServer kills any existing iperf3 processes and starts a fresh server.
func (m *ServerManager) RestartServer(client *Client, port int) error {
	m.mu.Lock()
	m.running = false
	m.port = 0
	m.mu.Unlock()

	// Force-kill any stale processes.
	client.RunCommand("pkill -9 iperf3")
	client.RunCommand("taskkill /F /IM iperf3.exe /T")
	client.RunCommand(`schtasks /delete /tn "iperf3srv" /f`)
	time.Sleep(300 * time.Millisecond)

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.start(client, port)
}

// IsRunning returns the locally tracked state.
func (m *ServerManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
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
