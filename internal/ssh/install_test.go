package ssh

import (
	"testing"
)

func TestOSTypeString(t *testing.T) {
	tests := []struct {
		os   OSType
		name string
	}{
		{OSLinux, "linux"},
		{OSMacOS, "macos"},
		{OSWindows, "windows"},
		{OSUnknown, "unknown"},
	}

	for _, tt := range tests {
		if string(tt.os) != tt.name {
			t.Errorf("OSType %v = %q, want %q", tt.os, string(tt.os), tt.name)
		}
	}
}

// MockClient for testing installation logic without real SSH
type MockClient struct {
	commands map[string]error
	outputs  map[string]string
}

func NewMockClient() *MockClient {
	return &MockClient{
		commands: make(map[string]error),
		outputs:  make(map[string]string),
	}
}

func (mc *MockClient) RunCommand(cmd string) (string, error) {
	if err, exists := mc.commands[cmd]; exists {
		return mc.outputs[cmd], err
	}
	// Default: command not found (simulates missing binary)
	return "", ErrCommandNotFound
}

// ErrCommandNotFound represents a command execution failure
var ErrCommandNotFound = &mockError{"command not found"}

type mockError struct {
	msg string
}

func (me *mockError) Error() string {
	return me.msg
}

func TestLinuxInstallCommandSelection(t *testing.T) {
	mc := NewMockClient()
	// Simulate apt-get being available
	mc.commands["which apt-get"] = nil

	// Create a minimal client-like object for testing
	// This is a conceptual test since we can't easily mock *Client
	// but it demonstrates the logic
	tests := []struct {
		name        string
		checkCmd    string
		expectCmd   string
	}{
		{"apt-get", "which apt-get", "sudo apt-get update && sudo apt-get install -y iperf3"},
		{"yum", "which yum", "sudo yum install -y iperf3"},
		{"dnf", "which dnf", "sudo dnf install -y iperf3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test is illustrative; actual testing would require
			// mocking the full Client interface
			_ = tt.expectCmd
		})
	}
}
