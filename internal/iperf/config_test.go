package iperf

import (
	"strings"
	"testing"
)

func validConfig() Config {
	c := DefaultConfig()
	c.ServerAddr = "192.168.1.1"
	return c
}

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
	if cfg.BinaryPath != "iperf" {
		t.Errorf("expected default binary 'iperf', got %s", cfg.BinaryPath)
	}
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
		{"lowercase m", "100m", false},
		{"lowercase k", "500k", false},
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

func TestValidate_RequiredFields(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty server address")
	}
	if err := cfg.Validate(); !strings.Contains(err.Error(), "server address") {
		t.Errorf("expected server address error, got: %v", err)
	}
}

func TestValidate_BidirPortRange(t *testing.T) {
	cfg := validConfig()
	cfg.Bidir = true
	cfg.Port = 65530
	cfg.Parallel = 4
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for bidir port range overflow")
	}
	if err != nil && !strings.Contains(err.Error(), "port range") {
		t.Errorf("expected port range error, got: %v", err)
	}
}

func TestValidate_SSHFallbackRequiresFile(t *testing.T) {
	cfg := validConfig()
	cfg.SSHFallback = true
	cfg.RemoteOutputFile = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when SSHFallback without RemoteOutputFile")
	}
}

func TestPortRangeStr(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		parallel int
		offset   int
		want     string
	}{
		{"single port", 5201, 1, 0, "5201"},
		{"two ports", 5201, 2, 0, "5201-5202"},
		{"with offset", 5201, 2, 2, "5203-5204"},
		{"single with offset", 5201, 1, 3, "5204"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Port: tt.port, Parallel: tt.parallel}
			got := cfg.PortRangeStr(tt.offset)
			if got != tt.want {
				t.Errorf("PortRangeStr(%d) = %q, want %q", tt.offset, got, tt.want)
			}
		})
	}
}

func TestFwdClientArgs_TCP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "tcp"
	cfg.Duration = 10
	cfg.Interval = 1
	args := cfg.fwdClientArgs()

	if !containsArg(args, "-c") {
		t.Error("expected -c flag")
	}
	if containsArg(args, "-u") {
		t.Error("unexpected -u flag for TCP")
	}
	if containsArg(args, "-b") {
		t.Error("unexpected -b flag for TCP without bandwidth")
	}
}

func TestFwdClientArgs_UDP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"
	cfg.Bandwidth = "7M"
	cfg.Duration = 10
	cfg.Interval = 1
	args := cfg.fwdClientArgs()

	if !containsArg(args, "-u") {
		t.Error("expected -u flag for UDP")
	}
	if !containsArg(args, "-b") {
		t.Error("expected -b flag for UDP with bandwidth")
	}
}

func TestFwdServerArgs(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"
	cfg.Enhanced = true
	cfg.Interval = 1
	args := cfg.fwdServerArgs()

	if !containsArg(args, "-s") {
		t.Error("expected -s flag")
	}
	if !containsArg(args, "-u") {
		t.Error("expected -u flag")
	}
	if !containsArg(args, "-e") {
		t.Error("expected -e flag for enhanced")
	}
}

func TestRemoteServerStartCmd_Unix(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"
	cfg.Interval = 1
	cmd := cfg.remoteServerStartCmd()
	if !strings.Contains(cmd, "iperf") {
		t.Error("expected 'iperf' in command")
	}
	if !strings.HasSuffix(cmd, "&") {
		t.Error("expected command to end with &")
	}
}

func TestRemoteServerStartCmd_Windows(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"
	cfg.IsWindows = true
	cfg.Interval = 1
	cmd := cfg.remoteServerStartCmd()
	if !strings.Contains(cmd, "Invoke-WmiMethod") {
		t.Error("expected 'Invoke-WmiMethod' for Windows")
	}
	if !strings.Contains(cmd, "iperf.exe") {
		t.Error("expected 'iperf.exe' for Windows")
	}
}

func TestRemoteServerKillCmd(t *testing.T) {
	cfg := Config{IsWindows: false}
	if !strings.Contains(cfg.remoteServerKillCmd(), "pkill") {
		t.Error("expected pkill for Unix")
	}
	cfg.IsWindows = true
	if !strings.Contains(cfg.remoteServerKillCmd(), "taskkill") {
		t.Error("expected taskkill for Windows")
	}
}

