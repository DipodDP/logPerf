package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Client wraps an SSH connection.
type Client struct {
	conn *ssh.Client
	host string
	user string
}

// ConnectConfig holds SSH connection parameters.
type ConnectConfig struct {
	Host               string
	Port               int
	User               string
	KeyPath            string // path to private key file
	Password           string // fallback if KeyPath is empty
	InsecureSkipVerify bool   // disables host key checking; use only where known_hosts is unavailable
}

// DefaultKeyPaths returns common SSH private key paths that exist on disk.
func DefaultKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}
	var found []string
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			found = append(found, p)
		}
	}
	return found
}

// Connect establishes an SSH connection using key auth (preferred) or password.
// If no KeyPath or Password is provided, it tries the SSH agent and then
// auto-discovers keys from default locations. It also honors ProxyCommand
// from ~/.ssh/config.
func Connect(cfg ConnectConfig) (*Client, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	var authMethods []ssh.AuthMethod
	var signers []ssh.Signer

	// Try SSH agent (handles passphrase-protected keys)
	if agentSigners := sshAgentSigners(); len(agentSigners) > 0 {
		signers = append(signers, agentSigners...)
	}

	if cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read SSH key %q: %w", cfg.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse SSH key: %w", err)
		}
		signers = append(signers, signer)
	}

	// Auto-discover default SSH keys if no explicit key/password was provided
	if cfg.KeyPath == "" {
		for _, keyPath := range DefaultKeyPaths() {
			key, err := os.ReadFile(keyPath)
			if err != nil {
				continue
			}
			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				continue
			}
			signers = append(signers, signer)
		}
	}

	if len(signers) > 0 {
		authMethods = append(authMethods, ssh.PublicKeys(signers...))
	}

	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth method available (no key found in ~/.ssh/ and no password provided)")
	}

	hostKeyCallback, err := buildHostKeyCallback(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure host key verification: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Check ~/.ssh/config for ProxyCommand
	if proxyCmd := lookupProxyCommand(cfg.Host); proxyCmd != "" {
		proxyCmd = strings.ReplaceAll(proxyCmd, "%h", cfg.Host)
		proxyCmd = strings.ReplaceAll(proxyCmd, "%p", fmt.Sprintf("%d", cfg.Port))

		conn, err := dialViaProxyCommand(proxyCmd, sshConfig, addr)
		if err != nil {
			return nil, fmt.Errorf("SSH connect via ProxyCommand to %s: %w", addr, err)
		}
		return &Client{conn: conn, host: cfg.Host, user: cfg.User}, nil
	}

	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s: %w", addr, err)
	}

	return &Client{conn: conn, host: cfg.Host, user: cfg.User}, nil
}

// RunCommand executes a command on the remote host and returns its output.
func (c *Client) RunCommand(cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(out), fmt.Errorf("remote command %q: %w: %s", cmd, err, string(out))
	}
	return string(out), nil
}

// RunCommandStream executes a command on the remote host, invoking onLine
// for each stdout line as it arrives. Stderr is captured and returned along
// with the full combined output. Useful for live progress streaming.
func (c *Client) RunCommandStream(cmd string, onLine func(string)) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return "", fmt.Errorf("start remote command: %w", err)
	}

	var buf strings.Builder
	doneStderr := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			buf.WriteString(sc.Text())
			buf.WriteString("\n")
		}
		close(doneStderr)
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		buf.WriteString(line)
		buf.WriteString("\n")
		if onLine != nil {
			onLine(line)
		}
	}
	<-doneStderr

	if err := session.Wait(); err != nil {
		return buf.String(), fmt.Errorf("remote command %q: %w: %s", cmd, err, buf.String())
	}
	return buf.String(), nil
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// LocalAddr returns the local network address used for the SSH connection.
// It returns an empty string if the connection is not active or if the
// local address cannot be determined.
func (c *Client) LocalAddr() string {
	if c.conn == nil {
		return ""
	}
	
	addr := c.conn.LocalAddr()
	if addr == nil {
		return ""
	}
	
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// sshAgentSigners returns signers from the running SSH agent, or nil.
func sshAgentSigners() []ssh.Signer {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil
	}
	signers, err := agent.NewClient(conn).Signers()
	if err != nil || len(signers) == 0 {
		conn.Close()
		return nil
	}
	return signers
}

// lookupProxyCommand does a minimal parse of ~/.ssh/config to find a
// ProxyCommand that applies to the given host.
func lookupProxyCommand(host string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".ssh", "config")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var currentHosts []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val := parseSSHConfigLine(line)
		key = strings.ToLower(key)

		if key == "host" {
			currentHosts = strings.Fields(val)
		} else if key == "proxycommand" && matchesHost(host, currentHosts) {
			if strings.ToLower(val) == "none" {
				return ""
			}
			return val
		}
	}
	return ""
}

