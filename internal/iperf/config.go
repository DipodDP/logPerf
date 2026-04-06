package iperf

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iperf-tool/internal/model"
)

// Config holds the parameters for an iperf2 test run.
type Config struct {
	BinaryPath       string        // path to iperf2 binary (default "iperf")
	ServerAddr       string        // target server hostname or IP
	Port             int           // server port (default 5201)
	Parallel         int           // number of parallel streams (maps to port range)
	Duration         int           // test duration in seconds
	Interval         int           // reporting interval in seconds
	Protocol         string        // "tcp" or "udp"
	BlockSize        int           // buffer/datagram size in bytes (0 = iperf2 default)
	MeasurePing      bool          // run ping before and during test
	Reverse          bool          // reverse direction (remote→local)
	Bidir            bool          // bidirectional (both directions simultaneously)
	Bandwidth        string        // -b: target bandwidth (e.g. "100M", "1G"), empty = unlimited
	Enhanced         bool          // -e flag (enhanced output with latency, PPS, etc.)
	LocalAddr        string        // local IP address for reverse/bidir connections
	SSHFallback      bool          // use SSH file fallback for server-side data
	RemoteOutputFile string        // file path on remote host for server output
	IsWindows        bool          // remote host is Windows
	ProbeTimeout     time.Duration // UDP probe timeout, default 2s
	SkipProbe        bool          // skip pre-flight UDP reachability probe
	KillWaitMs       int           // post-kill wait before reading file, default 500
	IPv6             bool          // Use IPv6 (-V flag)
}

// IperfConfig is an alias for Config to ease the migration.
// Existing code that references IperfConfig will continue to compile.
type IperfConfig = Config

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BinaryPath:   "iperf",
		Port:         5201,
		Parallel:     1,
		Duration:     10,
		Interval:     1,
		Protocol:     "tcp",
		ProbeTimeout: 2 * time.Second,
		KillWaitMs:   500,
	}
}

var (
	validHostname  = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)
	validBandwidth = regexp.MustCompile(`^\d+[KMGkmg]?$`)
)

// Validate checks the config for invalid or dangerous values.
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
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
		return fmt.Errorf("iperf binary path is required")
	}
	if c.Reverse && c.Bidir {
		return fmt.Errorf("reverse (-R) and bidirectional (--bidir) are mutually exclusive")
	}
	if c.Bandwidth != "" && !validBandwidth.MatchString(c.Bandwidth) {
		return fmt.Errorf("bandwidth must match pattern digits[KMG], got %q", c.Bandwidth)
	}
	// Check port range fits for bidir (forward + reverse need separate ranges)
	if c.Bidir {
		if c.Port+c.Parallel*2-1 > 65535 {
			return fmt.Errorf("bidirectional port range exceeds 65535: need %d ports starting at %d", c.Parallel*2, c.Port)
		}
	} else if c.Port+c.Parallel-1 > 65535 {
		return fmt.Errorf("port range exceeds 65535: need %d ports starting at %d", c.Parallel, c.Port)
	}
	if c.SSHFallback && c.RemoteOutputFile == "" {
		return fmt.Errorf("remote output file path is required when SSH fallback is enabled")
	}
	return nil
}

// parseBandwidthBits parses a bandwidth string (e.g. "50M", "500K", "1G", "100")
// and returns the value in bits per second. Returns 0 on parse error.
func parseBandwidthBits(bw string) float64 {
	if bw == "" {
		return 0
	}
	s := strings.ToUpper(bw)
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

// BandwidthPerStreamMbps returns the per-stream target in Mbps.
// iperf2 applies the -b value per stream, so this returns the -b value directly.
// Returns 0 if Bandwidth is empty (unlimited).
func (c *Config) BandwidthPerStreamMbps() float64 {
	bits := parseBandwidthBits(c.Bandwidth)
	if bits == 0 {
		return 0
	}
	return bits / 1_000_000
}

const (
	// DefaultUDPBlockSize is the iperf2 default datagram size for UDP tests (bytes).
	DefaultUDPBlockSize = 1470
	// DefaultTCPBlockSize is the iperf2 default send buffer size for TCP tests (bytes).
	DefaultTCPBlockSize = 131072
)

// ApplyToResult sets result fields from cfg, using config values as authoritative
// overrides (handles partial runs where parsed values may be empty).
// mode should be "CLI" or "GUI".
func (c *Config) ApplyToResult(result *model.TestResult, mode string) {
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
	} else {
		result.Direction = "Forward"
	}
	if bw := c.BandwidthPerStreamMbps(); bw > 0 {
		result.Bandwidth = fmt.Sprintf("%.2f", bw)
	} else if isUDP {
		udpDefault := Config{Bandwidth: "1M", Parallel: c.Parallel}
		result.Bandwidth = fmt.Sprintf("%.2f", udpDefault.BandwidthPerStreamMbps())
	}
	result.Mode = mode
}

// PortRangeStr returns a port range string for iperf2 -p flag.
// offset shifts the starting port (0 for forward, Parallel for reverse).
// Single port: "5201". Multiple: "5201-5202".
func (c *Config) PortRangeStr(offset int) string {
	start := c.Port + offset
	n := c.Parallel
	if n <= 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d-%d", start, start+n-1)
}

