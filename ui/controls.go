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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
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
	mu         sync.Mutex
	state      testState
	stopRepeat bool // signals repeat loop to exit; protected by mu
	repeatOn   bool // toggle state of the repeat button

	startBtn      *StyledButton
	stopBtn       *StyledButton
	repeatBtn     *StyledButton
	fileNameEntry *widget.Entry

	configForm     *ConfigForm
	outputView     *OutputView
	savedFilesList *SavedFilesList
	remotePanel    *RemotePanel
	runner         *iperf.Runner
	win            fyne.Window

	container *fyne.Container
}

// NewControls creates the control buttons wired to the given views.
// Set IPERF_DEBUG=1 in the environment to enable raw stream logging to
// /tmp/iperf-debug.log.
func NewControls(cf *ConfigForm, ov *OutputView, sfl *SavedFilesList, rp *RemotePanel, win fyne.Window) *Controls {
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
		win:            win,
	}

	white := color.White
	greenBg := color.NRGBA{R: 40, G: 167, B: 69, A: 255}
	redBg := color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	repeatOffBg := color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	c.startBtn = NewStyledButton("Start Test", c.onStart, greenBg, white)
	c.stopBtn = NewStyledButton("Stop Test", c.onStop, redBg, white)
	c.stopBtn.Disable()

	c.repeatBtn = NewStyledButton("Repeat: Off", c.onRepeatToggle, repeatOffBg, white)

	c.fileNameEntry = widget.NewEntry()
	c.fileNameEntry.SetPlaceHolder("results/results")

	c.container = container.NewVBox(
		c.startBtn,
		c.stopBtn,
		c.repeatBtn,
		widget.NewLabel("Output File Path and Name"),
		c.fileNameEntry,
	)
	return c
}

// Container returns the controls container.
func (c *Controls) Container() *fyne.Container {
	return c.container
}

func (c *Controls) onRepeatToggle() {
	c.repeatOn = !c.repeatOn
	if c.repeatOn {
		c.repeatBtn.Text = "Repeat: On"
		c.repeatBtn.bgColor = color.NRGBA{R: 204, G: 122, B: 0, A: 255}
	} else {
		c.repeatBtn.Text = "Repeat: Off"
		c.repeatBtn.bgColor = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	}
	c.repeatBtn.Refresh()
}

// LoadPreferences restores persisted control state.
func (c *Controls) LoadPreferences(prefs fyne.Preferences) {
	if prefs.Bool("controls.repeat") {
		c.repeatOn = true
		c.repeatBtn.Text = "Repeat: On"
		c.repeatBtn.bgColor = color.NRGBA{R: 204, G: 122, B: 0, A: 255}
		c.repeatBtn.Refresh()
	}
	if v := prefs.String("controls.output_path"); v != "" {
		c.fileNameEntry.SetText(v)
	}
}

// SavePreferences persists control state.
func (c *Controls) SavePreferences(prefs fyne.Preferences) {
	prefs.SetBool("controls.repeat", c.repeatOn)
	prefs.SetString("controls.output_path", c.fileNameEntry.Text)
}

func (c *Controls) onStart() {
	c.mu.Lock()
	if c.state == stateRunning {
		c.mu.Unlock()
		return
	}
	c.state = stateRunning
	c.stopRepeat = false
	c.mu.Unlock()

	c.startBtn.Disable()
	c.stopBtn.Enable()

	cfg := c.configForm.Config()

	if err := cfg.Validate(); err != nil {
		c.outputView.AppendLine("Config error: " + err.Error())
		c.resetState()
		return
	}

	c.outputView.Clear()

	go func() {
		defer c.resetState()
		for runNum := 1; ; runNum++ {
			if runNum > 1 {
				c.outputView.AppendLine(fmt.Sprintf("--- Repeat run %d ---", runNum))
			}
			if !c.runOnce(cfg) {
				break
			}
		}
	}()
}

