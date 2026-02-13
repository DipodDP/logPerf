package ssh

import (
	"testing"
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
