package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestConnectConfigValidation(t *testing.T) {
	// No auth method should fail
	_, err := Connect(ConnectConfig{
		Host: "localhost",
		Port: 22,
		User: "test",
	})
	if err == nil {
		t.Error("expected error when no auth method provided")
	}
}

func TestServerManagerState(t *testing.T) {
	mgr := NewServerManager()

	if mgr.IsRunning() {
		t.Error("new ServerManager should not be running")
	}

	// StopServer without running should error
	err := mgr.StopServer(nil)
	if err == nil {
		t.Error("expected error stopping non-running server")
	}
}

// generateTestKey returns a fresh RSA public key for use in tests.
func generateTestKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pub, err := gossh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("convert public key: %v", err)
	}
	return pub
}

func TestBuildHostKeyCallback_InsecureSkipVerify(t *testing.T) {
	cb, err := buildHostKeyCallback(ConnectConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// InsecureIgnoreHostKey accepts any key without error
	key := generateTestKey(t)
	if err := cb("anyhost:22", &net.TCPAddr{}, key); err != nil {
		t.Errorf("expected nil from insecure callback, got: %v", err)
	}
}

func TestTofuHostKeyCallback_NewHost(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")
	// Create empty known_hosts
	if err := os.WriteFile(khPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cb, err := tofuHostKeyCallback(khPath)
	if err != nil {
		t.Fatalf("tofuHostKeyCallback: %v", err)
	}

	key := generateTestKey(t)
	if err := cb("newhost:22", &net.TCPAddr{}, key); err != nil {
		t.Errorf("expected nil for new host TOFU, got: %v", err)
	}

	// The key should now be present in known_hosts
	data, _ := os.ReadFile(khPath)
	if len(data) == 0 {
		t.Error("expected key to be appended to known_hosts, but file is empty")
	}
}

func TestTofuHostKeyCallback_KnownHostMatch(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	key := generateTestKey(t)
	// Pre-populate known_hosts with the key
	line := knownhosts.Line([]string{knownhosts.Normalize("knownhost:22")}, key)
	if err := os.WriteFile(khPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cb, err := tofuHostKeyCallback(khPath)
	if err != nil {
		t.Fatalf("tofuHostKeyCallback: %v", err)
	}

	if err := cb("knownhost:22", &net.TCPAddr{}, key); err != nil {
		t.Errorf("expected nil for matching known host, got: %v", err)
	}
}

func TestTofuHostKeyCallback_KeyMismatch(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")

	originalKey := generateTestKey(t)
	// Pre-populate known_hosts with originalKey
	line := knownhosts.Line([]string{knownhosts.Normalize("mismatchhost:22")}, originalKey)
	if err := os.WriteFile(khPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cb, err := tofuHostKeyCallback(khPath)
	if err != nil {
		t.Fatalf("tofuHostKeyCallback: %v", err)
	}

	differentKey := generateTestKey(t)
	err = cb("mismatchhost:22", &net.TCPAddr{}, differentKey)
	if err == nil {
		t.Fatal("expected error for key mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected error to contain 'mismatch', got: %v", err)
	}
}

func TestBuildHostKeyCallback_MissingKnownHostsCreatesFile(t *testing.T) {
	dir := t.TempDir()
	sshDir := filepath.Join(dir, ".ssh")
	// Override HOME so buildHostKeyCallback uses our temp dir
	t.Setenv("HOME", dir)

	cb, err := buildHostKeyCallback(ConnectConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// known_hosts file should have been created
	khPath := filepath.Join(sshDir, "known_hosts")
	if _, err := os.Stat(khPath); os.IsNotExist(err) {
		t.Errorf("expected known_hosts to be created at %s", khPath)
	}

	// Callback should accept a new host via TOFU
	key := generateTestKey(t)
	if err := cb("freshhost:22", &net.TCPAddr{}, key); err != nil {
		t.Errorf("expected nil for TOFU on fresh known_hosts, got: %v", err)
	}
}
