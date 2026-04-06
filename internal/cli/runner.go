package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"iperf-tool/internal/export"
	"iperf-tool/internal/format"
	"iperf-tool/internal/iperf"
	"iperf-tool/internal/model"
	"iperf-tool/internal/netutil"
	"iperf-tool/internal/ping"
	"iperf-tool/internal/ssh"
)

// RunnerConfig holds all CLI options for a test run.
type RunnerConfig struct {
	// Local test
	ServerAddr  string
	Port        int
	Parallel    int
	Duration    int
	Interval    int
	Protocol    string
	BinaryPath  string
	BlockSize   int
	MeasurePing bool
	Reverse     bool
	Bidir       bool
	Bandwidth   string
	IPv6        bool

	// Remote server (optional)
	SSHHost      string
	SSHUser      string
	SSHKeyPath   string
	SSHPassword  string
	SSHPort      int
	StartServer  bool
	StopServer   bool
	InstallIperf bool

	// Repeat
	Repeat      bool // loop until Ctrl-C or RepeatCount exhausted
	RepeatCount int  // 0 = infinite; N > 0 = run exactly N times

	// Output
	OutputCSV string
	Verbose   bool
	Debug     bool

	// SSH client — set after Connect(), used by Reverse/Bidir/Forward with SSH
	SSHClient iperf.SSHClient
	// IsWindows — set after Connect() if remote is Windows
	IsWindows bool
	// LocalAddr — local IP for reverse/bidir (remote client connects back here)
	LocalAddr string
}

