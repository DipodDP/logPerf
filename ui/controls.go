package ui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
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
	"iperf-tool/internal/netutil"
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

	startBtn *StyledButton
	stopBtn  *StyledButton
	fileNameEntry *widget.Entry

	configForm     *ConfigForm
	outputView     *OutputView
	savedFilesList *SavedFilesList
	remotePanel    *RemotePanel
	runner         *iperf.Runner

	container *fyne.Container
}

// NewControls creates the control buttons wired to the given views.
// Set IPERF_DEBUG=1 in the environment to enable raw stream logging to
// /tmp/iperf-debug.log.
func NewControls(cf *ConfigForm, ov *OutputView, sfl *SavedFilesList, rp *RemotePanel) *Controls {
	runner := iperf.NewRunner()
	if os.Getenv("IPERF_DEBUG") == "1" {
		runner = iperf.NewDebugRunner()
	}
	c := &Controls{
		configForm:     cf,
		outputView:     ov,
		savedFilesList: sfl,
		remotePanel:    rp,
		runner:         runner,
	}

	white := color.White
	greenBg := color.NRGBA{R: 40, G: 167, B: 69, A: 255}
	redBg := color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	c.startBtn = NewStyledButton("Start Test", c.onStart, greenBg, white)
	c.stopBtn = NewStyledButton("Stop Test", c.onStop, redBg, white)
	c.stopBtn.Disable()

	c.fileNameEntry = widget.NewEntry()
	c.fileNameEntry.SetPlaceHolder("results/results")

	c.container = container.NewVBox(
		c.startBtn,
		c.stopBtn,
		widget.NewLabel("Output File Path and Name"),
		c.fileNameEntry,
	)
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

		iperfVersion, versionErr := iperf.CheckVersion(cfg.BinaryPath)
		useStream := versionErr == nil

		// Check congestion control support
		supportsCongestion := iperf.SupportsCongestionControl(cfg.BinaryPath)
		if cfg.Congestion != "" && !supportsCongestion {
			c.outputView.AppendLine("Warning: Congestion control not supported on this platform, ignoring -C flag")
		}

		result, err := c.runTest(cfg, useStream)

		// If the server is busy and we have an SSH connection, restart and retry once.
		if err != nil && isServerBusy(err) {
			if c.remotePanel.IsConnected() {
				c.outputView.AppendLine("Server is busy, restarting remote iperf3...")
				if restartErr := c.remotePanel.RestartServer(); restartErr != nil {
					c.outputView.AppendLine(fmt.Sprintf("Restart failed: %v", restartErr))
				} else {
					c.outputView.AppendLine("Server restarted, retrying test...")
					time.Sleep(time.Second)
					result, err = c.runTest(cfg, useStream)
				}
			} else {
				c.outputView.AppendLine("Tip: connect via SSH in the Remote panel, then retry — the server will be restarted automatically.")
			}
		}

		// Stop background ping and collect results
		hostname, _ := os.Hostname()
		localIP := netutil.OutboundIP()
		var pingBaseline, pingLoaded *model.PingResult
		if cfg.MeasurePing && pingCancel != nil {
			pingCancel()
			loaded := <-loadedCh
			pingBaseline = baseline.ToModel()
			pingLoaded = loaded.ToModel()
		}

		if err != nil {
			c.outputView.AppendLine(fmt.Sprintf("Error: %v", err))
			errResult := model.TestResult{
				Timestamp:     time.Now(),
				ServerAddr:    cfg.ServerAddr,
				Port:          cfg.Port,
				Protocol:      cfg.Protocol,
				Duration:      cfg.Duration,
				Parallel:      cfg.Parallel,
				BlockSize:     cfg.BlockSize,
				Error:         err.Error(),
				Mode:          "GUI",
				LocalHostname: hostname,
				LocalIP:       localIP,
				IperfVersion:  iperfVersion,
				PingBaseline:  pingBaseline,
				PingLoaded:    pingLoaded,
			}
			if cfg.Bidir {
				errResult.Direction = "Bidirectional"
			} else if cfg.Reverse {
				errResult.Direction = "Reverse"
			}
			errResult.MeasurementID = export.NextMeasurementID(errResult.Timestamp)
			c.autoSave(&errResult)
			return
		}

		// Set config echo fields on the successful result.
		// Always override with config values — parsed values may be empty on
		// partial runs (e.g. connection refused after start event).
		cfg.ApplyToResult(result, "GUI")
		// Only set congestion if it was actually used (platform supports it)
		if supportsCongestion {
			result.Congestion = cfg.Congestion
		}
		result.LocalHostname = hostname
		result.LocalIP = localIP
		result.IperfVersion = iperfVersion
		result.PingBaseline = pingBaseline
		result.PingLoaded = pingLoaded
		if c.remotePanel.IsConnected() {
			result.SSHRemoteHost = c.remotePanel.Host()
		}
		result.MeasurementID = export.NextMeasurementID(result.Timestamp)

		if useStream {
			c.outputView.AppendLine("")
			c.outputView.AppendLine(format.FormatResult(result))
		} else {
			c.outputView.Clear()
			c.outputView.AppendLine(format.FormatResult(result))
		}

		c.autoSave(result)
	}()
}

