package iperf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"iperf-tool/internal/model"
)

var DebugLogPath = filepath.Join(os.TempDir(), "iperf-debug.log")

// SSHClient is the interface for executing commands on a remote host.
// Satisfied by *ssh.Client from internal/ssh/client.go.
type SSHClient interface {
	RunCommand(cmd string) (string, error)
}

// LocalAddrProvider is an optional interface that SSHClient implementations
// may satisfy to expose the local IP address of their connection.
type LocalAddrProvider interface {
	LocalAddr() string
}


// Runner executes iperf2 commands.
type Runner struct {
	mu          sync.Mutex
	localSrvCmd *exec.Cmd
	fwdCmd      *exec.Cmd
	stopped     bool
	debug       bool
	onStatus    StatusCallback // optional callback for status/log messages
}

// StatusCallback is a function that receives status/log messages from the runner.
// Used to route probe and progress messages to the GUI output view.
type StatusCallback func(msg string)

// NewRunner creates a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// defaultRemoteOutputFile returns a sensible default path for the remote server
// output file based on the remote OS.
func defaultRemoteOutputFile(isWindows bool) string {
	if isWindows {
		return `C:\iperf2_server_output.txt`
	}
	return "/tmp/iperf2_server_output.txt"
}

// logStatus sends a status message to the onInterval callback as a nil-interval
// signal, or falls back to stderr if no callback is available.
func (r *Runner) logStatus(onInterval func(fwd, rev *model.IntervalResult), format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	// Status messages are printed to stderr as a fallback (CLI mode)
	fmt.Fprintln(os.Stderr, msg)
	// The GUI hooks into onInterval; we use the StatusCallback on the runner if set.
	if r.onStatus != nil {
		r.onStatus(msg)
	}
}

// NewDebugRunner creates a Runner that logs raw output to DebugLogPath.
func NewDebugRunner() *Runner {
	return &Runner{debug: true}
}

// SetStatusCallback sets a callback for status/log messages (probe results,
// progress updates). This routes messages to the GUI output view instead of
// printing to stdout/stderr.
func (r *Runner) SetStatusCallback(cb StatusCallback) {
	r.onStatus = cb
}

// nopWriteCloser wraps io.Discard as an io.WriteCloser.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// debugWriter opens or creates DebugLogPath in append mode and writes a
// timestamped header line. Caller must close the returned WriteCloser.
func (r *Runner) debugWriter(label string, args []string) (io.WriteCloser, func(string, ...any)) {
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
	fmt.Fprintf(f, "\n=== %s %s ===\nargs: %v\n", time.Now().Format("2006-01-02 15:04:05"), label, args)
	logf := func(format string, a ...any) { fmt.Fprintf(f, format, a...) }
	return f, logf
}

// Stop terminates running local processes.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.fwdCmd != nil && r.fwdCmd.Process != nil {
		stopProcess(r.fwdCmd.Process)
	}
	if r.localSrvCmd != nil && r.localSrvCmd.Process != nil {
		stopProcess(r.localSrvCmd.Process)
	}
}

var versionRegex = regexp.MustCompile(`iperf\s+version\s+([\d.]+)`)

// CheckVersion runs iperf --version and returns the version string.
func CheckVersion(binaryPath string) (string, error) {
	out, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run iperf --version: %w", err)
	}
	matches := versionRegex.FindSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not parse iperf version from: %s", strings.TrimSpace(string(out)))
	}
	return string(matches[1]), nil
}

