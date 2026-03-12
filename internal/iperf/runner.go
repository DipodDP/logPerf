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
	secondRunner         *Runner // used by RunBidir for the reverse test
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
// normally when its duration expires. In bidir mode, also stops the second runner.
func (r *Runner) Stop() {
	r.mu.Lock()
	second := r.secondRunner
	if r.cmd != nil && r.cmd.Process != nil {
		r.stopped = true
		r.cmd.Process.Signal(syscall.SIGTERM)
	}
	r.mu.Unlock()
	if second != nil {
		second.Stop()
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
				if result.Error != "" {
					return result, fmt.Errorf("iperf3: %s", result.Error)
				}
				return result, nil
			}
		}
		return nil, fmt.Errorf("iperf3 failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	result, err := ParseResult(buf.Bytes())
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return result, fmt.Errorf("iperf3: %s", result.Error)
	}
	return result, nil
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

// RunBidir runs two simultaneous iperf3 processes — one forward and one
// reverse — and combines them into a single bidirectional TestResult.
// The reverse test uses port+1 because iperf3 servers only handle one
// client at a time. This replaces the broken --bidir flag which fails
// on Windows/Cygwin servers.
func (r *Runner) RunBidir(ctx context.Context, cfg IperfConfig, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	// Build forward and reverse configs — neither uses --bidir.
	// Reverse test uses port+1 since iperf3 handles one client per port.
	fwdCfg := cfg
	fwdCfg.Bidir = false
	fwdCfg.Reverse = false

	revCfg := cfg
	revCfg.Bidir = false
	revCfg.Reverse = true
	revCfg.Port = cfg.Port + 1

	// Create a second runner for the reverse test so Stop() can kill both.
	var revRunner *Runner
	if r.debug {
		revRunner = NewDebugRunner()
	} else {
		revRunner = NewRunner()
	}
	r.mu.Lock()
	r.secondRunner = revRunner
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.secondRunner = nil
		r.mu.Unlock()
	}()

	type testOutput struct {
		result *model.TestResult
		err    error
	}

	fwdCh := make(chan testOutput, 1)
	revCh := make(chan testOutput, 1)

	// Buffer the latest reverse interval so the forward callback can pair them.
	// We also buffer the first forward interval: if reverse hasn't reported yet,
	// we hold it and replay once the first reverse sample arrives.
	var revMu sync.Mutex
	var lastRev *model.IntervalResult
	var pendingFwd *model.IntervalResult // first fwd held until rev is available

	// Launch forward test.
	go func() {
		res, err := r.RunWithIntervals(ctx, fwdCfg, func(fwd, _ *model.IntervalResult) {
			if onInterval != nil {
				revMu.Lock()
				rev := lastRev
				if rev == nil {
					// No reverse data yet — hold this fwd for later.
					pendingFwd = fwd
					revMu.Unlock()
					return
				}
				revMu.Unlock()
				onInterval(fwd, rev)
			}
		})
		fwdCh <- testOutput{res, err}
	}()

	// Launch reverse test — buffer intervals silently for the forward callback.
	go func() {
		res, err := revRunner.RunWithIntervals(ctx, revCfg, func(rev, _ *model.IntervalResult) {
			revMu.Lock()
			lastRev = rev
			// If a forward interval was waiting for the first reverse sample,
			// replay it now with the real reverse data.
			held := pendingFwd
			pendingFwd = nil
			revMu.Unlock()
			if held != nil && onInterval != nil {
				onInterval(held, rev)
			}
		})
		revCh <- testOutput{res, err}
	}()

	fwdOut := <-fwdCh
	revOut := <-revCh

	if fwdOut.err != nil {
		return nil, fmt.Errorf("forward test: %w", fwdOut.err)
	}
	if revOut.err != nil {
		return nil, fmt.Errorf("reverse test: %w", revOut.err)
	}

	// Combine into a single bidirectional result.
	result := fwdOut.result
	result.Direction = "Bidirectional"

	rev := revOut.result
	result.ReverseSentBps = rev.SentBps
	result.ReverseReceivedBps = rev.ReceivedBps
	result.ReverseRetransmits = rev.Retransmits
	result.ReverseBytesSent = rev.BytesSent
	result.ReverseBytesReceived = rev.BytesReceived
	result.ReverseLostPackets = rev.LostPackets
	result.ReverseLostPercent = rev.LostPercent
	result.ReversePackets = rev.Packets
	result.ReverseJitterMs = rev.JitterMs
	result.ReverseIntervals = rev.Intervals

	// Propagate interrupted flag if either test was stopped.
	result.Interrupted = result.Interrupted || rev.Interrupted

	return result, nil
}

