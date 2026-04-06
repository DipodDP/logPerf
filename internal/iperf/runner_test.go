package iperf

import (
	"testing"
)

func TestDefaultRunnerConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 5201 {
		t.Errorf("expected default port 5201, got %d", cfg.Port)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("expected default protocol tcp, got %s", cfg.Protocol)
	}
	if cfg.Parallel != 1 {
		t.Errorf("expected default parallel 1, got %d", cfg.Parallel)
	}
}

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner() returned nil")
	}
	if r.debug {
		t.Error("NewRunner() should not have debug=true")
	}
}

func TestNewDebugRunner(t *testing.T) {
	r := NewDebugRunner()
	if r == nil {
		t.Fatal("NewDebugRunner() returned nil")
	}
	if !r.debug {
		t.Error("NewDebugRunner() should have debug=true")
	}
}

func TestStop(t *testing.T) {
	r := NewRunner()
	// Should not panic even with no running processes
	r.Stop()
	if !r.stopped {
		t.Error("Stop() should set stopped=true")
	}
}
