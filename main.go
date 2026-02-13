package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/app"

	"iperf-tool/internal/cli"
	"iperf-tool/ui"
)

func main() {
	cfg, err := cli.ParseFlags()
	if err != nil {
		os.Exit(1)
	}

	// No flags provided or help requested = use GUI
	if cfg == nil {
		a := app.NewWithID("com.iperf-tool.gui")
		win := ui.BuildMainWindow(a)
		win.ShowAndRun()
		return
	}

	// CLI mode
	if err := runCLI(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(cfg *cli.RunnerConfig) error {
	// Handle remote server operations
	if cfg.SSHHost != "" {
		return runRemoteServer(cfg)
	}

	// Handle local test
	result, err := cli.LocalTestRunner(*cfg)
	if err != nil {
		return err
	}

	cli.PrintResult(result)
	return nil
}

func runRemoteServer(cfg *cli.RunnerConfig) error {
	runner := cli.NewRemoteServerRunner(*cfg)
	defer runner.Close()

	if err := runner.Connect(); err != nil {
		return err
	}

	// Install iperf3 if requested
	if cfg.InstallIperf {
		if err := runner.Install(); err != nil {
			return err
		}
	}

	// Start server if requested
	if cfg.StartServer {
		if err := runner.Start(); err != nil {
			return err
		}
	}

	// Stop server if requested
	if cfg.StopServer {
		if err := runner.Stop(); err != nil {
			return err
		}
	}

	// Run local test if server address provided
	if cfg.ServerAddr != "" {
		result, err := cli.LocalTestRunner(*cfg)
		if err != nil {
			return err
		}
		cli.PrintResult(result)
	}

	return nil
}