// RunBidirPipe is like RunBidir but uses RunWithPipe (standard -J mode)
// instead of RunWithIntervals (--json-stream). Used as a fallback when
// --json-stream fails (e.g. UDP EAGAIN bug on some servers).
func (r *Runner) RunBidirPipe(ctx context.Context, cfg IperfConfig, onLine func(string)) (*model.TestResult, error) {
	fwdCfg := cfg
	fwdCfg.Bidir = false
	fwdCfg.Reverse = false

	revCfg := cfg
	revCfg.Bidir = false
	revCfg.Reverse = true
	revCfg.Port = cfg.Port + 1

	var revRunner *Runner
	if r.debug {
		revRunner = NewDebugRunner()
	} else {
		revRunner = NewRunner()
	}
	r.mu.Lock()
	r.secondRunner = revRunner
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.secondRunner = nil
		r.mu.Unlock()
	}()

	type testOutput struct {
		result *model.TestResult
		err    error
	}

	fwdCh := make(chan testOutput, 1)
	revCh := make(chan testOutput, 1)

	go func() {
		res, err := r.RunWithPipe(ctx, fwdCfg, onLine)
		fwdCh <- testOutput{res, err}
	}()
	go func() {
		res, err := revRunner.RunWithPipe(ctx, revCfg, onLine)
		revCh <- testOutput{res, err}
	}()

	fwdOut := <-fwdCh
	revOut := <-revCh

	if fwdOut.err != nil {
		return nil, fmt.Errorf("forward test: %w", fwdOut.err)
	}
	if revOut.err != nil {
		return nil, fmt.Errorf("reverse test: %w", revOut.err)
	}

	result := fwdOut.result
	result.Direction = "Bidirectional"

	rev := revOut.result
	result.ReverseSentBps = rev.SentBps
	result.ReverseReceivedBps = rev.ReceivedBps
	result.ReverseRetransmits = rev.Retransmits
	result.ReverseBytesSent = rev.BytesSent
	result.ReverseBytesReceived = rev.BytesReceived
	result.ReverseLostPackets = rev.LostPackets
	result.ReverseLostPercent = rev.LostPercent
	result.ReversePackets = rev.Packets
	result.ReverseJitterMs = rev.JitterMs
	result.ReverseIntervals = rev.Intervals

	result.Interrupted = result.Interrupted || rev.Interrupted

	return result, nil
}