// fwdServerArgs returns server args for the forward direction (remote side receives).
func (c *Config) fwdServerArgs() []string {
	args := []string{"-s"}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	args = append(args, "-p", c.PortRangeStr(0), "-f", "m", "-i", strconv.Itoa(c.Interval))
	if c.Enhanced {
		args = append(args, "-e")
	}
	if c.RemoteOutputFile != "" {
		args = append(args, "-o", c.RemoteOutputFile)
	}
	if c.IPv6 {
		args = append(args, "-V")
	}
	return args
}

// fwdClientArgs returns client args for forward direction (local side sends).
func (c *Config) fwdClientArgs() []string {
	args := []string{"-c", c.ServerAddr}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	args = append(args, "-p", c.PortRangeStr(0),
		"-t", strconv.Itoa(c.Duration),
		"-f", "m",
		"-i", strconv.Itoa(c.Interval))
	if c.BlockSize > 0 {
		args = append(args, "-l", strconv.Itoa(c.BlockSize))
	}
	if c.Bandwidth != "" && c.Protocol == "udp" {
		args = append(args, "-b", c.Bandwidth)
	}
	if c.Enhanced {
		args = append(args, "-e")
	}
	if c.IPv6 {
		args = append(args, "-V")
	}
	return args
}

// revServerArgs returns server args for reverse direction (local side receives).
func (c *Config) revServerArgs() []string {
	args := []string{"-s"}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	args = append(args, "-p", c.PortRangeStr(c.Parallel),
		"-f", "m",
		"-i", strconv.Itoa(c.Interval))
	if c.Enhanced {
		args = append(args, "-e")
	}
	if c.IPv6 {
		args = append(args, "-V")
	}
	return args
}

// revClientCmd returns the remote client command string for reverse direction
// (remote side sends to local).
func (c *Config) revClientCmd() string {
	binary := "iperf"
	if c.IsWindows {
		binary = "iperf.exe"
	}
	parts := []string{binary, "-c", c.LocalAddr}
	if c.Protocol == "udp" {
		parts = append(parts, "-u")
	}
	parts = append(parts, "-p", c.PortRangeStr(c.Parallel),
		"-t", strconv.Itoa(c.Duration),
		"-f", "m",
		"-i", strconv.Itoa(c.Interval))
	if c.BlockSize > 0 {
		parts = append(parts, "-l", strconv.Itoa(c.BlockSize))
	}
	if c.Bandwidth != "" && c.Protocol == "udp" {
		parts = append(parts, "-b", c.Bandwidth)
	}
	if c.Enhanced {
		parts = append(parts, "-e")
	}
	if c.IPv6 {
		parts = append(parts, "-V")
	}
	return strings.Join(parts, " ")
}

// dualtestClientArgs returns client args for iperf2's native bidirectional
// dualtest mode (-d flag). This runs both directions in a single process.
func (c *Config) dualtestClientArgs() []string {
	args := []string{"-c", c.ServerAddr}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	args = append(args, "-d") // dualtest flag
	args = append(args, "-p", c.PortRangeStr(0),
		"-t", strconv.Itoa(c.Duration),
		"-f", "m",
		"-i", strconv.Itoa(c.Interval))
	if c.BlockSize > 0 {
		args = append(args, "-l", strconv.Itoa(c.BlockSize))
	}
	if c.Bandwidth != "" && c.Protocol == "udp" {
		args = append(args, "-b", c.Bandwidth)
	}
	if c.Enhanced {
		args = append(args, "-e")
	}
	if c.IPv6 {
		args = append(args, "-V")
	}
	return args
}

// remoteServerStartCmd returns the SSH command to start the remote iperf server.
// On Unix, nohup backgrounds the process so it survives SSH session close.
// On Windows, schtasks detaches the process from the SSH session.
func (c *Config) remoteServerStartCmd() string {
	args := c.fwdServerArgs()
	if c.IsWindows {
		argStr := strings.Join(args, " ")
		// Use WMI to start a detached process — schtasks fails in non-interactive
		// SSH sessions (Interactive only logon mode).
		return fmt.Sprintf(
			`powershell -Command "Invoke-WmiMethod -Class Win32_Process -Name Create -ArgumentList 'iperf.exe %s'"`,
			argStr)
	}
	return fmt.Sprintf("nohup iperf %s > /dev/null 2>&1 &", strings.Join(args, " "))
}

// remoteServerKillCmd returns the SSH command to kill the remote iperf server.
func (c *Config) remoteServerKillCmd() string {
	if c.IsWindows {
		return `taskkill /IM iperf.exe /F`
	}
	return "pkill -f 'iperf -s'"
}

// remoteServerReadCmd returns the SSH command to read the remote server output file.
// Only used when SSHFallback is true.
func (c *Config) remoteServerReadCmd() string {
	if c.IsWindows {
		return fmt.Sprintf("type %s", c.RemoteOutputFile)
	}
	return fmt.Sprintf("cat %s", c.RemoteOutputFile)
}
