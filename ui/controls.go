package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"iperf-tool/internal/export"
	"iperf-tool/internal/format"
	"iperf-tool/internal/iperf"
	"iperf-tool/internal/model"
)

type testState int

const (
	stateIdle testState = iota
	stateRunning
)

// Controls manages the Start/Stop/Export buttons and test execution.
type Controls struct {
	mu     sync.Mutex
	state  testState
	cancel context.CancelFunc

	startBtn  *widget.Button
	stopBtn   *widget.Button
	exportBtn *widget.Button

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
	c.exportBtn = widget.NewButton("Export CSV", c.onExport)

	c.container = container.NewHBox(c.startBtn, c.stopBtn, c.exportBtn)
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

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	go func() {
		defer c.resetState()

		c.outputView.AppendLine(fmt.Sprintf("Starting iperf3 test to %s:%d ...", cfg.ServerAddr, cfg.Port))

		// Try json-stream mode for live interval display
		_, versionErr := iperf.CheckVersion(cfg.BinaryPath)
		useStream := versionErr == nil

		var result *model.TestResult
		var err error

		if useStream {
			c.outputView.AppendLine(format.FormatIntervalHeader())
			c.outputView.AppendLine(strings.Repeat("-", 60))

			result, err = c.runner.RunWithIntervals(ctx, cfg, func(interval *model.IntervalResult) {
				c.outputView.AppendLine(format.FormatInterval(interval))
			})
		} else {
			c.outputView.AppendLine(fmt.Sprintf("Note: %v", versionErr))
			c.outputView.AppendLine("Falling back to standard JSON mode (no live intervals)")
			result, err = c.runner.RunWithPipe(ctx, cfg, func(line string) {
				c.outputView.AppendLine(line)
			})
		}

		if err != nil {
			if ctx.Err() == context.Canceled {
				c.outputView.AppendLine("Test cancelled.")
				return
			}
			c.outputView.AppendLine(fmt.Sprintf("Error: %v", err))

			// Store error result in history
			c.historyView.AddResult(model.TestResult{
				Timestamp:  time.Now(),
				ServerAddr: cfg.ServerAddr,
				Port:       cfg.Port,
				Protocol:   cfg.Protocol,
				Duration:   cfg.Duration,
				Parallel:   cfg.Parallel,
				Error:      err.Error(),
			})
			return
		}

		if useStream {
			// In stream mode, keep intervals visible and append summary
			c.outputView.AppendLine("")
			c.outputView.AppendLine(format.FormatResult(result))
		} else {
			// In fallback mode, replace raw JSON with formatted result
			c.outputView.Clear()
			c.outputView.AppendLine(format.FormatResult(result))
		}

		c.historyView.AddResult(*result)
	}()
}

func (c *Controls) onStop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Controls) onExport() {
	results := c.historyView.Results()
	if len(results) == 0 {
		c.outputView.AppendLine("No results to export.")
		return
	}

	// Use a save dialog if we have a window; otherwise default path.
	win := fyne.CurrentApp().Driver().AllWindows()
	if len(win) > 0 {
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			path := writer.URI().Path()
			writer.Close()

			if exportErr := export.WriteCSV(path, results); exportErr != nil {
				c.outputView.AppendLine(fmt.Sprintf("CSV export error: %v", exportErr))
				return
			}
			c.outputView.AppendLine(fmt.Sprintf("Exported %d results to %s", len(results), path))

			txtPath := strings.TrimSuffix(path, ".csv") + ".txt"
			if exportErr := export.WriteTXT(txtPath, results); exportErr != nil {
				c.outputView.AppendLine(fmt.Sprintf("TXT export error: %v", exportErr))
			} else {
				c.outputView.AppendLine(fmt.Sprintf("Exported %d results to %s", len(results), txtPath))
			}
		}, win[0])
	}
}

func (c *Controls) resetState() {
	c.mu.Lock()
	c.state = stateIdle
	c.cancel = nil
	c.mu.Unlock()
	fyne.Do(func() {
		c.startBtn.Enable()
		c.stopBtn.Disable()
	})
}
