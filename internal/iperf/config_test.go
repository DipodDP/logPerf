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

	args := cfg.ToArgs(true) // assume congestion supported in tests
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

	args := cfg.ToArgs(true) // assume congestion supported in tests
	for _, a := range args {
		if a == "-l" {
			t.Errorf("should not contain -l when BlockSize is 0, got %v", args)
		}
	}
}

func TestToArgs_UDP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"

	args := cfg.ToArgs(true) // assume congestion supported in tests
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

func TestValidate_ReverseBidirMutuallyExclusive(t *testing.T) {
	cfg := validConfig()
	cfg.Reverse = true
	cfg.Bidir = true
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when both Reverse and Bidir are set")
	}
	if err != nil && !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually exclusive error, got: %v", err)
	}
}

func TestValidate_ReverseOnly(t *testing.T) {
	cfg := validConfig()
	cfg.Reverse = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_BidirOnly(t *testing.T) {
	cfg := validConfig()
	cfg.Bidir = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_Bandwidth(t *testing.T) {
	tests := []struct {
		name      string
		bandwidth string
		wantErr   bool
	}{
		{"empty (default)", "", false},
		{"bare number", "100", false},
		{"with K suffix", "500K", false},
		{"with M suffix", "100M", false},
		{"with G suffix", "1G", false},
		{"invalid suffix", "100X", true},
		{"letters only", "abc", true},
		{"with spaces", "100 M", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Bandwidth = tt.bandwidth
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_Congestion(t *testing.T) {
	tests := []struct {
		name       string
		congestion string
		wantErr    bool
	}{
		{"empty (default)", "", false},
		{"bbr", "bbr", false},
		{"cubic", "cubic", false},
		{"with underscore", "bbr_v2", false},
		{"uppercase", "BBR", true},
		{"with spaces", "bb r", true},
		{"starts with number", "2bbr", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Congestion = tt.congestion
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToArgs_Reverse(t *testing.T) {
	cfg := validConfig()
	cfg.Reverse = true
	args := cfg.ToArgs(true) // assume congestion supported in tests
	found := false
	for _, a := range args {
		if a == "-R" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected -R in args, got %v", args)
	}
}

func TestToArgs_Bidir(t *testing.T) {
	cfg := validConfig()
	cfg.Bidir = true
	args := cfg.ToArgs(true) // assume congestion supported in tests
	found := false
	for _, a := range args {
		if a == "--bidir" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --bidir in args, got %v", args)
	}
}

func TestToArgs_Bandwidth(t *testing.T) {
	cfg := validConfig()
	cfg.Bandwidth = "100M"
	args := cfg.ToArgs(true) // assume congestion supported in tests
	found := false
	for i, a := range args {
		if a == "-b" && i+1 < len(args) && args[i+1] == "100M" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -b 100M in args, got %v", args)
	}
}

func TestToArgs_Congestion(t *testing.T) {
	cfg := validConfig()
	cfg.Congestion = "bbr"
	args := cfg.ToArgs(true) // assume congestion supported in tests
	found := false
	for i, a := range args {
		if a == "-C" && i+1 < len(args) && args[i+1] == "bbr" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -C bbr in args, got %v", args)
	}
}

func TestToArgs_NoNewFlagsWhenDefault(t *testing.T) {
	cfg := validConfig()
	args := cfg.ToArgs(true) // assume congestion supported in tests
	for _, a := range args {
		switch a {
		case "-R", "--bidir", "-b", "-C":
			t.Errorf("should not contain %s when defaults are used, got %v", a, args)
		}
	}
}

func TestToArgs_CongestionNotSupported(t *testing.T) {
	cfg := validConfig()
	cfg.Congestion = "bbr"
	args := cfg.ToArgs(false) // congestion NOT supported
	for i, a := range args {
		if a == "-C" {
			t.Errorf("should not contain -C when congestion not supported, got %v", args)
		}
		if a == "bbr" && i > 0 && args[i-1] == "-C" {
			t.Errorf("should not contain congestion value when not supported, got %v", args)
		}
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
