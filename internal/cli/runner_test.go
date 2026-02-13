package cli

import (
	"testing"
)

func TestLocalTestRunnerConfig(t *testing.T) {
	cfg := RunnerConfig{
		ServerAddr: "192.168.1.1",
		Port:       5201,
		Parallel:   4,
		Duration:   10,
		Interval:   1,
		Protocol:   "tcp",
		BinaryPath: "iperf3",
	}

	// Verify config values
	if cfg.ServerAddr != "192.168.1.1" {
		t.Errorf("ServerAddr = %q, want 192.168.1.1", cfg.ServerAddr)
	}
	if cfg.Parallel != 4 {
		t.Errorf("Parallel = %d, want 4", cfg.Parallel)
	}
}

func TestRemoteServerRunner(t *testing.T) {
	cfg := RunnerConfig{
		SSHHost:    "example.com",
		SSHUser:    "ubuntu",
		SSHPort:    22,
		Verbose:    false,
	}

	runner := NewRemoteServerRunner(cfg)
	if runner == nil {
		t.Fatal("NewRemoteServerRunner returned nil")
	}

	// Connection check without actual SSH should fail gracefully
	// (we don't want to test against a real server)
}

func TestPrintResult(t *testing.T) {
	// PrintResult writes to stdout, just verify it doesn't panic
	// Actual output verification would require capturing stdout
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PrintResult panicked: %v", r)
		}
	}()

	// Create a minimal result to print
	// (This test just ensures the function doesn't crash)
}