// RunForward runs a forward-only test (local client → remote server).
// sshCli may be nil for local-only tests (server must already be running).
func (r *Runner) RunForward(ctx context.Context, cfg Config, sshCli SSHClient, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// For UDP forward tests with SSH, probe reachability to decide direct vs fallback.
	if sshCli != nil && cfg.Protocol == "udp" && !cfg.SkipProbe {
		localAddr := cfg.LocalAddr
		if localAddr == "" {
			if lap, ok := sshCli.(LocalAddrProvider); ok {
				localAddr = lap.LocalAddr()
			}
		}
		if localAddr != "" {
			isOpen, err := ProbeUDPReachability(ctx, sshCli, localAddr, cfg.ProbeTimeout, cfg.IsWindows, cfg.IPv6)
			if err != nil {
				r.logStatus(onInterval, "iperf probe warning: %v — using SSH fallback", err)
				cfg.SSHFallback = true
			} else if isOpen {
				r.logStatus(onInterval, "UDP reachability probe: open — using direct mode")
				cfg.SSHFallback = false
			} else {
				r.logStatus(onInterval, "UDP reachability probe: blocked — using SSH fallback")
				cfg.SSHFallback = true
			}
		} else {
			cfg.SSHFallback = true
		}
	} else if sshCli != nil && cfg.Protocol == "udp" && cfg.SkipProbe {
		// Probe skipped: honour whatever SSHFallback the caller set.
	}
	if sshCli != nil && cfg.Protocol == "udp" && cfg.RemoteOutputFile == "" {
		cfg.RemoteOutputFile = defaultRemoteOutputFile(cfg.IsWindows)
	}

	// Start remote server if SSH is available
	if sshCli != nil {
		if err := r.startRemoteServer(cfg, sshCli); err != nil {
			return nil, fmt.Errorf("start remote server: %w", err)
		}
		defer r.killRemoteServer(cfg, sshCli)
	}

	// Run local client
	clientArgs := cfg.fwdClientArgs()
	clientOutput, err := r.runLocalClient(ctx, cfg.BinaryPath, clientArgs, onInterval)
	if err != nil {
		return nil, fmt.Errorf("forward client: %w", err)
	}

	// Parse client output
	clientResult, err := ParseOutput(clientOutput, false)
	if err != nil {
		return nil, fmt.Errorf("parse client output: %w", err)
	}

	// Get server-side data: try Server Report first, fall back to SSH file read
	var serverResult *model.TestResult
	if sshCli != nil && cfg.SSHFallback {
		serverOutput, readErr := r.readRemoteServerOutput(cfg, sshCli)
		if readErr == nil && strings.TrimSpace(serverOutput) != "" {
			serverResult, _ = ParseOutput(serverOutput, true)
		}
	} else {
		status := ValidateServerReport(clientOutput)
		if status == ServerReportValid {
			serverResult, _ = parseServerReportFromClient(clientOutput)
		} else if status == ServerReportFabricated {
			clientResult.FabricatedServerReport = true
		}
		// Server Report unavailable — fall back to SSH file read
		if serverResult == nil && sshCli != nil && cfg.RemoteOutputFile != "" {
			serverOutput, readErr := r.readRemoteServerOutput(cfg, sshCli)
			if readErr == nil && strings.TrimSpace(serverOutput) != "" {
				serverResult, _ = ParseOutput(serverOutput, true)
				clientResult.FabricatedServerReport = false
			}
		}
	}

	if serverResult != nil {
		return MergeUnidirResults(clientResult, serverResult), nil
	}
	return clientResult, nil
}

// RunReverse runs a reverse-only test (remote client → local server).
// sshCli is required for reverse tests.
func (r *Runner) RunReverse(ctx context.Context, cfg Config, sshCli SSHClient, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if sshCli == nil {
		return nil, fmt.Errorf("SSH connection required for reverse tests (iperf2 has no built-in -R flag; use --ssh to control the remote client)")
	}

	if cfg.Protocol == "udp" && !cfg.SkipProbe {
		localAddr := cfg.LocalAddr
		if localAddr == "" {
			if lap, ok := sshCli.(LocalAddrProvider); ok {
				localAddr = lap.LocalAddr()
			}
		}
		if localAddr != "" {
			isOpen, err := ProbeUDPReachability(ctx, sshCli, localAddr, cfg.ProbeTimeout, cfg.IsWindows, cfg.IPv6)
			if err != nil {
				r.logStatus(onInterval, "iperf probe warning: %v", err)
			} else if !isOpen {
				r.logStatus(onInterval, "UDP reachability probe: blocked (reverse test will fail to receive data)")
			}
		}
	}

	// Kill any leftover remote iperf
	sshCli.RunCommand(cfg.remoteServerKillCmd())
	time.Sleep(300 * time.Millisecond)

	// Start local server for reverse direction (port offset = Parallel)
	localSrvArgs := cfg.revServerArgs()
	var localSrvBuf bytes.Buffer
	localSrvCmd := exec.CommandContext(ctx, cfg.BinaryPath, localSrvArgs...)
	localSrvCmd.Stdout = &localSrvBuf
	localSrvCmd.Stderr = &localSrvBuf

	if err := localSrvCmd.Start(); err != nil {
		return nil, fmt.Errorf("start local server: %w", err)
	}
	r.mu.Lock()
	r.localSrvCmd = localSrvCmd
	r.mu.Unlock()
	time.Sleep(400 * time.Millisecond)

	// Start remote client via SSH
	revCmd := cfg.revClientCmd()
	revOutput, revErr := sshCli.RunCommand(revCmd)

	// Stop local server
	if localSrvCmd.Process != nil {
		stopProcess(localSrvCmd.Process)
	}
	localSrvCmd.Wait()
	r.mu.Lock()
	r.localSrvCmd = nil
	r.mu.Unlock()

	localSrvOutput := localSrvBuf.String()

	// Parse local server output (authoritative — this is the receiving side)
	localSrvResult, err := ParseOutput(localSrvOutput, true)
	if err != nil {
		return nil, fmt.Errorf("parse local server output: %w", err)
	}

	// Parse remote client output (send-side stats)
	var revClientResult *model.TestResult
	if revErr == nil && strings.TrimSpace(revOutput) != "" {
		revClientResult, _ = ParseOutput(revOutput, false)
	}

	// Merge: client sends, server receives
	if revClientResult != nil {
		result := MergeUnidirResults(revClientResult, localSrvResult)
		result.Direction = "Reverse"
		// Fire interval callback with server data
		if onInterval != nil {
			for i := range localSrvResult.Intervals {
				onInterval(&localSrvResult.Intervals[i], nil)
			}
		}
		return result, nil
	}

	localSrvResult.Direction = "Reverse"
	if onInterval != nil {
		for i := range localSrvResult.Intervals {
			onInterval(&localSrvResult.Intervals[i], nil)
		}
	}
	return localSrvResult, nil
}

