package cli

import (
	"context"
	"fmt"

	"iperf-tool/internal/export"
	"iperf-tool/internal/iperf"
	"iperf-tool/internal/model"
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
}

// LocalTestRunner runs a single iperf3 test locally and optionally saves results.
func LocalTestRunner(cfg RunnerConfig) (*model.TestResult, error) {
	iperfCfg := iperf.IperfConfig{
		BinaryPath: cfg.BinaryPath,
		ServerAddr: cfg.ServerAddr,
		Port:       cfg.Port,
		Parallel:   cfg.Parallel,
		Duration:   cfg.Duration,
		Interval:   cfg.Interval,
		Protocol:   cfg.Protocol,
	}

	if err := iperfCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	runner := iperf.NewRunner()

	if cfg.Verbose {
		fmt.Printf("Starting test: %s:%d (TCP, %d parallel, %ds duration)\n",
			cfg.ServerAddr, cfg.Port, cfg.Parallel, cfg.Duration)
	}

	result, err := runner.RunWithPipe(context.Background(), iperfCfg, func(line string) {
		if cfg.Verbose {
			fmt.Println(line)
		}
	})

	if err != nil {
		return nil, err
	}

	if cfg.OutputCSV != "" {
		if err := export.WriteCSV(cfg.OutputCSV, []model.TestResult{*result}); err != nil {
			return result, fmt.Errorf("save CSV: %w", err)
		}
		if cfg.Verbose {
			fmt.Printf("Results saved to: %s\n", cfg.OutputCSV)
		}
	}

	return result, nil
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
	fmt.Println("\n=== Test Results ===")
	fmt.Printf("Timestamp:       %s\n", result.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("Server:          %s:%d\n", result.ServerAddr, result.Port)
	fmt.Printf("Protocol:        %s\n", result.Protocol)
	fmt.Printf("Parallel:        %d streams\n", result.Parallel)
	fmt.Printf("Duration:        %d seconds\n", result.Duration)
	fmt.Printf("Sent:            %.2f Mbps\n", result.SentMbps())
	fmt.Printf("Received:        %.2f Mbps\n", result.ReceivedMbps())
	fmt.Printf("Retransmits:     %d\n", result.Retransmits)
	if result.Error != "" {
		fmt.Printf("Error:           %s\n", result.Error)
	}
	fmt.Println("====================")
}
