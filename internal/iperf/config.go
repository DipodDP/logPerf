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

var validHostname = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

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
	if c.BinaryPath == "" {
		return fmt.Errorf("iperf3 binary path is required")
	}
	return nil
}

// ToArgs converts the config into iperf3 CLI arguments.
// The -J flag (JSON output) is NOT included here — the runner adds it.
func (c *IperfConfig) ToArgs() []string {
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
	return args
}