// RunBidir runs a bidirectional test — both directions simultaneously.
// sshCli is required for bidirectional tests.
func (r *Runner) RunBidir(ctx context.Context, cfg Config, sshCli SSHClient, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if sshCli == nil {
		return nil, fmt.Errorf("SSH connection required for SSH-controlled bidirectional tests (use RunBidirDualtest for no-SSH dualtest mode)")
	}

	// For UDP bidir tests, run a pre-flight UDP reachability probe to decide
	// whether to use direct mode (Server Report from client stdout) or SSH
	// file fallback. ValidateServerReport acts as a secondary safety net on
	// the direct path in case the probe result was stale.
	if cfg.Protocol == "udp" && !cfg.SkipProbe {
		localAddr := cfg.LocalAddr
		if localAddr == "" {
			if lap, ok := sshCli.(LocalAddrProvider); ok {
				localAddr = lap.LocalAddr()
			}
		}
		if localAddr != "" {
			isOpen, err := ProbeUDPReachability(ctx, sshCli, localAddr, cfg.ProbeTimeout, cfg.IsWindows, cfg.IPv6)
			if err != nil {
				r.logStatus(onInterval, "iperf probe warning: %v — using SSH fallback", err)
				cfg.SSHFallback = true
			} else if isOpen {
				r.logStatus(onInterval, "UDP reachability probe: open — using direct mode")
				cfg.SSHFallback = false
			} else {
				r.logStatus(onInterval, "UDP reachability probe: blocked — using SSH fallback")
				cfg.SSHFallback = true
			}
		} else {
			// No local addr available — default to SSH fallback to be safe
			cfg.SSHFallback = true
		}
	} else if cfg.Protocol == "udp" && cfg.SkipProbe {
		// Probe skipped: honour whatever SSHFallback the caller set (default false).
	}
	if cfg.Protocol == "udp" && cfg.RemoteOutputFile == "" {
		cfg.RemoteOutputFile = defaultRemoteOutputFile(cfg.IsWindows)
	}

	// Kill any leftover remote iperf
	sshCli.RunCommand(cfg.remoteServerKillCmd())
	time.Sleep(300 * time.Millisecond)

	// Start remote server (forward direction receives)
	if _, err := sshCli.RunCommand(cfg.remoteServerStartCmd()); err != nil {
		return nil, fmt.Errorf("start remote server: %w", err)
	}
	time.Sleep(600 * time.Millisecond)

	// Start local server (reverse direction receives)
	localSrvArgs := cfg.revServerArgs()
	var localSrvBuf bytes.Buffer
	localSrvCmd := exec.CommandContext(ctx, cfg.BinaryPath, localSrvArgs...)
	localSrvCmd.Stdout = &localSrvBuf
	localSrvCmd.Stderr = &localSrvBuf

	if err := localSrvCmd.Start(); err != nil {
		r.killRemoteServer(cfg, sshCli)
		return nil, fmt.Errorf("start local server: %w", err)
	}
	r.mu.Lock()
	r.localSrvCmd = localSrvCmd
	r.mu.Unlock()
	time.Sleep(400 * time.Millisecond)

	// Run both clients concurrently
	type cmdResult struct {
		output string
		err    error
	}
	fwdCh := make(chan cmdResult, 1)
	revCh := make(chan cmdResult, 1)

	// Forward: local client → remote server
	go func() {
		clientArgs := cfg.fwdClientArgs()
		out, err := r.runLocalClient(ctx, cfg.BinaryPath, clientArgs, nil)
		fwdCh <- cmdResult{out, err}
	}()

	// Reverse: remote client → local server
	go func() {
		out, err := sshCli.RunCommand(cfg.revClientCmd())
		revCh <- cmdResult{out, err}
	}()

	fwdOut := <-fwdCh
	revOut := <-revCh

	// Stop local server
	if localSrvCmd.Process != nil {
		stopProcess(localSrvCmd.Process)
	}
	localSrvCmd.Wait()
	r.mu.Lock()
	r.localSrvCmd = nil
	r.mu.Unlock()

	localSrvOutput := localSrvBuf.String()

	// Kill remote server and read output file (always available for UDP bidir).
	sshCli.RunCommand(cfg.remoteServerKillCmd())
	time.Sleep(time.Duration(cfg.KillWaitMs) * time.Millisecond)
	var remoteSrvOutput string
	if cfg.RemoteOutputFile != "" {
		remoteSrvOutput, _ = sshCli.RunCommand(cfg.remoteServerReadCmd())
	}

	// Parse all outputs
	var fwdClientResult, fwdServerResult, revClientResult, revServerResult *model.TestResult

	if fwdOut.err == nil {
		fwdClientResult, _ = ParseOutput(fwdOut.output, false)
	}
	if fwdClientResult == nil {
		fwdClientResult = &model.TestResult{Timestamp: time.Now()}
	}

	if fwdOut.err == nil && !cfg.SSHFallback {
		// Try direct mode first: get server data from client's Server Report
		status := ValidateServerReport(fwdOut.output)
		if status == ServerReportValid {
			fwdServerResult, _ = parseServerReportFromClient(fwdOut.output)
		}
	}
	// Fall back to SSH file read if direct mode failed or SSHFallback was set
	if fwdServerResult == nil && strings.TrimSpace(remoteSrvOutput) != "" {
		fwdServerResult, _ = ParseOutput(remoteSrvOutput, true)
	}

	if revOut.err == nil && strings.TrimSpace(revOut.output) != "" {
		revClientResult, _ = ParseOutput(revOut.output, false)
	}

	if strings.TrimSpace(localSrvOutput) != "" {
		revServerResult, _ = ParseOutput(localSrvOutput, true)
	}

	// Merge all results
	merged := MergeBidirResults(fwdClientResult, fwdServerResult, revClientResult, revServerResult)

	// Fire onInterval callbacks (post-test replay)
	if onInterval != nil {
		fwdIvs := merged.Intervals
		revIvs := merged.ReverseIntervals
		maxLen := len(fwdIvs)
		if len(revIvs) > maxLen {
			maxLen = len(revIvs)
		}
		for i := 0; i < maxLen; i++ {
			var fwd, rev *model.IntervalResult
			if i < len(fwdIvs) {
				fwd = &fwdIvs[i]
			}
			if i < len(revIvs) {
				rev = &revIvs[i]
			}
			onInterval(fwd, rev)
		}
	}

	return merged, nil
}

