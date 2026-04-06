package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const windowsHostsPrefKey = "remote.known_windows_hosts"

// BuildMainWindow creates and configures the main application window.
func BuildMainWindow(app fyne.App) fyne.Window {
	win := app.NewWindow("iperf2 Test Tool")

	configForm := NewConfigForm()
	remotePanel := NewRemotePanel(win)
	outputView := NewOutputView()
	savedFilesList := NewSavedFilesList()
	controls := NewControls(configForm, outputView, savedFilesList, remotePanel, win)

	prefs := app.Preferences()

	// Auto-fill the server address from the SSH host on successful connect.
	remotePanel.OnConnect = func(host string) {
		configForm.SetServerAddrIfEmpty(host)
	}

	// Remember Windows hosts so we can warn about UDP-without-SSH next time.
	remotePanel.OnOSDetected = func(host string, isWindows bool) {
		if host == "" {
			return
		}
		known := loadKnownWindowsHosts(prefs)
		if isWindows {
			if _, ok := known[host]; !ok {
				known[host] = struct{}{}
				saveKnownWindowsHosts(prefs, known)
			}
		} else if _, ok := known[host]; ok {
			delete(known, host)
			saveKnownWindowsHosts(prefs, known)
		}
	}

	// Controls consults this to decide whether UDP-without-SSH should warn.
	controls.IsHostKnownWindows = func(host string) bool {
		if host == "" {
			return false
		}
		known := loadKnownWindowsHosts(prefs)
		_, ok := known[host]
		return ok
	}

	// Show informational note when switching to UDP with a Windows remote.
	configForm.OnProtocolChange = func(protocol string) {
		if protocol == "UDP" && remotePanel.IsConnected() && remotePanel.IsWindows() {
			outputView.AppendLine("Note: UDP + Windows remote — SSH fallback will be used for server-side statistics.")
		}
	}

	configForm.LoadPreferences(prefs)
	remotePanel.LoadPreferences(prefs)
	controls.LoadPreferences(prefs)

	leftPanel := container.NewVBox(
		configForm.Container(),
		widget.NewSeparator(),
		controls.Container(),
	)
	centerPanel := container.NewVBox(
		remotePanel.Container(),
	)
	rightPanel := container.NewStack(
		savedFilesList.Container(),
	)

	showFiles := prefs.BoolWithFallback("ui.show_files", false)
	if !showFiles {
		rightPanel.Hide()
	}

	leftCenter := container.NewHSplit(leftPanel, centerPanel)
	leftCenter.SetOffset(0.5)

	// Right panel gets ~33% when visible (col1=33%, col2=33%, col3=33%)
	mainArea := container.NewHSplit(leftCenter, rightPanel)
	mainArea.SetOffset(0.67)

	filesBtn := widget.NewButton("Files", func() {})
	filesBtn.Importance = widget.LowImportance
	if showFiles {
		filesBtn.SetText("Files ✓")
	}
	filesBtn.OnTapped = func() {
		showFiles = !showFiles
		if showFiles {
			filesBtn.SetText("Files ✓")
			rightPanel.Show()
		} else {
			filesBtn.SetText("Files")
			rightPanel.Hide()
		}
		mainArea.Refresh()
	}

	topBar := container.NewBorder(nil, nil, nil, container.NewHBox(filesBtn))
	upper := container.NewBorder(topBar, nil, nil, nil, mainArea)
	content := container.NewVSplit(upper, outputView.Container())
	content.SetOffset(MainSplitRatio)

	// Restore saved window size or use defaults
	w := prefs.FloatWithFallback("ui.window_width", float64(WindowWidth))
	h := prefs.FloatWithFallback("ui.window_height", float64(WindowHeight))
	win.Resize(fyne.NewSize(float32(w), float32(h)))

	win.SetContent(content)

	win.SetCloseIntercept(func() {
		configForm.SavePreferences(prefs)
		remotePanel.SavePreferences(prefs)
		controls.SavePreferences(prefs)
		prefs.SetBool("ui.show_files", showFiles)
		// Save window size
		size := win.Canvas().Size()
		prefs.SetFloat("ui.window_width", float64(size.Width))
		prefs.SetFloat("ui.window_height", float64(size.Height))
		win.Close()
	})

	return win
}

// loadKnownWindowsHosts returns the set of hosts previously detected as Windows.
func loadKnownWindowsHosts(prefs fyne.Preferences) map[string]struct{} {
	s := prefs.String(windowsHostsPrefKey)
	out := make(map[string]struct{})
	if s == "" {
		return out
	}
	for _, h := range strings.Split(s, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			out[h] = struct{}{}
		}
	}
	return out
}

// saveKnownWindowsHosts persists the set of Windows hosts to preferences.
func saveKnownWindowsHosts(prefs fyne.Preferences, hosts map[string]struct{}) {
	parts := make([]string, 0, len(hosts))
	for h := range hosts {
		parts = append(parts, h)
	}
	prefs.SetString(windowsHostsPrefKey, strings.Join(parts, ","))
}
