package iperf

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"iperf-tool/internal/model"
)

const DebugLogPath = "/tmp/iperf-debug.log"

// Runner executes iperf3 commands.
type Runner struct {
	mu                   sync.Mutex
	cmd                  *exec.Cmd
	stopped              bool // true after user-initiated Stop()
	supportsCongestion   bool
	checkedCongestion    bool
	congestionCheckMutex sync.Mutex
	debug                bool
}

// NewRunner creates a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// NewDebugRunner creates a Runner that logs all raw iperf3 stream output to
// DebugLogPath (/tmp/iperf-debug.log) for each RunWithIntervals call.
func NewDebugRunner() *Runner {
	return &Runner{debug: true}
}

// nopWriteCloser wraps io.Discard as an io.WriteCloser.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// debugWriter opens or creates DebugLogPath in append mode and writes a
// timestamped header line. Caller must close the returned WriteCloser.
// Returns a no-op writer if debug is disabled or the file cannot be opened.
func (r *Runner) debugWriter(args []string) (io.WriteCloser, func(string, ...any)) {
	nop := nopWriteCloser{io.Discard}
	nopf := func(string, ...any) {}
	if !r.debug {
		return nop, nopf
	}
	f, err := os.OpenFile(DebugLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iperf debug: cannot open log %s: %v\n", DebugLogPath, err)
		return nop, nopf
	}
	fmt.Fprintf(f, "\n=== %s ===\nargs: %v\n", time.Now().Format("2006-01-02 15:04:05"), args)
	logf := func(format string, a ...any) { fmt.Fprintf(f, format, a...) }
	return f, logf
}

// checkCongestionSupport checks once if iperf3 supports -C flag and caches the result.
func (r *Runner) checkCongestionSupport(binaryPath string) bool {
	r.congestionCheckMutex.Lock()
	defer r.congestionCheckMutex.Unlock()

	if !r.checkedCongestion {
		r.supportsCongestion = SupportsCongestionControl(binaryPath)
		r.checkedCongestion = true
	}
	return r.supportsCongestion
}

// Stop sends SIGTERM to the running iperf3 process, allowing it to finish
// gracefully and produce a summary. This is equivalent to the test ending
// normally when its duration expires.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil && r.cmd.Process != nil {
		r.stopped = true
		r.cmd.Process.Signal(syscall.SIGTERM)
	}
}

func (r *Runner) setCmd(cmd *exec.Cmd) {
	r.mu.Lock()
	r.cmd = cmd
	r.stopped = false
	r.mu.Unlock()
}

func (r *Runner) clearCmd() {
	r.mu.Lock()
	r.cmd = nil
	r.mu.Unlock()
}

