package iperf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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

// MinStreamVersion is the minimum iperf3 version required for --json-stream.
const MinStreamVersion = "3.17"

var versionRegex = regexp.MustCompile(`iperf (\d+\.\d+)`)

// CheckVersion runs iperf3 --version and returns the version string.
// Returns an error if the version is below MinStreamVersion.
func CheckVersion(binaryPath string) (string, error) {
	out, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run iperf3 --version: %w", err)
	}
	matches := versionRegex.FindSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not parse iperf3 version from: %s", strings.TrimSpace(string(out)))
	}
	version := string(matches[1])

	// Compare major.minor
	if !versionAtLeast(version, MinStreamVersion) {
		return version, fmt.Errorf("iperf3 %s found, but --json-stream requires >= %s", version, MinStreamVersion)
	}
	return version, nil
}

func versionAtLeast(have, want string) bool {
	haveParts := strings.SplitN(have, ".", 2)
	wantParts := strings.SplitN(want, ".", 2)
	if len(haveParts) != 2 || len(wantParts) != 2 {
		return false
	}
	haveMajor, _ := strconv.Atoi(haveParts[0])
	haveMinor, _ := strconv.Atoi(haveParts[1])
	wantMajor, _ := strconv.Atoi(wantParts[0])
	wantMinor, _ := strconv.Atoi(wantParts[1])
	if haveMajor != wantMajor {
		return haveMajor > wantMajor
	}
	return haveMinor >= wantMinor
}

// RunWithIntervals executes iperf3 with --json-stream --forceflush, calling
// onInterval for each interval measurement as it arrives. Returns the final
// TestResult after the process completes.
func (r *Runner) RunWithIntervals(ctx context.Context, cfg IperfConfig, onInterval func(*model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	args := cfg.ToArgs()
	args = append(args, "--json-stream", "--forceflush")

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

	var result *model.TestResult
	var intervals []model.IntervalResult
	startMeta := &model.TestResult{}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		ev, err := ParseStreamEvent(line)
		if err != nil {
			continue // skip unparseable lines
		}

		switch ev.Event {
		case "start":
			_ = ParseStartData(ev.Data, startMeta)
		case "interval":
			interval, err := ParseIntervalData(ev.Data)
			if err != nil {
				continue
			}
			intervals = append(intervals, *interval)
			if onInterval != nil && !interval.Omitted {
				onInterval(interval)
			}
		case "end":
			result, err = ParseEndData(ev.Data)
			if err != nil {
				continue
			}
		}
	}

	waitErr := cmd.Wait()

	if result == nil {
		if waitErr != nil {
			return nil, fmt.Errorf("iperf3 failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("iperf3 produced no end event")
	}

	// Merge start metadata into result
	result.ServerAddr = startMeta.ServerAddr
	result.Port = startMeta.Port
	result.Protocol = startMeta.Protocol
	result.Parallel = startMeta.Parallel
	result.Duration = startMeta.Duration
	if !startMeta.Timestamp.IsZero() {
		result.Timestamp = startMeta.Timestamp
	}
	result.Intervals = intervals

	return result, nil
}
