package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"iperf-tool/internal/export"
	"iperf-tool/internal/format"
	"iperf-tool/internal/iperf"
	"iperf-tool/internal/model"
	"iperf-tool/internal/ping"
)

type testState int

const (
	stateIdle testState = iota
	stateRunning
)

// Controls manages the Start/Stop buttons and test execution with auto-save.
type Controls struct {
	mu    sync.Mutex
	state testState

	startBtn *widget.Button
	stopBtn  *widget.Button

	configForm  *ConfigForm
	outputView  *OutputView
	historyView *HistoryView
	remotePanel *RemotePanel
	runner      *iperf.Runner

	container *fyne.Container
}

// NewControls creates the control buttons wired to the given views.
func NewControls(cf *ConfigForm, ov *OutputView, hv *HistoryView, rp *RemotePanel) *Controls {
	c := &Controls{
		configForm:  cf,
		outputView:  ov,
		historyView: hv,
		remotePanel: rp,
		runner:      iperf.NewRunner(),
	}

	c.startBtn = widget.NewButton("Start Test", c.onStart)
	c.stopBtn = widget.NewButton("Stop Test", c.onStop)
	c.stopBtn.Disable()

	c.container = container.NewHBox(c.startBtn, c.stopBtn)
	return c
}

// Container returns the controls container.
func (c *Controls) Container() *fyne.Container {
	return c.container
}

func (c *Controls) onStart() {
	c.mu.Lock()
	if c.state == stateRunning {
		c.mu.Unlock()
		return
	}
	c.state = stateRunning
	c.mu.Unlock()

	c.startBtn.Disable()
	c.stopBtn.Enable()
	c.outputView.Clear()

	cfg := c.configForm.Config()

	if err := cfg.Validate(); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Config error: %v", err))
		c.resetState()
		return
	}

	go func() {
		defer c.resetState()

		c.outputView.AppendLine(fmt.Sprintf("Starting iperf3 test to %s:%d ...", cfg.ServerAddr, cfg.Port))

		ctx := context.Background()

		// Phase 1: baseline ping
		var baseline *ping.Result
		if cfg.MeasurePing {
			c.outputView.AppendLine("Running baseline ping (4 packets)...")
			var err error
			baseline, err = ping.Run(ctx, cfg.ServerAddr, 4)
			if err != nil {
				c.outputView.AppendLine(fmt.Sprintf("Baseline ping failed: %v", err))
			} else {
				c.outputView.AppendLine(fmt.Sprintf("Baseline latency: min/avg/max = %.2f / %.2f / %.2f ms",
					baseline.MinMs, baseline.AvgMs, baseline.MaxMs))
			}
		}

		// Phase 2: start background ping during iperf
		var loadedCh chan *ping.Result
		var pingCancel context.CancelFunc
		if cfg.MeasurePing {
			var pingCtx context.Context
			pingCtx, pingCancel = context.WithCancel(ctx)
			loadedCh = make(chan *ping.Result, 1)
			go func() {
				loaded, err := ping.RunUntilCancel(pingCtx, cfg.ServerAddr)
				if err != nil {
					loadedCh <- nil
				} else {
					loadedCh <- loaded
				}
			}()
		}

		_, versionErr := iperf.CheckVersion(cfg.BinaryPath)
		useStream := versionErr == nil

		result, err := c.runTest(cfg, useStream)

		// If the server is busy and we have an SSH connection, restart and retry once.
		if err != nil && isServerBusy(err) && c.remotePanel.IsConnected() {
			c.outputView.AppendLine("Server is busy, restarting remote iperf3...")
			if restartErr := c.remotePanel.RestartServer(); restartErr != nil {
				c.outputView.AppendLine(fmt.Sprintf("Restart failed: %v", restartErr))
			} else {
				c.outputView.AppendLine("Server restarted, retrying test...")
				time.Sleep(time.Second)
				result, err = c.runTest(cfg, useStream)
			}
		}

		// Stop background ping and collect result
		if cfg.MeasurePing && pingCancel != nil {
			pingCancel()
			loaded := <-loadedCh
			if result != nil {
				if baseline != nil {
					result.PingBaseline = &model.PingResult{
						PacketsSent: baseline.PacketsSent,
						PacketsRecv: baseline.PacketsRecv,
						PacketLoss:  baseline.PacketLoss,
						MinMs:       baseline.MinMs,
						AvgMs:       baseline.AvgMs,
						MaxMs:       baseline.MaxMs,
					}
				}
				if loaded != nil {
					result.PingLoaded = &model.PingResult{
						PacketsSent: loaded.PacketsSent,
						PacketsRecv: loaded.PacketsRecv,
						PacketLoss:  loaded.PacketLoss,
						MinMs:       loaded.MinMs,
						AvgMs:       loaded.AvgMs,
						MaxMs:       loaded.MaxMs,
					}
				}
			}
		}

		if err != nil {
			c.outputView.AppendLine(fmt.Sprintf("Error: %v", err))
			errResult := model.TestResult{
				Timestamp:  time.Now(),
				ServerAddr: cfg.ServerAddr,
				Port:       cfg.Port,
				Protocol:   cfg.Protocol,
				Duration:   cfg.Duration,
				Parallel:   cfg.Parallel,
				Error:      err.Error(),
			}
			c.historyView.AddResult(errResult)
			c.autoSave(&errResult)
			return
		}

		if useStream {
			c.outputView.AppendLine("")
			c.outputView.AppendLine(format.FormatResult(result))
		} else {
			c.outputView.Clear()
			c.outputView.AppendLine(format.FormatResult(result))
		}

		c.historyView.AddResult(*result)
		c.autoSave(result)
	}()
}

// runTest executes a single iperf3 test, printing live output along the way.
func (c *Controls) runTest(cfg iperf.IperfConfig, useStream bool) (*model.TestResult, error) {
	if useStream {
		c.outputView.AppendLine(format.FormatIntervalHeader())
		c.outputView.AppendLine(strings.Repeat("-", 60))
		return c.runner.RunWithIntervals(nil, cfg, func(interval *model.IntervalResult) {
			c.outputView.AppendLine(format.FormatInterval(interval))
		})
	}

	c.outputView.AppendLine("Falling back to standard JSON mode (no live intervals)")
	return c.runner.RunWithPipe(nil, cfg, func(line string) {
		c.outputView.AppendLine(line)
	})
}

func isServerBusy(err error) bool {
	return strings.Contains(err.Error(), "server is busy")
}

func (c *Controls) onStop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == stateRunning {
		c.runner.Stop()
	}
}

func (c *Controls) autoSave(result *model.TestResult) {
	const (
		csvPath         = "iperf_results.csv"
		txtPath         = "iperf_results.txt"
		intervalLogPath = "iperf_results_log.csv"
	)

	if err := export.WriteCSV(csvPath, []model.TestResult{*result}); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Auto-save CSV error: %v", err))
	}

	allResults := c.historyView.Results()
	if err := export.WriteTXT(txtPath, allResults); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Auto-save TXT error: %v", err))
	}

	if len(result.Intervals) > 0 {
		if err := export.WriteIntervalLog(intervalLogPath, result.Intervals); err != nil {
			c.outputView.AppendLine(fmt.Sprintf("Auto-save interval log error: %v", err))
		}
	}

	c.outputView.AppendLine(fmt.Sprintf("Results saved to %s, %s", csvPath, txtPath))
}

func (c *Controls) resetState() {
	c.mu.Lock()
	c.state = stateIdle
	c.mu.Unlock()
	fyne.Do(func() {
		c.startBtn.Enable()
		c.stopBtn.Disable()
	})
}