func TestBandwidthPerStreamMbps(t *testing.T) {
	tests := []struct {
		bw       string
		parallel int
		want     float64
	}{
		{"", 1, 0},
		{"100M", 1, 100},
		{"100M", 4, 100},
		{"1G", 2, 1000},
		{"500K", 1, 0.5},
	}
	for _, tt := range tests {
		cfg := Config{Bandwidth: tt.bw, Parallel: tt.parallel}
		got := cfg.BandwidthPerStreamMbps()
		if got != tt.want {
			t.Errorf("BandwidthPerStreamMbps(%q, %d) = %f, want %f", tt.bw, tt.parallel, got, tt.want)
		}
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{"valid", func(c *Config) { c.ServerAddr = "192.168.1.1" }, false},
		{"valid hostname", func(c *Config) { c.ServerAddr = "server.example.com" }, false},
		{"empty server", func(c *Config) {}, true},
		{"invalid server chars", func(c *Config) { c.ServerAddr = "foo; rm -rf /" }, true},
		{"port too low", func(c *Config) { c.ServerAddr = "1.2.3.4"; c.Port = 0 }, true},
		{"port too high", func(c *Config) { c.ServerAddr = "1.2.3.4"; c.Port = 70000 }, true},
		{"bad protocol", func(c *Config) { c.ServerAddr = "1.2.3.4"; c.Protocol = "sctp" }, true},
		{"zero duration", func(c *Config) { c.ServerAddr = "1.2.3.4"; c.Duration = 0 }, true},
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

func TestFwdClientArgs_IPv6(t *testing.T) {
	cfg := validConfig()
	cfg.IPv6 = true
	args := cfg.fwdClientArgs()
	if !containsArg(args, "-V") {
		t.Error("expected -V flag in fwdClientArgs when IPv6 is true")
	}
}

func TestRevClientCmd_IPv6(t *testing.T) {
	cfg := validConfig()
	cfg.IPv6 = true
	cfg.LocalAddr = "192.168.1.2"
	cmd := cfg.revClientCmd()
	if !strings.Contains(cmd, "-V") {
		t.Errorf("expected -V flag in revClientCmd when IPv6 is true, got: %s", cmd)
	}
}

func TestDualtestClientArgs_TCP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "tcp"
	cfg.Duration = 10
	cfg.Interval = 1
	args := cfg.dualtestClientArgs()

	if !containsArg(args, "-d") {
		t.Error("expected -d flag for dualtest")
	}
	if !containsArg(args, "-c") {
		t.Error("expected -c flag")
	}
	if containsArg(args, "-u") {
		t.Error("unexpected -u flag for TCP dualtest")
	}
	if containsArg(args, "-b") {
		t.Error("unexpected -b flag for TCP dualtest without bandwidth")
	}
	if !containsArg(args, "-p") {
		t.Error("expected -p flag")
	}
	if !containsArg(args, "-t") {
		t.Error("expected -t flag")
	}
	if !containsArg(args, "-f") {
		t.Error("expected -f flag")
	}
	if !containsArg(args, "-i") {
		t.Error("expected -i flag")
	}
	// Verify format is 'm' (Mbits)
	for i, a := range args {
		if a == "-f" && i+1 < len(args) {
			if args[i+1] != "m" {
				t.Errorf("expected -f m, got -f %s", args[i+1])
			}
		}
	}
	// Verify server addr follows -c
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			if args[i+1] != cfg.ServerAddr {
				t.Errorf("expected -c %s, got -c %s", cfg.ServerAddr, args[i+1])
			}
		}
	}
}

func TestDualtestClientArgs_UDP(t *testing.T) {
	cfg := validConfig()
	cfg.Protocol = "udp"
	cfg.Bandwidth = "10M"
	cfg.Duration = 5
	cfg.Interval = 1
	args := cfg.dualtestClientArgs()

	if !containsArg(args, "-d") {
		t.Error("expected -d flag for dualtest")
	}
	if !containsArg(args, "-u") {
		t.Error("expected -u flag for UDP dualtest")
	}
	if !containsArg(args, "-b") {
		t.Error("expected -b flag for UDP dualtest with bandwidth set")
	}
	// Verify bandwidth value follows -b
	for i, a := range args {
		if a == "-b" && i+1 < len(args) {
			if args[i+1] != cfg.Bandwidth {
				t.Errorf("expected -b %s, got -b %s", cfg.Bandwidth, args[i+1])
			}
		}
	}
}

func TestDualtestClientArgs_TCP_NoBandwidth(t *testing.T) {
	// TCP dualtest should never include -b even if Bandwidth is set
	cfg := validConfig()
	cfg.Protocol = "tcp"
	cfg.Bandwidth = "100M"
	cfg.Duration = 10
	cfg.Interval = 1
	args := cfg.dualtestClientArgs()

	if containsArg(args, "-b") {
		t.Error("unexpected -b flag for TCP dualtest (bandwidth only applies to UDP)")
	}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}