// RunBidirParallel runs N forward + N reverse iperf3 instances on separate ports,
// each with -P 1, to work around the Windows/Cygwin EAGAIN bug with UDP parallel
// streams. Forward instances use ports base, base+2, base+4, ... and reverse
// instances use ports base+1, base+3, base+5, ...
// The bandwidth is split evenly across the N instances.
// onInterval is called with aggregated forward/reverse intervals as they arrive.
func (r *Runner) RunBidirParallel(ctx context.Context, cfg IperfConfig, n int, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if n < 1 {
		n = 1
	}

	type testOutput struct {
		result *model.TestResult
		err    error
	}

	runners := make([]*Runner, 0, 2*n)
	fwdChs := make([]chan testOutput, n)
	revChs := make([]chan testOutput, n)

	// Split bandwidth across instances.
	perInstanceBW := splitBandwidth(cfg.Bandwidth, n)

	// Aggregator for live interval callbacks from N forward + N reverse instances.
	agg := newIntervalAggregator(n, n, onInterval)

	for i := 0; i < n; i++ {
		fwdCfg := cfg
		fwdCfg.Bidir = false
		fwdCfg.Reverse = false
		fwdCfg.Port = cfg.Port + i*2
		fwdCfg.Parallel = 1
		fwdCfg.Bandwidth = perInstanceBW

		revCfg := cfg
		revCfg.Bidir = false
		revCfg.Reverse = true
		revCfg.Port = cfg.Port + i*2 + 1
		revCfg.Parallel = 1
		revCfg.Bandwidth = perInstanceBW

		var fwdRunner, revRunner *Runner
		if i == 0 {
			fwdRunner = r
		} else if r.debug {
			fwdRunner = NewDebugRunner()
		} else {
			fwdRunner = NewRunner()
		}
		if r.debug {
			revRunner = NewDebugRunner()
		} else {
			revRunner = NewRunner()
		}
		runners = append(runners, fwdRunner, revRunner)

		fwdChs[i] = make(chan testOutput, 1)
		revChs[i] = make(chan testOutput, 1)

		fwdIdx := i
		go func(runner *Runner, c IperfConfig, ch chan testOutput) {
			res, err := runner.RunWithIntervals(ctx, c, func(fwd, _ *model.IntervalResult) {
				agg.addForward(fwdIdx, fwd)
			})
			ch <- testOutput{res, err}
		}(fwdRunner, fwdCfg, fwdChs[i])

		revIdx := i
		go func(runner *Runner, c IperfConfig, ch chan testOutput) {
			res, err := runner.RunWithIntervals(ctx, c, func(rev, _ *model.IntervalResult) {
				agg.addReverse(revIdx, rev)
			})
			ch <- testOutput{res, err}
		}(revRunner, revCfg, revChs[i])
	}

	// Store all runners so Stop() can kill them all.
	r.mu.Lock()
	if n > 1 {
		r.secondRunner = runners[len(runners)-1]
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.secondRunner = nil
		r.mu.Unlock()
	}()

	// Collect forward results.
	var fwdResults []*model.TestResult
	for i := 0; i < n; i++ {
		out := <-fwdChs[i]
		if out.err != nil {
			return nil, fmt.Errorf("forward instance %d: %w", i, out.err)
		}
		fwdResults = append(fwdResults, out.result)
	}

	// Collect reverse results.
	var revResults []*model.TestResult
	for i := 0; i < n; i++ {
		out := <-revChs[i]
		if out.err != nil {
			return nil, fmt.Errorf("reverse instance %d: %w", i, out.err)
		}
		revResults = append(revResults, out.result)
	}

	// Merge forward results.
	result := mergeResults(fwdResults)
	result.Direction = "Bidirectional"
	result.Parallel = cfg.Parallel

	// In multi-instance mode each forward instance is a standalone test where
	// sum_received reflects the server's perspective. Use ReceivedBps as the
	// server-received metric if FwdReceivedBps was not set via server_output_json.
	if result.FwdReceivedBps == 0 && result.ReceivedBps > 0 {
		result.FwdReceivedBps = result.ReceivedBps
	}
	if result.FwdPackets == 0 && result.Packets > 0 {
		result.FwdLostPackets = result.LostPackets
		result.FwdPackets = result.Packets
		result.FwdLostPercent = result.LostPercent
	}
	if result.FwdJitterMs == 0 && result.JitterMs > 0 {
		result.FwdJitterMs = result.JitterMs
	}

	// Merge reverse results.
	rev := mergeResults(revResults)
	result.ReverseSentBps = rev.SentBps
	result.ReverseReceivedBps = rev.ReceivedBps
	result.ReverseRetransmits = rev.Retransmits
	result.ReverseBytesSent = rev.BytesSent
	result.ReverseBytesReceived = rev.BytesReceived
	result.ReverseLostPackets = rev.LostPackets
	result.ReverseLostPercent = rev.LostPercent
	result.ReversePackets = rev.Packets
	result.ReverseJitterMs = rev.JitterMs
	result.ReverseIntervals = rev.Intervals

	return result, nil
}

// RunParallel runs N iperf3 instances on separate consecutive ports, each with
// -P 1, to work around the Windows/Cygwin EAGAIN bug with UDP parallel streams.
// Ports used: base, base+1, ..., base+N-1.
// onInterval is called with the aggregated interval as each time slot completes.
func (r *Runner) RunParallel(ctx context.Context, cfg IperfConfig, n int, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if n < 1 {
		n = 1
	}

	type testOutput struct {
		result *model.TestResult
		err    error
	}

	runners := make([]*Runner, 0, n)
	chs := make([]chan testOutput, n)

	perInstanceBW := splitBandwidth(cfg.Bandwidth, n)

	// Aggregator for live interval callbacks from N instances.
	agg := newIntervalAggregator(n, 0, onInterval)

	for i := 0; i < n; i++ {
		instCfg := cfg
		instCfg.Port = cfg.Port + i
		instCfg.Parallel = 1
		instCfg.Bandwidth = perInstanceBW
		instCfg.Bidir = false

		var runner *Runner
		if i == 0 {
			runner = r
		} else if r.debug {
			runner = NewDebugRunner()
		} else {
			runner = NewRunner()
		}
		runners = append(runners, runner)

		idx := i
		chs[i] = make(chan testOutput, 1)
		go func(rr *Runner, c IperfConfig, ch chan testOutput) {
			res, err := rr.RunWithIntervals(ctx, c, func(fwd, _ *model.IntervalResult) {
				agg.addForward(idx, fwd)
			})
			ch <- testOutput{res, err}
		}(runner, instCfg, chs[i])
	}

	// Store last runner for Stop() chaining.
	if n > 1 {
		r.mu.Lock()
		r.secondRunner = runners[len(runners)-1]
		r.mu.Unlock()
		defer func() {
			r.mu.Lock()
			r.secondRunner = nil
			r.mu.Unlock()
		}()
	}

	var results []*model.TestResult
	for i := 0; i < n; i++ {
		out := <-chs[i]
		if out.err != nil {
			return nil, fmt.Errorf("instance %d: %w", i, out.err)
		}
		results = append(results, out.result)
	}

	result := mergeResults(results)
	result.Parallel = cfg.Parallel
	return result, nil
}

