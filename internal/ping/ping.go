package ping

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"iperf-tool/internal/model"
)

// Result holds parsed ping summary statistics.
type Result struct {
	PacketsSent int
	PacketsRecv int
	PacketLoss  float64
	MinMs       float64
	AvgMs       float64
	MaxMs       float64
}

// ToModel converts a ping Result to the model representation.
func (r *Result) ToModel() *model.PingResult {
	if r == nil {
		return nil
	}
	return &model.PingResult{
		PacketsSent: r.PacketsSent,
		PacketsRecv: r.PacketsRecv,
		PacketLoss:  r.PacketLoss,
		MinMs:       r.MinMs,
		AvgMs:       r.AvgMs,
		MaxMs:       r.MaxMs,
	}
}

// statsRe matches the rtt summary line from ping output on macOS and Linux.
// Example: "round-trip min/avg/max/stddev = 1.234/5.678/9.012/1.234 ms"
// Example: "rtt min/avg/max/mdev = 1.234/5.678/9.012/1.234 ms"
var statsRe = regexp.MustCompile(`(?:round-trip|rtt)\s+min/avg/max/(?:std|m)dev\s*=\s*([\d.]+)/([\d.]+)/([\d.]+)`)

// lossRe matches the packet loss summary line.
// Example: "4 packets transmitted, 4 received, 0% packet loss"
// Example: "4 packets transmitted, 4 packets received, 0.0% packet loss"
var lossRe = regexp.MustCompile(`(\d+)\s+packets?\s+transmitted,\s+(\d+)\s+(?:packets?\s+)?received,\s+([\d.]+)%\s+packet loss`)

// Run executes ping with a fixed count and returns the parsed result.
func Run(ctx context.Context, host string, count int) (*Result, error) {
	cmd := exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(count), host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// ping returns exit code 1 on partial loss — still parse output
	if err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("ping failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return ParseOutput(stdout.String())
}

// RunUntilCancel runs ping continuously until the context is cancelled.
// On cancellation it sends SIGINT so ping prints its summary, then parses output.
func RunUntilCancel(ctx context.Context, host string) (*Result, error) {
	cmd := exec.CommandContext(ctx, "ping", host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// When context is cancelled, CommandContext sends SIGKILL by default.
	// We want SIGINT so ping prints the summary line.
	cmd.Cancel = func() error {
		return cmd.Process.Signal(sigInterrupt())
	}
	cmd.WaitDelay = 0 // wait for output after signal

	err := cmd.Run()
	output := stdout.String()
	// Context cancellation is expected — try to parse what we got
	if err != nil && ctx.Err() != nil && len(output) > 0 {
		return ParseOutput(output)
	}
	if err != nil {
		return nil, fmt.Errorf("ping failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return ParseOutput(output)
}

// ParseOutput extracts ping statistics from raw ping command output.
func ParseOutput(output string) (*Result, error) {
	r := &Result{}

	lm := lossRe.FindStringSubmatch(output)
	if lm == nil {
		return nil, fmt.Errorf("could not parse packet loss from ping output")
	}
	r.PacketsSent, _ = strconv.Atoi(lm[1])
	r.PacketsRecv, _ = strconv.Atoi(lm[2])
	r.PacketLoss, _ = strconv.ParseFloat(lm[3], 64)

	sm := statsRe.FindStringSubmatch(output)
	if sm == nil {
		// 100% loss — no RTT stats available
		return r, nil
	}
	r.MinMs, _ = strconv.ParseFloat(sm[1], 64)
	r.AvgMs, _ = strconv.ParseFloat(sm[2], 64)
	r.MaxMs, _ = strconv.ParseFloat(sm[3], 64)

	return r, nil
}