// LocalTestRunner runs a single iperf2 test locally and optionally saves results.
func LocalTestRunner(cfg RunnerConfig) (*model.TestResult, error) {
	iperfCfg := iperf.Config{
		BinaryPath: cfg.BinaryPath,
		ServerAddr: cfg.ServerAddr,
		Port:       cfg.Port,
		Parallel:   cfg.Parallel,
		Duration:   cfg.Duration,
		Interval:   cfg.Interval,
		Protocol:   cfg.Protocol,
		BlockSize:  cfg.BlockSize,
		Reverse:    cfg.Reverse,
		Bidir:      cfg.Bidir,
		Bandwidth:  cfg.Bandwidth,
		IPv6:       cfg.IPv6,
		IsWindows:  cfg.IsWindows,
		LocalAddr:  cfg.LocalAddr,
		Enhanced:   true,
	}

	if err := iperfCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	var runner *iperf.Runner
	if cfg.Debug {
		runner = iperf.NewDebugRunner()
	} else {
		runner = iperf.NewRunner()
	}
	ctx := context.Background()

	dirLabel := ""
	if cfg.Reverse {
		dirLabel = ", reverse"
	} else if cfg.Bidir {
		dirLabel = ", bidirectional"
	}
	fmt.Printf("Starting test: %s:%d (%s, %d parallel, %ds duration%s)\n",
		cfg.ServerAddr, cfg.Port, strings.ToUpper(cfg.Protocol), cfg.Parallel, cfg.Duration, dirLabel)

	// Phase 1: baseline ping (before iperf)
	var baseline *ping.Result
	if cfg.MeasurePing {
		fmt.Println("Running baseline ping (4 packets)...")
		var err error
		baseline, err = ping.Run(ctx, cfg.ServerAddr, 4)
		if err != nil {
			fmt.Printf("Baseline ping failed: %v\n", err)
		} else {
			fmt.Printf("Baseline latency: min/avg/max = %.2f / %.2f / %.2f ms\n",
				baseline.MinMs, baseline.AvgMs, baseline.MaxMs)
		}
	}

	// Phase 2: start background ping (during iperf)
	var loadedCh chan *ping.Result
	var pingCancel context.CancelFunc
	if cfg.MeasurePing {
		var pingCtx context.Context
		pingCtx, pingCancel = context.WithCancel(ctx)
		loadedCh = make(chan *ping.Result, 1)
		go func() {
			loaded, err := ping.RunUntilCancel(pingCtx, cfg.ServerAddr)
			if err != nil {
				fmt.Printf("Under-load ping failed: %v\n", err)
				loadedCh <- nil
			} else {
				loadedCh <- loaded
			}
		}()
	}

	// Print interval header
	isUDP := strings.EqualFold(iperfCfg.Protocol, "udp")
	if iperfCfg.Bidir {
		header := "Time      " + format.FormatBidirIntervalHeader(isUDP)
		fmt.Println(header)
		fmt.Println(strings.Repeat("-", len(header)))
	} else {
		header := "Time      " + format.FormatIntervalHeader(isUDP)
		fmt.Println(header)
		fmt.Println(strings.Repeat("-", len(header)))
	}

	testStart := time.Now()

	emit := func(fwd, rev *model.IntervalResult) {
		if fwd == nil && rev == nil {
			return
		}
		ref := fwd
		if ref == nil {
			ref = rev
		}
		ts := testStart.Add(time.Duration(ref.TimeStart * float64(time.Second))).Format("15:04:05")
		if iperfCfg.Bidir {
			fmt.Println(ts + "  " + format.FormatBidirInterval(fwd, rev, isUDP))
		} else if fwd != nil {
			fmt.Println(ts + "  " + format.FormatInterval(fwd, isUDP))
		}
	}

	var onInterval func(fwd, rev *model.IntervalResult)
	var flushIntervals func()
	if iperfCfg.Bidir {
		onInterval, flushIntervals = iperf.PairBidirIntervals(emit)
	} else {
		onInterval = emit
	}

	// Run iperf test — dispatch based on direction
	var result *model.TestResult
	var err error

	version, _ := iperf.CheckVersion(iperfCfg.BinaryPath)

	if iperfCfg.Bidir {
		if cfg.SSHClient == nil {
			result, err = runner.RunBidirDualtest(ctx, iperfCfg, onInterval)
		} else {
			result, err = runner.RunBidir(ctx, iperfCfg, cfg.SSHClient, onInterval)
		}
	} else if iperfCfg.Reverse {
		result, err = runner.RunReverse(ctx, iperfCfg, cfg.SSHClient, onInterval)
	} else {
		result, err = runner.RunForward(ctx, iperfCfg, cfg.SSHClient, onInterval)
	}

	if flushIntervals != nil {
		flushIntervals()
	}

	// Stop background ping and collect result
	var pingBaseline, pingLoaded *model.PingResult
	if cfg.MeasurePing && pingCancel != nil {
		pingCancel()
		loaded := <-loadedCh
		pingBaseline = baseline.ToModel()
		pingLoaded = loaded.ToModel()
	}

	if err != nil {
		return nil, err
	}

	result.PingBaseline = pingBaseline
	result.PingLoaded = pingLoaded

	// Set config echo fields on the result.
	iperfCfg.ApplyToResult(result, "CLI")
	result.IperfVersion = version
	if h, herr := os.Hostname(); herr == nil {
		result.LocalHostname = h
	}
	result.LocalIP = netutil.OutboundIP()
	if cfg.SSHHost != "" {
		result.SSHRemoteHost = cfg.SSHHost
	}
	result.MeasurementID = export.NextMeasurementID(result.Timestamp)

	saveResults(result, cfg)
	return result, nil
}

func saveResults(result *model.TestResult, cfg RunnerConfig) {
	if cfg.OutputCSV == "" {
		return // opt-in: only save when -o is specified
	}

	base := strings.TrimSuffix(cfg.OutputCSV, ".csv")

	if err := export.EnsureDir(base + ".csv"); err != nil {
		fmt.Printf("Cannot create output directory: %v\n", err)
		return
	}

	date := result.Timestamp
	logPath := export.BuildLogPath(base, "_log", ".csv")
	csvPath := export.BuildPath(base, "", ".csv", date)
	txtPath := export.BuildPath(base, "", ".txt", date)

	if err := export.WriteCSV(logPath, []model.TestResult{*result}); err != nil {
		fmt.Printf("Save CSV error: %v\n", err)
		return
	}
	if err := export.WriteTXT(txtPath, []model.TestResult{*result}); err != nil {
		fmt.Printf("Save TXT error: %v\n", err)
	}
	if len(result.Intervals) > 0 {
		if err := export.WriteIntervalLog(csvPath, result); err != nil {
			fmt.Printf("Save interval log error: %v\n", err)
		}
	}
	fmt.Printf("Results saved: %s, %s\n", logPath, txtPath)
}

