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
	ServerAddr string
	Port       int
	Parallel   int
	Duration   int
	Interval   int
	Protocol   string
	BinaryPath string
	BlockSize   int
	MeasurePing bool
	Reverse    bool
	Bidir      bool
	Bandwidth  string
	Congestion string

	// Remote server (optional)
	SSHHost     string
	SSHUser     string
	SSHKeyPath  string
	SSHPassword string
	SSHPort     int
	StartServer bool
	StopServer  bool
	InstallIperf bool

	// Output
	OutputCSV string
	Verbose   bool
	Debug     bool
}

// LocalTestRunner runs a single iperf3 test locally and optionally saves results.
// It uses --json-stream mode for live interval reporting when iperf3 >= 3.17,
// falling back to -J mode otherwise.
func LocalTestRunner(cfg RunnerConfig) (*model.TestResult, error) {
	iperfCfg := iperf.IperfConfig{
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
		Congestion: cfg.Congestion,
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

	// Run iperf test
	result, iperfVersion, err := runIperfTest(runner, iperfCfg, cfg)

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
	// Always override with config values — parsed values may be empty on
	// partial runs (e.g. connection refused after start event).
	iperfCfg.ApplyToResult(result, "CLI")
	result.Congestion = cfg.Congestion
	result.IperfVersion = iperfVersion
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

func runIperfTest(runner *iperf.Runner, iperfCfg iperf.IperfConfig, cfg RunnerConfig) (*model.TestResult, string, error) {
	version, versionErr := iperf.CheckVersion(iperfCfg.BinaryPath)
	if versionErr != nil {
		fmt.Printf("Note: %v — falling back to standard JSON mode (no live intervals)\n", versionErr)
		result, err := runner.RunWithPipe(context.Background(), iperfCfg, func(line string) {
			if cfg.Verbose {
				fmt.Println(line)
			}
		})
		return result, version, err
	}

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
	result, err := runner.RunWithIntervals(context.Background(), iperfCfg, func(fwd, rev *model.IntervalResult) {
		ts := testStart.Add(time.Duration(fwd.TimeStart * float64(time.Second))).Format("15:04:05")
		if rev != nil {
			fmt.Println(ts + "  " + format.FormatBidirInterval(fwd, rev, isUDP))
		} else {
			fmt.Println(ts + "  " + format.FormatInterval(fwd, isUDP))
		}
	})
	return result, version, err
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

// RemoteServerRunner manages a remote iperf3 server via SSH.
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

// Install attempts to install iperf3 on the remote host.
func (r *RemoteServerRunner) Install() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	if r.cfg.Verbose {
		fmt.Println("Checking/installing iperf3 on remote host...")
	}

	if err := r.client.InstallIperf3(); err != nil {
		return fmt.Errorf("install iperf3: %w", err)
	}

	if r.cfg.Verbose {
		fmt.Println("iperf3 ready on remote host")
	}
	return nil
}

// Start starts the remote iperf3 server.
func (r *RemoteServerRunner) Start() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	port := r.cfg.Port
	if port == 0 {
		port = 5201
	}

	if r.cfg.Verbose {
		fmt.Printf("Starting remote iperf3 server on port %d...\n", port)
	}

	if err := r.mgr.StartServer(r.client, port); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	if r.cfg.Verbose {
		fmt.Println("Remote server started")
	}
	return nil
}

// Stop stops the remote iperf3 server.
func (r *RemoteServerRunner) Stop() error {
	if r.client == nil {
		return fmt.Errorf("not connected")
	}

	if r.cfg.Verbose {
		fmt.Println("Stopping remote iperf3 server...")
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

// PrintResult formats and prints a test result.
func PrintResult(result *model.TestResult) {
	fmt.Println()
	fmt.Println(format.FormatResult(result))
}