// mergeResults aggregates multiple single-instance TestResults into one.
func mergeResults(results []*model.TestResult) *model.TestResult {
	if len(results) == 0 {
		return &model.TestResult{}
	}
	if len(results) == 1 {
		return results[0]
	}

	merged := *results[0] // copy first as base
	merged.Streams = nil
	merged.Intervals = nil

	// Zero out all summable fields before accumulating.
	merged.SentBps = 0
	merged.ReceivedBps = 0
	merged.Retransmits = 0
	merged.BytesSent = 0
	merged.BytesReceived = 0
	merged.LostPackets = 0
	merged.Packets = 0
	merged.JitterMs = 0
	merged.FwdReceivedBps = 0
	merged.FwdLostPackets = 0
	merged.FwdPackets = 0
	merged.FwdJitterMs = 0
	merged.ActualDuration = 0

	var jitterWeightSum float64
	var fwdJitterWeightSum float64

	for _, r := range results {
		merged.SentBps += r.SentBps
		merged.ReceivedBps += r.ReceivedBps
		merged.Retransmits += r.Retransmits
		merged.BytesSent += r.BytesSent
		merged.BytesReceived += r.BytesReceived
		merged.LostPackets += r.LostPackets
		merged.Packets += r.Packets
		merged.Streams = append(merged.Streams, r.Streams...)
		merged.Interrupted = merged.Interrupted || r.Interrupted

		// Server-measured forward metrics.
		merged.FwdReceivedBps += r.FwdReceivedBps
		merged.FwdLostPackets += r.FwdLostPackets
		merged.FwdPackets += r.FwdPackets

		// Weighted jitter: weight by packet count.
		if r.Packets > 0 {
			merged.JitterMs += r.JitterMs * float64(r.Packets)
			jitterWeightSum += float64(r.Packets)
		}
		if r.FwdPackets > 0 {
			merged.FwdJitterMs += r.FwdJitterMs * float64(r.FwdPackets)
			fwdJitterWeightSum += float64(r.FwdPackets)
		}

		// Keep the longest actual duration.
		if r.ActualDuration > merged.ActualDuration {
			merged.ActualDuration = r.ActualDuration
		}
	}

	// Weighted average jitter.
	if jitterWeightSum > 0 {
		merged.JitterMs = merged.JitterMs / jitterWeightSum
	}
	if fwdJitterWeightSum > 0 {
		merged.FwdJitterMs = merged.FwdJitterMs / fwdJitterWeightSum
	}

	// Recompute lost percent from totals.
	if merged.Packets > 0 {
		merged.LostPercent = float64(merged.LostPackets) / float64(merged.Packets) * 100
	}
	if merged.FwdPackets > 0 {
		merged.FwdLostPercent = float64(merged.FwdLostPackets) / float64(merged.FwdPackets) * 100
	}

	// Merge intervals by time index: sum bandwidths at matching indices.
	maxIntervals := 0
	for _, r := range results {
		if len(r.Intervals) > maxIntervals {
			maxIntervals = len(r.Intervals)
		}
	}
	if maxIntervals > 0 {
		merged.Intervals = make([]model.IntervalResult, maxIntervals)
		for idx := 0; idx < maxIntervals; idx++ {
			for _, r := range results {
				if idx < len(r.Intervals) {
					iv := r.Intervals[idx]
					m := &merged.Intervals[idx]
					if m.TimeEnd == 0 {
						m.TimeStart = iv.TimeStart
						m.TimeEnd = iv.TimeEnd
						m.Omitted = iv.Omitted
					}
					m.Bytes += iv.Bytes
					m.BandwidthBps += iv.BandwidthBps
					m.Retransmits += iv.Retransmits
					m.Packets += iv.Packets
					m.LostPackets += iv.LostPackets
				}
			}
			m := &merged.Intervals[idx]
			if m.Packets > 0 {
				m.LostPercent = float64(m.LostPackets) / float64(m.Packets) * 100
			}
		}
	}

	return &merged
}

