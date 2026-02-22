package iperf

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"iperf-tool/internal/model"
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

// parseBandwidthBits parses a bandwidth string (e.g. "50M", "500K", "1G", "100")
// and returns the value in bits per second. Returns 0 on parse error.
func parseBandwidthBits(bw string) float64 {
	if bw == "" {
		return 0
	}
	s := bw
	mult := 1.0
	switch s[len(s)-1] {
	case 'K':
		mult = 1_000
		s = s[:len(s)-1]
	case 'M':
		mult = 1_000_000
		s = s[:len(s)-1]
	case 'G':
		mult = 1_000_000_000
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v * mult
}

// BandwidthPerStreamMbps returns the per-stream target in Mbps
// (total Bandwidth / number of parallel streams).
// Returns 0 if Bandwidth is empty (unlimited).
func (c *IperfConfig) BandwidthPerStreamMbps() float64 {
	bits := parseBandwidthBits(c.Bandwidth)
	if bits == 0 {
		return 0
	}
	streams := float64(c.Parallel)
	if streams < 1 {
		streams = 1
	}
	return bits / streams / 1_000_000
}

const (
	// DefaultUDPBlockSize is the iperf3 default datagram size for UDP tests (bytes).
	DefaultUDPBlockSize = 1460
	// DefaultTCPBlockSize is the iperf3 default send buffer size for TCP tests (bytes).
	DefaultTCPBlockSize = 131072
)

// ApplyToResult sets result fields from cfg, using config values as authoritative
// overrides (handles partial runs where parsed values may be empty).
// mode should be "CLI" or "GUI".
func (c *IperfConfig) ApplyToResult(result *model.TestResult, mode string) {
	if c.ServerAddr != "" {
		result.ServerAddr = c.ServerAddr
	}
	if c.Port != 0 {
		result.Port = c.Port
	}
	if c.Protocol != "" {
		result.Protocol = strings.ToUpper(c.Protocol)
	}
	if c.Duration != 0 {
		result.Duration = c.Duration
	}
	if c.Parallel != 0 {
		result.Parallel = c.Parallel
	}
	isUDP := strings.EqualFold(c.Protocol, "udp")
	switch {
	case c.BlockSize > 0:
		result.BlockSize = c.BlockSize
	case isUDP:
		result.BlockSize = DefaultUDPBlockSize
	default:
		result.BlockSize = DefaultTCPBlockSize
	}
	if c.Reverse {
		result.Direction = "Reverse"
	} else if c.Bidir {
		result.Direction = "Bidirectional"
	}
	if bw := c.BandwidthPerStreamMbps(); bw > 0 {
		result.Bandwidth = fmt.Sprintf("%.2f", bw)
	} else if isUDP {
		udpDefault := IperfConfig{Bandwidth: "1M", Parallel: c.Parallel}
		result.Bandwidth = fmt.Sprintf("%.2f", udpDefault.BandwidthPerStreamMbps())
	}
	result.Mode = mode
}

// bandwidthPerStreamArg returns the per-stream bandwidth as an iperf3 -b
// argument string (integer bits/sec). Returns "" if Bandwidth is empty.
func (c *IperfConfig) bandwidthPerStreamArg() string {
	bits := parseBandwidthBits(c.Bandwidth)
	if bits == 0 {
		return ""
	}
	streams := float64(c.Parallel)
	if streams < 1 {
		streams = 1
	}
	return strconv.FormatInt(int64(bits/streams), 10)
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
	if bwArg := c.bandwidthPerStreamArg(); bwArg != "" {
		args = append(args, "-b", bwArg)
	}
	if c.Congestion != "" && supportsCongestion {
		args = append(args, "-C", c.Congestion)
	}
	args = append(args, "--get-server-output")
	return args
}