// RemoteServerRunner manages a remote iperf2 server via SSH.
type RemoteServerRunner struct {
	cfg    RunnerConfig
	client *ssh.Client
	mgr    *ssh.ServerManager
}

// NewRemoteServerRunner creates a new remote server manager.
func NewRemoteServerRunner(cfg RunnerConfig) *RemoteServerRunner {
	return &RemoteServerRunner{
		cfg: cfg,
		mgr: ssh.NewServerManager(),
	}
}

// Connect establishes SSH connection to the remote host.
func (r *RemoteServerRunner) Connect() error {
	sshCfg := ssh.ConnectConfig{
		Host:     r.cfg.SSHHost,
		Port:     r.cfg.SSHPort,
		User:     r.cfg.SSHUser,
		KeyPath:  r.cfg.SSHKeyPath,
		Password: r.cfg.SSHPassword,
	}

	client, err := ssh.Connect(sshCfg)
	if err != nil {
		return fmt.Errorf("SSH connect: %w", err)
	}

	r.client = client
	if r.cfg.Verbose {
		fmt.Printf("Connected to %s@%s\n", r.cfg.SSHUser, r.cfg.SSHHost)
	}

	return nil
}

// Close disconnects from the remote host.
func (r *RemoteServerRunner) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Install attempts to install iperf2 on the remote host.
func (r *RemoteServerRunner) Install() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	if r.cfg.Verbose {
		fmt.Println("Checking/installing iperf2 on remote host...")
	}

	if err := r.client.InstallIperf(); err != nil {
		return fmt.Errorf("install iperf2: %w", err)
	}

	if r.cfg.Verbose {
		fmt.Println("iperf2 ready on remote host")
	}
	return nil
}

// Start starts the remote iperf2 server.
func (r *RemoteServerRunner) Start() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	port := r.cfg.Port
	if port == 0 {
		port = 5201
	}

	if r.cfg.Verbose {
		fmt.Printf("Starting remote iperf2 servers on ports %d, %d...\n", port, port+1)
	}

	if err := r.mgr.StartServer(r.client, port); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	if r.cfg.Verbose {
		fmt.Printf("Remote servers started on ports %d, %d\n", port, port+1)
	}
	return nil
}

// Restart kills any existing iperf2 processes and starts a fresh server.
// numInstances controls how many server instances to start (0 = default of 2).
func (r *RemoteServerRunner) Restart(numInstances int) error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}
	port := r.cfg.Port
	if port == 0 {
		port = 5201
	}
	return r.mgr.RestartServer(r.client, port, numInstances)
}

// Stop stops the remote iperf2 server.
func (r *RemoteServerRunner) Stop() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	if r.cfg.Verbose {
		fmt.Println("Stopping remote iperf2 server...")
	}

	if err := r.mgr.StopServer(r.client); err != nil {
		return fmt.Errorf("stop server: %w", err)
	}

	if r.cfg.Verbose {
		fmt.Println("Remote server stopped")
	}
	return nil
}

// CheckStatus checks if the remote server is running.
func (r *RemoteServerRunner) CheckStatus() (bool, error) {
	if r.client == nil {
		return false, fmt.Errorf("not connected")
	}
	return r.mgr.CheckStatus(r.client)
}

// Client returns the underlying SSH client for use with the iperf2 runner.
func (r *RemoteServerRunner) Client() *ssh.Client {
	return r.client
}

// IsWindows returns true if the remote host is running Windows.
func (r *RemoteServerRunner) IsWindows() bool {
	if r.client == nil {
		return false
	}
	os, err := r.client.DetectOS()
	return err == nil && os == ssh.OSWindows
}

// PrintResult formats and prints a test result.
func PrintResult(result *model.TestResult) {
	fmt.Println()
	fmt.Println(format.FormatResult(result))
}
