package iperf

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
)

// IperfConfig holds the parameters for an iperf3 client run.
type IperfConfig struct {
	BinaryPath string // path to iperf3 binary
	ServerAddr string // target server hostname or IP
	Port       int    // server port (default 5201)
	Parallel   int    // number of parallel streams
	Duration   int    // test duration in seconds
	Interval   int    // reporting interval in seconds
	Protocol   string // "tcp" or "udp"
	BlockSize   int  // buffer/datagram size in bytes (0 = iperf3 default)
	MeasurePing bool // run ping before and during test
	Reverse    bool   // -R: server sends, client receives
	Bidir      bool   // --bidir: simultaneous both directions
	Bandwidth  string // -b: target bandwidth (e.g. "100M", "1G"), empty = unlimited
	Congestion string // -C: congestion algorithm (e.g. "bbr", "cubic"), empty = system default
}

// DefaultConfig returns an IperfConfig with sensible defaults.
func DefaultConfig() IperfConfig {
	return IperfConfig{
		BinaryPath: "iperf3",
		Port:       5201,
		Parallel:   1,
		Duration:   10,
		Interval:   1,
		Protocol:   "tcp",
	}
}

var (
	validHostname   = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)
	validBandwidth  = regexp.MustCompile(`^\d+[KMG]?$`)
	validCongestion = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Validate checks the config for invalid or dangerous values.
func (c *IperfConfig) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
	// Allow valid IP addresses and hostnames only
	if net.ParseIP(c.ServerAddr) == nil && !validHostname.MatchString(c.ServerAddr) {
		return fmt.Errorf("invalid server address: %q", c.ServerAddr)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	if c.Parallel < 1 || c.Parallel > 128 {
		return fmt.Errorf("parallel streams must be between 1 and 128, got %d", c.Parallel)
	}
	if c.Duration < 1 {
		return fmt.Errorf("duration must be at least 1 second, got %d", c.Duration)
	}
	if c.Interval < 1 {
		return fmt.Errorf("interval must be at least 1 second, got %d", c.Interval)
	}
	if c.Protocol != "tcp" && c.Protocol != "udp" {
		return fmt.Errorf("protocol must be tcp or udp, got %q", c.Protocol)
	}
	if c.BlockSize < 0 || c.BlockSize > 134217728 {
		return fmt.Errorf("block size must be between 1 and 134217728, got %d", c.BlockSize)
	}
	if c.BinaryPath == "" {
		return fmt.Errorf("iperf3 binary path is required")
	}
	if c.Reverse && c.Bidir {
		return fmt.Errorf("reverse (-R) and bidirectional (--bidir) are mutually exclusive")
	}
	if c.Bandwidth != "" && !validBandwidth.MatchString(c.Bandwidth) {
		return fmt.Errorf("bandwidth must match pattern digits[KMG], got %q", c.Bandwidth)
	}
	if c.Congestion != "" && !validCongestion.MatchString(c.Congestion) {
		return fmt.Errorf("congestion algorithm must be lowercase alphanumeric, got %q", c.Congestion)
	}
	return nil
}

// ToArgs converts the config into iperf3 CLI arguments.
// The -J flag (JSON output) is NOT included here — the runner adds it.
// If supportsCongestion is false, the -C flag will be skipped even if Congestion is set.
func (c *IperfConfig) ToArgs(supportsCongestion bool) []string {
	args := []string{
		"-c", c.ServerAddr,
		"-p", strconv.Itoa(c.Port),
		"-P", strconv.Itoa(c.Parallel),
		"-t", strconv.Itoa(c.Duration),
		"-i", strconv.Itoa(c.Interval),
	}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	if c.BlockSize > 0 {
		args = append(args, "-l", strconv.Itoa(c.BlockSize))
	}
	if c.Reverse {
		args = append(args, "-R")
	}
	if c.Bidir {
		args = append(args, "--bidir")
	}
	if c.Bandwidth != "" {
		args = append(args, "-b", c.Bandwidth)
	}
	if c.Congestion != "" && supportsCongestion {
		args = append(args, "-C", c.Congestion)
	}
	return args
}