// RunBidirDualtest runs a bidirectional test using iperf2's native -d (dualtest)
// flag. No SSH connection is needed — the remote server connects back to the
// local client. Both directions run simultaneously in a single process.
func (r *Runner) RunBidirDualtest(ctx context.Context, cfg Config, onInterval func(fwd, rev *model.IntervalResult)) (*model.TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	r.logStatus(onInterval, "Starting iperf2 bidirectional dualtest (-d) — server must be able to reach this client (no NAT)")

	clientArgs := cfg.dualtestClientArgs()
	clientOutput, err := r.runLocalClient(ctx, cfg.BinaryPath, clientArgs, nil)
	if err != nil {
		// Check for common NAT-blocked error patterns
		if strings.Contains(err.Error(), "connect failed") ||
			strings.Contains(err.Error(), "unable to connect") ||
			strings.Contains(err.Error(), "Connection refused") {
			return nil, fmt.Errorf("iperf2 dualtest failed — server could not connect back.\n"+
				"This usually means NAT is blocking inbound connections.\n"+
				"Connect via SSH to use the SSH-controlled bidirectional mode instead.\n"+
				"Original error: %w", err)
		}
		return nil, fmt.Errorf("dualtest client: %w", err)
	}

	// Check for reverse connect failure (iperf2 exits 0 but prints error)
	if strings.Contains(clientOutput, "expected reverse connect did not occur") {
		return nil, fmt.Errorf("iperf2 dualtest failed — server could not connect back.\n" +
			"This usually means NAT is blocking inbound connections.\n" +
			"Connect via SSH to use the SSH-controlled bidirectional mode instead.")
	}

	// Parse dualtest output — iperf2 -d interleaves forward and reverse streams
	result, err := ParseDualtestOutput(clientOutput)
	if err != nil {
		return nil, fmt.Errorf("parse dualtest output: %w", err)
	}

	// Fire onInterval callbacks (post-test replay)
	if onInterval != nil {
		fwdIvs := result.Intervals
		revIvs := result.ReverseIntervals
		maxLen := len(fwdIvs)
		if len(revIvs) > maxLen {
			maxLen = len(revIvs)
		}
		for i := 0; i < maxLen; i++ {
			var fwd, rev *model.IntervalResult
			if i < len(fwdIvs) {
				fwd = &fwdIvs[i]
			}
			if i < len(revIvs) {
				rev = &revIvs[i]
			}
			onInterval(fwd, rev)
		}
	}

	return result, nil
}