// runOnce executes a single iperf2 measurement and returns true if the repeat
// loop should continue, false if it should stop.
func (c *Controls) runOnce(cfg iperf.IperfConfig) bool {
	c.outputView.AppendLine(fmt.Sprintf("Starting iperf2 test to %s:%d ...", cfg.ServerAddr, cfg.Port))

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

	iperfVersion, _ := iperf.CheckVersion(cfg.BinaryPath)

	// Print interval header
	isUDP := strings.EqualFold(cfg.Protocol, "udp")
	var header string
	if cfg.Bidir {
		header = "Time      " + format.FormatBidirIntervalHeader(isUDP)
	} else {
		header = "Time      " + format.FormatIntervalHeader(isUDP)
	}
	c.outputView.AppendLine("")
	c.outputView.AppendLine(header)
	c.outputView.AppendLine(strings.Repeat("-", len(header)))

	testStart := time.Now()
	onInterval := func(fwd, rev *model.IntervalResult) {
		if fwd == nil {
			return
		}
		ts := testStart.Add(time.Duration(fwd.TimeStart * float64(time.Second))).Format("15:04:05")
		if cfg.Bidir && rev != nil {
			c.outputView.AppendLine(ts + "  " + format.FormatBidirInterval(fwd, rev, isUDP))
		} else {
			c.outputView.AppendLine(ts + "  " + format.FormatInterval(fwd, isUDP))
		}
	}

	// Dispatch based on direction
	var result *model.TestResult
	var err error
	if cfg.Bidir {
		result, err = c.runner.RunBidir(ctx, cfg, nil, onInterval)
	} else if cfg.Reverse {
		result, err = c.runner.RunReverse(ctx, cfg, nil, onInterval)
	} else {
		result, err = c.runner.RunForward(ctx, cfg, nil, onInterval)
	}

	// If the test failed to reach the server and we have an SSH connection,
	// start (or restart) the remote iperf2 and retry once.
	if err != nil && isServerUnreachable(err) {
		if c.remotePanel.IsConnected() {
			c.outputView.AppendLine("Server not responding, starting remote iperf2...")
			if restartErr := c.remotePanel.RestartServer(); restartErr != nil {
				c.outputView.AppendLine(fmt.Sprintf("Start failed: %v", restartErr))
			} else {
				c.outputView.AppendLine("Server started, retrying test...")
				time.Sleep(time.Second)
				if cfg.Bidir {
					result, err = c.runner.RunBidir(ctx, cfg, nil, onInterval)
				} else if cfg.Reverse {
					result, err = c.runner.RunReverse(ctx, cfg, nil, onInterval)
				} else {
					result, err = c.runner.RunForward(ctx, cfg, nil, onInterval)
				}
			}
		} else {
			c.outputView.AppendLine("Tip: connect via SSH in the Remote panel, then retry — the server will be started automatically.")
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
		return false
	}

	// Set config echo fields on the successful result.
	cfg.ApplyToResult(result, "GUI")
	result.LocalHostname = hostname
	result.LocalIP = localIP
	result.IperfVersion = iperfVersion
	result.PingBaseline = pingBaseline
	result.PingLoaded = pingLoaded
	if c.remotePanel.IsConnected() {
		result.SSHRemoteHost = c.remotePanel.Host()
	}
	result.MeasurementID = export.NextMeasurementID(result.Timestamp)

	c.outputView.AppendLine("")
	c.outputView.AppendLine(format.FormatResult(result))

	c.autoSave(result)

	c.mu.Lock()
	cont := c.repeatOn && !c.stopRepeat
	c.mu.Unlock()
	return cont
}

func isServerUnreachable(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "server is busy") ||
		strings.Contains(msg, "unable to connect") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "Operation timed out") ||
		strings.Contains(msg, "Connection reset by peer")
}

// onStop is always called on the UI thread (button tap handler).
func (c *Controls) onStop() {
	c.mu.Lock()
	if c.state != stateRunning {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	// Read widget state on UI thread, outside the mutex.
	if c.repeatOn {
		var d *dialog.CustomDialog
		interruptBtn := widget.NewButtonWithIcon("Interrupt Now", theme.MediaStopIcon(), func() {
			d.Hide()
			c.mu.Lock()
			c.stopRepeat = true
			c.mu.Unlock()
			c.runner.Stop()
		})
		finishBtn := widget.NewButtonWithIcon("Finish This Run", theme.MediaReplayIcon(), func() {
			d.Hide()
			c.mu.Lock()
			c.stopRepeat = true
			c.mu.Unlock()
		})
		content := container.NewVBox(
			widget.NewLabel("Interrupt the current measurement now, or let it finish first?"),
			container.NewHBox(interruptBtn, finishBtn),
		)
		d = dialog.NewCustom("Stop Repeat", "Cancel", content, c.win)
		d.Show()
		return
	}

	// Non-repeat: stop immediately as before.
	c.runner.Stop()
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
	c.stopRepeat = false
	c.mu.Unlock()
	fyne.Do(func() {
		c.startBtn.Enable()
		c.stopBtn.Disable()
	})
}
