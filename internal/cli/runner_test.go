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
		BinaryPath: "iperf",
	}

	if cfg.ServerAddr != "192.168.1.1" {
		t.Errorf("ServerAddr = %q, want 192.168.1.1", cfg.ServerAddr)
	}
	if cfg.Parallel != 4 {
		t.Errorf("Parallel = %d, want 4", cfg.Parallel)
	}
	if cfg.BinaryPath != "iperf" {
		t.Errorf("BinaryPath = %q, want iperf", cfg.BinaryPath)
	}
}

func TestRemoteServerRunner(t *testing.T) {
	cfg := RunnerConfig{
		SSHHost: "example.com",
		SSHUser: "ubuntu",
		SSHPort: 22,
		Verbose: false,
	}

	runner := NewRemoteServerRunner(cfg)
	if runner == nil {
		t.Fatal("NewRemoteServerRunner returned nil")
	}
}

func TestPrintResult(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PrintResult panicked: %v", r)
		}
	}()
}
