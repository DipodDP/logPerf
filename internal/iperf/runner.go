package iperf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"iperf-tool/internal/model"
)

const DebugLogPath = "/tmp/iperf-debug.log"

// SSHClient is the interface for executing commands on a remote host.
// Satisfied by *ssh.Client from internal/ssh/client.go.
type SSHClient interface {
	RunCommand(cmd string) (string, error)
}

// Runner executes iperf2 commands.
type Runner struct {
	mu          sync.Mutex
	localSrvCmd *exec.Cmd
	fwdCmd      *exec.Cmd
	stopped     bool
	debug       bool
}

// NewRunner creates a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// NewDebugRunner creates a Runner that logs raw output to DebugLogPath.
func NewDebugRunner() *Runner {
	return &Runner{debug: true}
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

// Stop sends SIGTERM to running local processes.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.fwdCmd != nil && r.fwdCmd.Process != nil {
		r.fwdCmd.Process.Signal(syscall.SIGTERM)
	}
	if r.localSrvCmd != nil && r.localSrvCmd.Process != nil {
		r.localSrvCmd.Process.Signal(syscall.SIGTERM)
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

	// Get server-side data
	var serverResult *model.TestResult
	if sshCli != nil && cfg.SSHFallback {
		serverOutput, readErr := r.readRemoteServerOutput(cfg, sshCli)
		if readErr == nil && strings.TrimSpace(serverOutput) != "" {
			serverResult, _ = ParseOutput(serverOutput, true)
		}
	} else {
		// Validate Server Report from client output
		status := ValidateServerReport(clientOutput)
		if status == ServerReportValid {
			// Parse server data embedded in client output
			serverResult, _ = parseServerReportFromClient(clientOutput)
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
		return nil, fmt.Errorf("SSH client required for reverse tests")
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
		localSrvCmd.Process.Signal(syscall.SIGTERM)
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
		return nil, fmt.Errorf("SSH client required for bidirectional tests")
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
		localSrvCmd.Process.Signal(syscall.SIGTERM)
	}
	localSrvCmd.Wait()
	r.mu.Lock()
	r.localSrvCmd = nil
	r.mu.Unlock()

	localSrvOutput := localSrvBuf.String()

	// Get remote server data
	var remoteSrvOutput string
	if cfg.SSHFallback {
		sshCli.RunCommand(cfg.remoteServerKillCmd()) // graceful kill
		time.Sleep(time.Duration(cfg.KillWaitMs) * time.Millisecond)
		remoteSrvOutput, _ = sshCli.RunCommand(cfg.remoteServerReadCmd())
	} else {
		sshCli.RunCommand(cfg.remoteServerKillCmd())
	}

	// Parse all outputs
	var fwdClientResult, fwdServerResult, revClientResult, revServerResult *model.TestResult

	if fwdOut.err == nil {
		fwdClientResult, _ = ParseOutput(fwdOut.output, false)
	}
	if fwdClientResult == nil {
		fwdClientResult = &model.TestResult{Timestamp: time.Now()}
	}

	if cfg.SSHFallback && strings.TrimSpace(remoteSrvOutput) != "" {
		fwdServerResult, _ = ParseOutput(remoteSrvOutput, true)
	} else if fwdOut.err == nil {
		// Try to get server data from client's Server Report
		status := ValidateServerReport(fwdOut.output)
		if status == ServerReportValid {
			fwdServerResult, _ = parseServerReportFromClient(fwdOut.output)
		}
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

// killRemoteServer sends a graceful kill to the remote iperf server.
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