// runTest executes a single iperf3 test, printing live output along the way.
func (c *Controls) runTest(cfg iperf.IperfConfig, useStream bool) (*model.TestResult, error) {
	if useStream {
		isUDP := strings.EqualFold(cfg.Protocol, "udp")
		var header string
		if cfg.Bidir {
			header = "Time      " + format.FormatBidirIntervalHeader(isUDP)
		} else {
			header = "Time      " + format.FormatIntervalHeader(isUDP)
		}
		c.outputView.AppendLine("")
		c.outputView.AppendLine("=== Client-Side Results ===")
		c.outputView.AppendLine(header)
		c.outputView.AppendLine(strings.Repeat("-", len(header)))
		testStart := time.Now()
		return c.runner.RunWithIntervals(nil, cfg, func(fwd, rev *model.IntervalResult) {
			ts := testStart.Add(time.Duration(fwd.TimeStart * float64(time.Second))).Format("15:04:05")
			if rev != nil {
				c.outputView.AppendLine(ts + "  " + format.FormatBidirInterval(fwd, rev, isUDP))
			} else {
				c.outputView.AppendLine(ts + "  " + format.FormatInterval(fwd, isUDP))
			}
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
	baseName := strings.TrimSuffix(c.fileNameEntry.Text, ".csv")
	if baseName == "" {
		baseName = "results/results"
	} else if filepath.Dir(baseName) == "." {
		// Bare name with no directory component: put it under ./results/
		baseName = filepath.Join("results", baseName)
	}

	dir := filepath.Dir(baseName)
	fyne.Do(func() {
		c.savedFilesList.SetDir(dir)
	})

	if err := export.EnsureDir(baseName + ".csv"); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Auto-save error (mkdir): %v", err))
		return
	}

	date := result.Timestamp
	logPath := export.BuildLogPath(baseName, "_log", ".csv")
	csvPath := export.BuildPath(baseName, "", ".csv", date)
	txtPath := export.BuildPath(baseName, "", ".txt", date)

	if err := export.WriteCSV(logPath, []model.TestResult{*result}); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Auto-save CSV error: %v", err))
	}

	if err := export.WriteTXT(txtPath, []model.TestResult{*result}); err != nil {
		c.outputView.AppendLine(fmt.Sprintf("Auto-save TXT error: %v", err))
	}

	if len(result.Intervals) > 0 {
		if err := export.WriteIntervalLog(csvPath, result); err != nil {
			c.outputView.AppendLine(fmt.Sprintf("Auto-save interval log error: %v", err))
		}
	}

	c.outputView.AppendLine(fmt.Sprintf("Results saved to %s, %s", logPath, txtPath))

	// Refresh file list on UI thread
	fyne.Do(func() {
		c.savedFilesList.Refresh()
	})
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