// Run executes iperf3 with JSON output and returns the raw JSON bytes.
func (r *Runner) Run(_ context.Context, cfg IperfConfig) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	supportsCongestion := r.checkCongestionSupport(cfg.BinaryPath)
	args := cfg.ToArgs(supportsCongestion)
	args = append(args, "-J")

	cmd := exec.Command(cfg.BinaryPath, args...)
	r.setCmd(cmd)
	defer r.clearCmd()

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
func (r *Runner) RunWithPipe(_ context.Context, cfg IperfConfig, onLine func(string)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	supportsCongestion := r.checkCongestionSupport(cfg.BinaryPath)
	args := cfg.ToArgs(supportsCongestion)
	args = append(args, "-J")

	cmd := exec.Command(cfg.BinaryPath, args...)
	r.setCmd(cmd)
	defer r.clearCmd()

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

// SupportsCongestionControl tests if iperf3 supports the -C flag.
// This is platform-dependent (e.g., not supported on macOS).
func SupportsCongestionControl(binaryPath string) bool {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// Check if help output mentions -C flag
	return bytes.Contains(out, []byte("-C, --congestion"))
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
// onInterval for each interval measurement as it arrives. In bidirectional
// mode rev is the paired reverse interval; it is nil in all other modes.
// Returns the final TestResult after the process completes.
func (r *Runner) RunWithIntervals(_ context.Context, cfg IperfConfig, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	supportsCongestion := r.checkCongestionSupport(cfg.BinaryPath)
	args := cfg.ToArgs(supportsCongestion)
	args = append(args, "--json-stream", "--forceflush")

	cmd := exec.Command(cfg.BinaryPath, args...)
	r.setCmd(cmd)
	defer r.clearCmd()

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
	var fwdIntervals, revIntervals []model.IntervalResult
	startMeta := &model.TestResult{}

	var streamErr string
	var serverOutputText string

	dbg, logf := r.debugWriter(args)
	defer dbg.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		logf("%s\n", line)

		ev, err := ParseStreamEvent(line)
		if err != nil {
			logf("[parse error: %v]\n", err)
			continue // skip unparseable lines
		}

		switch ev.Event {
		case "start":
			_ = ParseStartData(ev.Data, startMeta)
		case "interval":
			fwd, rev, err := ParseIntervalData(ev.Data)
			if err != nil {
				logf("[interval parse error: %v]\n", err)
				continue
			}
			logf("[interval] fwd=%.2fMbps omitted=%v rev=%v\n", fwd.BandwidthBps/1e6, fwd.Omitted, rev != nil)
			fwdIntervals = append(fwdIntervals, *fwd)
			if rev != nil {
				revIntervals = append(revIntervals, *rev)
			}
			if onInterval != nil && !fwd.Omitted {
				onInterval(fwd, rev)
			}
		case "server_output_text":
			var txt string
			if err := json.Unmarshal(ev.Data, &txt); err == nil {
				serverOutputText += txt
			}
			logf("[server_output_text] %d bytes\n", len(ev.Data))
		case "error":
			// iperf3 reports errors as JSON string in data field
			_ = json.Unmarshal(ev.Data, &streamErr)
			logf("[stream error] %s\n", streamErr)
		case "end":
			result, err = ParseEndData(ev.Data)
			if err != nil {
				logf("[end parse error: %v]\n", err)
				continue
			}
			logf("[end] sentBps=%.2fMbps fwdIntervals=%d revIntervals=%d fwdJitter=%.4fms\n",
				result.SentBps/1e6, len(fwdIntervals), len(revIntervals), result.FwdJitterMs)
		}
	}
	if err := scanner.Err(); err != nil {
		logf("[scanner error: %v]\n", err)
	}

	waitErr := cmd.Wait()

	if result == nil {
		// No valid end event — a stream error explains why (e.g. "server is busy").
		if streamErr != "" {
			return nil, fmt.Errorf("iperf3: %s", streamErr)
		}
		if waitErr != nil {
			return nil, fmt.Errorf("iperf3 failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("iperf3 produced no end event")
	}
	// If a stream error was received before the end event, the end event data
	// will be empty (zero result). If it was user-initiated (Stop()), keep the
	// partial result. Otherwise treat it as a fatal error (e.g. server busy).
	r.mu.Lock()
	userStopped := r.stopped
	r.mu.Unlock()
	result.Interrupted = userStopped
	if streamErr != "" && !userStopped {
		return nil, fmt.Errorf("iperf3: %s", streamErr)
	}
	// A SIGTERM from Stop() or natural expiry does not invalidate partial results.

	// Merge start metadata into result
	result.ServerAddr = startMeta.ServerAddr
	result.Port = startMeta.Port
	result.Protocol = strings.ToUpper(startMeta.Protocol)
	result.Parallel = startMeta.Parallel
	result.Duration = startMeta.Duration
	if !startMeta.Timestamp.IsZero() {
		result.Timestamp = startMeta.Timestamp
	}
	result.Intervals = fwdIntervals
	if len(revIntervals) > 0 {
		result.ReverseIntervals = revIntervals
		// For UDP bidir, the end event's sum_sent_bidir_reverse reflects the
		// server's sent rate (before packet loss), not what the client received.
		// sum_received_bidir_reverse is 0 in JSON-stream mode for UDP.
		// Compute ReverseReceivedBps from the interval averages (client-side
		// received), which is the accurate delivery metric.
		if result.Protocol == "UDP" {
			var sum float64
			n := 0
			for _, iv := range revIntervals {
				if !iv.Omitted {
					sum += iv.BandwidthBps
					n++
				}
			}
			if n > 0 {
				result.ReverseReceivedBps = sum / float64(n)
			}
		}
	}
	// Derive actual duration from last non-omitted forward interval's end time.
	for i := len(result.Intervals) - 1; i >= 0; i-- {
		if !result.Intervals[i].Omitted {
			result.ActualDuration = result.Intervals[i].TimeEnd
			break
		}
	}

	// Overlay server-measured metrics from server_output_text when
	// server_output_json was not available (typical in --json-stream mode).
	if serverOutputText != "" && result.FwdReceivedBps == 0 {
		ParseServerOutputText(serverOutputText, result, cfg.Bidir)
		logf("[server_output_text parsed] FwdReceivedBps=%.2fMbps FwdLost=%d FwdPkts=%d\n",
			result.FwdReceivedBps/1e6, result.FwdLostPackets, result.FwdPackets)
	}

	return result, nil
}
