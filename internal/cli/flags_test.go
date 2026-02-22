package cli

import (
	"os"
	"testing"
)

func TestParseFlags_NoArgs(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Simulate no arguments
	os.Args = []string{"iperf-tool"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Errorf("ParseFlags() error = %v, want nil", err)
	}
	if cfg != nil {
		t.Errorf("ParseFlags() with no args should return nil config for GUI mode, got %v", cfg)
	}
}

func TestParseFlags_HelpFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "--help"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Errorf("ParseFlags() error = %v, want nil", err)
	}
	if cfg != nil {
		t.Errorf("ParseFlags() with --help should return nil config, got %v", cfg)
	}
}

func TestParseFlags_LocalTest(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "192.168.1.1", "-p", "5201", "-P", "4", "-t", "30"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v, want nil", err)
	}
	if cfg == nil {
		t.Fatal("ParseFlags() returned nil, want config")
	}

	if cfg.ServerAddr != "192.168.1.1" {
		t.Errorf("ServerAddr = %q, want 192.168.1.1", cfg.ServerAddr)
	}
	if cfg.Port != 5201 {
		t.Errorf("Port = %d, want 5201", cfg.Port)
	}
	if cfg.Parallel != 4 {
		t.Errorf("Parallel = %d, want 4", cfg.Parallel)
	}
	if cfg.Duration != 30 {
		t.Errorf("Duration = %d, want 30", cfg.Duration)
	}
}

func TestParseFlags_UDP(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "10.0.0.1", "-u", "udp"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	if cfg.Protocol != "udp" {
		t.Errorf("Protocol = %q, want udp", cfg.Protocol)
	}
}

func TestParseFlags_RemoteServer(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-ssh", "remote.host", "-user", "ubuntu", "-key", "/path/to/key"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	if cfg.SSHHost != "remote.host" {
		t.Errorf("SSHHost = %q, want remote.host", cfg.SSHHost)
	}
	if cfg.SSHUser != "ubuntu" {
		t.Errorf("SSHUser = %q, want ubuntu", cfg.SSHUser)
	}
	if cfg.SSHKeyPath != "/path/to/key" {
		t.Errorf("SSHKeyPath = %q, want /path/to/key", cfg.SSHKeyPath)
	}
}

func TestParseFlags_ReverseFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "10.0.0.1", "-R"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if !cfg.Reverse {
		t.Error("Reverse should be true")
	}
}

func TestParseFlags_BidirFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "10.0.0.1", "-bidir"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if !cfg.Bidir {
		t.Error("Bidir should be true")
	}
}

func TestParseFlags_BandwidthFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "10.0.0.1", "-b", "100M"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if cfg.Bandwidth != "100M" {
		t.Errorf("Bandwidth = %q, want 100M", cfg.Bandwidth)
	}
}

func TestParseFlags_CongestionFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"iperf-tool", "-server", "10.0.0.1", "-C", "bbr"}

	cfg, err := ParseFlags()
	if err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if cfg.Congestion != "bbr" {
		t.Errorf("Congestion = %q, want bbr", cfg.Congestion)
	}
}

func TestParseFlags_MissingRequired(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// No server address and no SSH host
	os.Args = []string{"iperf-tool", "-p", "5201"}

	cfg, err := ParseFlags()
	if err == nil {
		t.Error("ParseFlags() with missing required flags should return error")
	}
	if cfg != nil {
		t.Errorf("ParseFlags() with error should return nil config, got %v", cfg)
	}
}