// splitBandwidth divides a bandwidth string (e.g. "50M") evenly among n instances.
// Returns a new bandwidth string suitable for iperf3 -b flag.
// If bw is empty (unlimited), returns empty.
func splitBandwidth(bw string, n int) string {
	if bw == "" || n <= 1 {
		return bw
	}
	bits := parseBandwidthBits(bw)
	if bits == 0 {
		return bw
	}
	perInstance := int64(bits) / int64(n)
	if perInstance < 1 {
		perInstance = 1
	}
	return strconv.FormatInt(perInstance, 10)
}

// intervalAggregator collects intervals from N forward and M reverse parallel
// instances. When all instances for a given time slot have reported, it fires
// the aggregated onInterval callback.
type intervalAggregator struct {
	mu         sync.Mutex
	nFwd       int // expected forward instances
	nRev       int // expected reverse instances (0 = no reverse)
	fwdSlots   map[int][]model.IntervalResult // time-slot index → collected intervals
	revSlots   map[int][]model.IntervalResult
	fwdFired   map[int]bool // already-fired slots
	onInterval func(fwd, rev *model.IntervalResult)
}

func newIntervalAggregator(nFwd, nRev int, onInterval func(fwd, rev *model.IntervalResult)) *intervalAggregator {
	return &intervalAggregator{
		nFwd:       nFwd,
		nRev:       nRev,
		fwdSlots:   make(map[int][]model.IntervalResult),
		revSlots:   make(map[int][]model.IntervalResult),
		fwdFired:   make(map[int]bool),
		onInterval: onInterval,
	}
}

func (a *intervalAggregator) addForward(instanceIdx int, iv *model.IntervalResult) {
	if a.onInterval == nil || iv == nil || iv.Omitted {
		return
	}
	slot := int(iv.TimeStart + 0.5) // round to nearest second as slot key
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fwdSlots[slot] = append(a.fwdSlots[slot], *iv)
	a.tryFire(slot)
}

func (a *intervalAggregator) addReverse(instanceIdx int, iv *model.IntervalResult) {
	if a.onInterval == nil || iv == nil || iv.Omitted {
		return
	}
	slot := int(iv.TimeStart + 0.5)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revSlots[slot] = append(a.revSlots[slot], *iv)
	a.tryFire(slot)
}

// tryFire checks if a slot has all expected instances and fires the callback.
// Must be called with a.mu held.
func (a *intervalAggregator) tryFire(slot int) {
	if a.fwdFired[slot] {
		return
	}
	fwds := a.fwdSlots[slot]
	if len(fwds) < a.nFwd {
		return
	}
	// If we expect reverse instances, wait for them too.
	if a.nRev > 0 {
		revs := a.revSlots[slot]
		if len(revs) < a.nRev {
			return
		}
	}
	a.fwdFired[slot] = true

	// Aggregate forward intervals.
	fwd := a.sumIntervals(fwds)

	// Aggregate reverse intervals (may be nil).
	var rev *model.IntervalResult
	if a.nRev > 0 {
		revs := a.revSlots[slot]
		rev = a.sumIntervals(revs)
	}

	a.onInterval(fwd, rev)
}

func (a *intervalAggregator) sumIntervals(ivs []model.IntervalResult) *model.IntervalResult {
	if len(ivs) == 0 {
		return nil
	}
	sum := ivs[0]
	for _, iv := range ivs[1:] {
		sum.Bytes += iv.Bytes
		sum.BandwidthBps += iv.BandwidthBps
		sum.Retransmits += iv.Retransmits
		sum.Packets += iv.Packets
		sum.LostPackets += iv.LostPackets
	}
	if sum.Packets > 0 {
		sum.LostPercent = float64(sum.LostPackets) / float64(sum.Packets) * 100
	}
	return &sum
}