func parseSSHConfigLine(line string) (key, value string) {
	// Handle both "Key=Value" and "Key Value"
	if idx := strings.IndexByte(line, '='); idx != -1 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(line, "\t", 2)
	}
	if len(parts) < 2 {
		return line, ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func matchesHost(host string, patterns []string) bool {
	for _, p := range patterns {
		if p == "*" || p == host {
			return true
		}
		// Simple prefix/suffix glob: e.g. "*.example.com"
		if strings.HasPrefix(p, "*") && strings.HasSuffix(host, p[1:]) {
			return true
		}
		// Simple prefix glob: e.g. "192.168.*"
		if strings.HasSuffix(p, "*") && strings.HasPrefix(host, p[:len(p)-1]) {
			return true
		}
	}
	return false
}

// dialViaProxyCommand runs a ProxyCommand and uses its stdin/stdout as the
// TCP transport for the SSH connection.
func dialViaProxyCommand(cmdLine string, config *ssh.ClientConfig, addr string) (*ssh.Client, error) {
	cmd := exec.Command("sh", "-c", cmdLine)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("proxy stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("proxy stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start proxy command: %w", err)
	}

	// Combine stdin (write) and stdout (read) into a net.Conn-like type
	proxyConn := &proxyRWC{ReadCloser: stdout, WriteCloser: stdin, cmd: cmd}

	c, chans, reqs, err := ssh.NewClientConn(proxyConn, addr, config)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// proxyRWC wraps a command's stdin/stdout as a ReadWriteCloser.
type proxyRWC struct {
	ReadCloser  interface{ Read([]byte) (int, error); Close() error }
	WriteCloser interface{ Write([]byte) (int, error); Close() error }
	cmd         *exec.Cmd
}

func (p *proxyRWC) Read(b []byte) (int, error)  { return p.ReadCloser.Read(b) }
func (p *proxyRWC) Write(b []byte) (int, error) { return p.WriteCloser.Write(b) }
func (p *proxyRWC) Close() error {
	p.WriteCloser.Close()
	p.ReadCloser.Close()
	return p.cmd.Process.Kill()
}

// LocalAddr / RemoteAddr satisfy the net.Conn interface that ssh.NewClientConn expects
// via the underlying transport, but ProxyCommand doesn't have real addresses.
func (p *proxyRWC) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (p *proxyRWC) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (p *proxyRWC) SetDeadline(t time.Time) error      { return nil }
func (p *proxyRWC) SetReadDeadline(t time.Time) error  { return nil }
func (p *proxyRWC) SetWriteDeadline(t time.Time) error { return nil }

func buildHostKeyCallback(cfg ConnectConfig) (ssh.HostKeyCallback, error) {
	if cfg.InsecureSkipVerify {
		fmt.Fprintln(os.Stderr, "Warning: SSH host key verification disabled (InsecureSkipVerify).")
		return ssh.InsecureIgnoreHostKey(), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot determine home directory (%v); host key verification disabled.\n", err)
		return ssh.InsecureIgnoreHostKey(), nil
	}
	sshDir := filepath.Join(home, ".ssh")
	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot create %s (%v); host key verification disabled.\n", sshDir, err)
		return ssh.InsecureIgnoreHostKey(), nil
	}
	if _, statErr := os.Stat(knownHostsPath); os.IsNotExist(statErr) {
		f, createErr := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY, 0o600)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot create %s (%v); host key verification disabled.\n", knownHostsPath, createErr)
			return ssh.InsecureIgnoreHostKey(), nil
		}
		f.Close()
	}
	return tofuHostKeyCallback(knownHostsPath)
}

func tofuHostKeyCallback(knownHostsPath string) (ssh.HostKeyCallback, error) {
	strictCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := strictCallback(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("SSH host key mismatch for %s: expected %s, got %s — possible MITM; "+
				"if the server was reprovisioned, remove the old entry from %s",
				hostname, keyErr.Want[0].Key.Type(), key.Type(), knownHostsPath)
		}
		// New host — TOFU: append and allow
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			return fmt.Errorf("add host key to %s: %w", knownHostsPath, err)
		}
		defer f.Close()
		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("write host key to %s: %w", knownHostsPath, err)
		}
		fmt.Printf("Warning: permanently added '%s' (%s) to the list of known hosts.\n", hostname, key.Type())
		return nil
	}, nil
}
