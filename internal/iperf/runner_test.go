package iperf

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
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

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*IperfConfig)
		wantErr bool
	}{
		{"valid", func(c *IperfConfig) { c.ServerAddr = "192.168.1.1" }, false},
		{"valid hostname", func(c *IperfConfig) { c.ServerAddr = "server.example.com" }, false},
		{"empty server", func(c *IperfConfig) {}, true},
		{"invalid server chars", func(c *IperfConfig) { c.ServerAddr = "foo; rm -rf /" }, true},
		{"port too low", func(c *IperfConfig) { c.ServerAddr = "1.2.3.4"; c.Port = 0 }, true},
		{"port too high", func(c *IperfConfig) { c.ServerAddr = "1.2.3.4"; c.Port = 70000 }, true},
		{"bad protocol", func(c *IperfConfig) { c.ServerAddr = "1.2.3.4"; c.Protocol = "sctp" }, true},
		{"zero duration", func(c *IperfConfig) { c.ServerAddr = "1.2.3.4"; c.Duration = 0 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToArgs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ServerAddr = "10.0.0.1"
	cfg.Port = 5201
	cfg.Parallel = 4
	cfg.Duration = 10
	cfg.Interval = 1
	cfg.Protocol = "tcp"

	args := cfg.ToArgs()
	expected := []string{"-c", "10.0.0.1", "-p", "5201", "-P", "4", "-t", "10", "-i", "1"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], v)
		}
	}

	// UDP should add -u flag
	cfg.Protocol = "udp"
	args = cfg.ToArgs()
	found := false
	for _, a := range args {
		if a == "-u" {
			found = true
		}
	}
	if !found {
		t.Error("UDP config should include -u flag")
	}
}
