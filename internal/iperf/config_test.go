package iperf

import (
	"strings"
	"testing"
)

func validConfig() IperfConfig {
	c := DefaultConfig()
	c.ServerAddr = "192.168.1.1"
	return c
}

func TestValidate_BlockSize(t *testing.T) {
	tests := []struct {
		name      string
		blockSize int
		wantErr   bool
	}{
		{"zero (default)", 0, false},
		{"valid small", 1024, false},
		{"valid max", 134217728, false},
		{"negative", -1, true},
		{"too large", 134217729, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.BlockSize = tt.blockSize
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToArgs_BlockSize(t *testing.T) {
	cfg := validConfig()
	cfg.BlockSize = 65536

	args := cfg.ToArgs()
	found := false
	for i, a := range args {
		if a == "-l" && i+1 < len(args) && args[i+1] == "65536" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -l 65536 in args, got %v", args)
	}
}

func TestToArgs_NoBlockSize(t *testing.T) {
	cfg := validConfig()

	args := cfg.ToArgs()
	for _, a := range args {
		if a == "-l" {
			t.Errorf("should not contain -l when BlockSize is 0, got %v", args)
		}
	}
}

func TestToArgs_UDP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"

	args := cfg.ToArgs()
	found := false
	for _, a := range args {
		if a == "-u" {
			found = true
		}
	}
	if !found {
		t.Error("expected -u flag for UDP protocol")
	}
}

func TestValidate_RequiredFields(t *testing.T) {
	cfg := DefaultConfig()
	// No server address
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty server address")
	}
	if err := cfg.Validate(); !strings.Contains(err.Error(), "server address") {
		t.Errorf("expected server address error, got: %v", err)
	}
}