// startRemoteServer kills leftover iperf and starts the remote server.
func (r *Runner) startRemoteServer(cfg Config, sshCli SSHClient) error {
	// Kill any leftover
	sshCli.RunCommand(cfg.remoteServerKillCmd())
	time.Sleep(300 * time.Millisecond)

	// Start server
	if _, err := sshCli.RunCommand(cfg.remoteServerStartCmd()); err != nil {
		return err
	}
	time.Sleep(600 * time.Millisecond)
	return nil
}

// killRemoteServer sends a kill to the remote iperf server.
func (r *Runner) killRemoteServer(cfg Config, sshCli SSHClient) {
	sshCli.RunCommand(cfg.remoteServerKillCmd())
}

// readRemoteServerOutput reads the server output file via SSH.
func (r *Runner) readRemoteServerOutput(cfg Config, sshCli SSHClient) (string, error) {
	sshCli.RunCommand(cfg.remoteServerKillCmd()) // graceful kill first
	time.Sleep(time.Duration(cfg.KillWaitMs) * time.Millisecond)
	return sshCli.RunCommand(cfg.remoteServerReadCmd())
}

// runLocalClient executes a local iperf client command, piping output line by line.
// Returns the full output text.
func (r *Runner) runLocalClient(ctx context.Context, binaryPath string, args []string, onInterval func(fwd, rev *model.IntervalResult)) (string, error) {
	dbg, logf := r.debugWriter("client", args)
	defer dbg.Close()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	r.mu.Lock()
	r.fwdCmd = cmd
	r.stopped = false
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.fwdCmd = nil
		r.mu.Unlock()
	}()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start iperf: %w", err)
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")
		logf("%s\n", line)

		if onInterval != nil {
			if iv, err := ParseIntervalLine(line); err == nil && iv != nil {
				onInterval(iv, nil)
			}
		}
	}

	waitErr := cmd.Wait()

	r.mu.Lock()
	userStopped := r.stopped
	r.mu.Unlock()

	if waitErr != nil && !userStopped && buf.Len() == 0 {
		return "", fmt.Errorf("iperf failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}

	// Append stderr to output so callers can see ERROR/WARNING lines
	if stderrStr := stderr.String(); strings.TrimSpace(stderrStr) != "" {
		buf.WriteString(stderrStr)
	}

	return buf.String(), nil
}

// parseServerReportFromClient extracts the server report data
// from client output (the lines after "Server Report:").
func parseServerReportFromClient(clientOutput string) (*model.TestResult, error) {
	lines := strings.Split(clientOutput, "\n")
	inReport := false
	var reportLines []string

	for _, line := range lines {
		if reServerReport.MatchString(line) {
			inReport = true
			continue
		}
		if inReport {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Server report lines look like server interval lines
			reportLines = append(reportLines, line)
		}
	}

	if len(reportLines) == 0 {
		return nil, fmt.Errorf("no server report data found")
	}

	return ParseOutput(strings.Join(reportLines, "\n"), true)
}
