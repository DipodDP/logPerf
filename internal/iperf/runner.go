package iperf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"iperf-tool/internal/model"
)

// Runner executes iperf3 commands.
type Runner struct{}

// NewRunner creates a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run executes iperf3 with JSON output and returns the raw JSON bytes.
func (r *Runner) Run(ctx context.Context, cfg IperfConfig) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	args := cfg.ToArgs()
	args = append(args, "-J")

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If we got JSON output despite the error, return it (iperf3 may
		// report errors inside the JSON).
		if stdout.Len() > 0 {
			return stdout.Bytes(), nil
		}
		return nil, fmt.Errorf("iperf3 failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// RunWithPipe executes iperf3 with JSON output, calling onLine for each line
// of stdout as it arrives (for live GUI updates). It returns the parsed
// TestResult after the process completes.
func (r *Runner) RunWithPipe(ctx context.Context, cfg IperfConfig, onLine func(string)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	args := cfg.ToArgs()
	args = append(args, "-J")

	cmd := exec.CommandContext(ctx, cfg.BinaryPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start iperf3: %w", err)
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")
		if onLine != nil {
			onLine(line)
		}
	}

	if err := cmd.Wait(); err != nil {
		if buf.Len() > 0 {
			// Try parsing even on error — iperf3 may have produced valid JSON.
			result, parseErr := ParseResult(buf.Bytes())
			if parseErr == nil {
				return result, nil
			}
		}
		return nil, fmt.Errorf("iperf3 failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return ParseResult(buf.Bytes())
}
