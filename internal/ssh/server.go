package ssh

import (
	"fmt"
	"strings"
	"sync"
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

// StartServer starts iperf3 in daemon mode on the remote host.
func (m *ServerManager) StartServer(client *Client, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("iperf3 server already running on port %d", m.port)
	}

	cmd := fmt.Sprintf("iperf3 -s -p %d -D", port)
	if _, err := client.RunCommand(cmd); err != nil {
		return fmt.Errorf("start remote iperf3 server: %w", err)
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

	// Try pkill first, fall back to killall
	if _, err := client.RunCommand("pkill -f 'iperf3 -s'"); err != nil {
		if _, err2 := client.RunCommand("killall iperf3"); err2 != nil {
			return fmt.Errorf("stop remote iperf3 server: %w", err)
		}
	}

	m.running = false
	m.port = 0
	return nil
}

// CheckStatus checks whether iperf3 is running on the remote host.
func (m *ServerManager) CheckStatus(client *Client) (bool, error) {
	out, err := client.RunCommand("pgrep -f 'iperf3 -s'")
	if err != nil {
		// pgrep returns exit code 1 when no process is found
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return false, nil
	}

	isRunning := strings.TrimSpace(out) != ""
	m.mu.Lock()
	m.running = isRunning
	m.mu.Unlock()
	return isRunning, nil
}

// RestartServer kills all iperf3 processes and starts a fresh server.
func (m *ServerManager) RestartServer(client *Client, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Force-kill any existing iperf3 processes
	client.RunCommand("pkill -9 iperf3")

	cmd := fmt.Sprintf("iperf3 -s -p %d -D", port)
	if _, err := client.RunCommand(cmd); err != nil {
		return fmt.Errorf("restart remote iperf3 server: %w", err)
	}

	m.running = true
	m.port = port
	return nil
}

// IsRunning returns the locally tracked state.
func (m *ServerManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
