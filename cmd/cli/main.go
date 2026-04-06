package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"

	"iperf-tool/internal/cli"
	"iperf-tool/internal/iperf"
)

func main() {
	cfg, err := cli.ParseFlags()
	if err != nil {
		os.Exit(1)
	}
	if cfg == nil {
		fmt.Println("GUI not available in this build. Please provide CLI flags.")
		os.Exit(1)
	}
	if err := runCLI(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(cfg *cli.RunnerConfig) error {
	if cfg.SSHHost != "" {
		return runRemoteServer(cfg)
	}
	if cfg.Repeat {
		return runCLIRepeat(cfg)
	}
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

	if cfg.InstallIperf {
		if err := runner.Install(); err != nil {
			return err
		}
	}

	if cfg.StartServer {
		if err := runner.Start(); err != nil {
			return err
		}
	}

	if cfg.StopServer {
		if err := runner.Stop(); err != nil {
			return err
		}
	}

	if cfg.ServerAddr != "" {
		cfg.SSHClient = runner.Client()
		cfg.IsWindows = runner.IsWindows()
		if (cfg.Reverse || cfg.Bidir) && cfg.SSHClient != nil {
			if lap, ok := cfg.SSHClient.(iperf.LocalAddrProvider); ok {
				cfg.LocalAddr = lap.LocalAddr()
			}
		}

		if cfg.Repeat {
			return runCLIRepeat(cfg)
		}

		result, err := cli.LocalTestRunner(*cfg)
		if err != nil {
			return err
		}
		cli.PrintResult(result)
	}

	return nil
}

func runCLIRepeat(cfg *cli.RunnerConfig) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	var stopped int32

	go func() {
		<-sigCh
		fmt.Println("\nStop requested — finishing current measurement...")
		atomic.StoreInt32(&stopped, 1)
	}()

	totalRuns := 0
	for runNum := 1; ; runNum++ {
		if atomic.LoadInt32(&stopped) == 1 {
			break
		}
		if cfg.RepeatCount > 0 && runNum > cfg.RepeatCount {
			break
		}

		if runNum > 1 {
			fmt.Printf("\n--- Repeat run %d", runNum)
			if cfg.RepeatCount > 0 {
				fmt.Printf(" of %d", cfg.RepeatCount)
			}
			fmt.Println(" ---")
		}

		result, err := cli.LocalTestRunner(*cfg)
		totalRuns++
		if err != nil {
			fmt.Fprintf(os.Stderr, "Run %d error: %v\n", runNum, err)
			continue
		}
		cli.PrintResult(result)
	}

	fmt.Printf("\nCompleted %d run(s).\n", totalRuns)
	return nil
}
