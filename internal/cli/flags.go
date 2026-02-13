package cli

import (
	"flag"
	"fmt"
	"os"
)

// ParseFlags parses command-line arguments and returns a RunnerConfig.
// Returns nil config and prints help if no arguments or --help is provided.
func ParseFlags() (*RunnerConfig, error) {
	if len(os.Args) < 2 {
		return nil, nil // No args = use GUI
	}

	if os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		PrintUsage()
		return nil, nil
	}

	cfg := &RunnerConfig{
		Port:       5201,
		Parallel:   1,
		Duration:   10,
		Interval:   1,
		Protocol:   "tcp",
		BinaryPath: "iperf3",
		SSHPort:    22,
	}

	fs := flag.NewFlagSet("iperf-tool", flag.ContinueOnError)

	// Local test flags
	fs.StringVar(&cfg.ServerAddr, "c", "", "Server address (required for local test)")
	fs.StringVar(&cfg.ServerAddr, "connect", "", "Server address (required for local test)")
	fs.IntVar(&cfg.Port, "p", cfg.Port, "Server port")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "Server port")
	fs.IntVar(&cfg.Parallel, "P", cfg.Parallel, "Parallel streams")
	fs.IntVar(&cfg.Parallel, "parallel", cfg.Parallel, "Parallel streams")
	fs.IntVar(&cfg.Duration, "t", cfg.Duration, "Test duration in seconds")
	fs.IntVar(&cfg.Duration, "time", cfg.Duration, "Test duration in seconds")
	fs.IntVar(&cfg.Interval, "i", cfg.Interval, "Reporting interval in seconds")
	fs.IntVar(&cfg.Interval, "interval", cfg.Interval, "Reporting interval in seconds")
	fs.StringVar(&cfg.Protocol, "u", cfg.Protocol, "UDP mode (use 'udp', default 'tcp')")
	fs.StringVar(&cfg.BinaryPath, "binary", cfg.BinaryPath, "Path to iperf3 binary")

	// Remote server flags
	fs.StringVar(&cfg.SSHHost, "ssh", "", "SSH host for remote server")
	fs.StringVar(&cfg.SSHUser, "user", os.Getenv("USER"), "SSH username")
	fs.StringVar(&cfg.SSHKeyPath, "key", "", "SSH private key path")
	fs.StringVar(&cfg.SSHPassword, "password", "", "SSH password (insecure, use key instead)")
	fs.IntVar(&cfg.SSHPort, "ssh-port", 22, "SSH port")
	fs.BoolVar(&cfg.StartServer, "start-server", false, "Start remote iperf3 server")
	fs.BoolVar(&cfg.StopServer, "stop-server", false, "Stop remote iperf3 server")
	fs.BoolVar(&cfg.InstallIperf, "install", false, "Install iperf3 on remote host")

	// Output flags
	fs.StringVar(&cfg.OutputCSV, "o", "", "Output CSV file")
	fs.StringVar(&cfg.OutputCSV, "output", "", "Output CSV file")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose output")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// Normalize protocol
	if cfg.Protocol == "udp" || cfg.Protocol == "u" {
		cfg.Protocol = "udp"
	} else {
		cfg.Protocol = "tcp"
	}

	// Validate: must have either server address or SSH host
	if cfg.ServerAddr == "" && cfg.SSHHost == "" {
		fmt.Fprintf(os.Stderr, "Error: must provide -c <server> for local test or -ssh <host> for remote server\n\n")
		PrintUsage()
		return nil, fmt.Errorf("missing required flags")
	}

	return cfg, nil
}

// PrintUsage prints the help message.
func PrintUsage() {
	fmt.Fprintf(os.Stderr, `iperf3 Test Tool

Usage: iperf-tool [flags]
       iperf-tool help    (show this message)

LOCAL TEST MODE:
  -c, -connect <addr>      Server address to test (required for local test)
  -p, -port <num>          Server port (default: 5201)
  -P, -parallel <num>      Parallel streams (default: 1)
  -t, -time <sec>          Test duration in seconds (default: 10)
  -i, -interval <sec>      Reporting interval (default: 1)
  -u <udp|tcp>             Protocol mode (default: tcp)
  -binary <path>           Path to iperf3 binary (default: iperf3)

REMOTE SERVER MODE:
  -ssh <host>              SSH host to manage remote iperf3 server
  -user <name>             SSH username (default: $USER)
  -key <path>              SSH private key path
  -password <pwd>          SSH password (insecure, prefer -key)
  -ssh-port <num>          SSH port (default: 22)
  -install                 Install iperf3 on remote host
  -start-server            Start remote iperf3 server
  -stop-server             Stop remote iperf3 server

OUTPUT:
  -o, -output <file>       Save results to CSV file
  -v, -verbose             Verbose output

EXAMPLES:
  # Run local test to server
  iperf-tool -c 192.168.1.1 -t 30 -P 4 -o results.csv

  # Run test with verbose output
  iperf-tool -c server.example.com -t 60 -v

  # Test via UDP
  iperf-tool -c 10.0.0.1 -u udp -t 20

  # Install iperf3 on remote server and start it
  iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -install -start-server

  # Run local test against remote server
  iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -start-server -c remote.host -t 30

  # Stop remote server
  iperf-tool -ssh remote.host -user ubuntu -key ~/.ssh/id_rsa -stop-server

`)
}
