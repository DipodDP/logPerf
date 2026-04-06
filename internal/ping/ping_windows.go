//go:build windows

package ping

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// statsRe matches the RTT summary line from Windows ping output.
// Example: "    Minimum = 1ms, Maximum = 5ms, Average = 3ms"
var statsRe = regexp.MustCompile(`Minimum\s*=\s*(\d+)\s*ms,\s*Maximum\s*=\s*(\d+)\s*ms,\s*Average\s*=\s*(\d+)\s*ms`)

// lossRe matches the packet loss summary line from Windows ping output.
// Example: "    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),"
var lossRe = regexp.MustCompile(`Sent\s*=\s*(\d+),\s*Received\s*=\s*(\d+),\s*Lost\s*=\s*\d+\s*\((\d+)%\s*loss\)`)

// Run executes ping with a fixed count and returns the parsed result.
func Run(ctx context.Context, host string, count int) (*Result, error) {
	cmd := exec.CommandContext(ctx, "ping", "-n", strconv.Itoa(count), host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("ping failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return ParseOutput(stdout.String())
}

// RunUntilCancel runs ping continuously until the context is cancelled.
// Uses -t flag for continuous ping on Windows.
func RunUntilCancel(ctx context.Context, host string) (*Result, error) {
	cmd := exec.CommandContext(ctx, "ping", "-t", host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Cancel = func() error {
		return cmd.Process.Signal(sigInterrupt())
	}
	cmd.WaitDelay = 0

	err := cmd.Run()
	output := stdout.String()
	if err != nil && ctx.Err() != nil && len(output) > 0 {
		return ParseOutput(output)
	}
	if err != nil {
		return nil, fmt.Errorf("ping failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return ParseOutput(output)
}

// ParseOutput extracts ping statistics from Windows ping command output.
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
