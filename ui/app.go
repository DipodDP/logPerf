package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// BuildMainWindow creates and configures the main application window.
func BuildMainWindow(app fyne.App) fyne.Window {
	win := app.NewWindow("iperf3 Test Tool")

	configForm := NewConfigForm()
	remotePanel := NewRemotePanel()
	outputView := NewOutputView()
	savedFilesList := NewSavedFilesList()
	controls := NewControls(configForm, outputView, savedFilesList, remotePanel)

	prefs := app.Preferences()
	configForm.LoadPreferences(prefs)
	remotePanel.LoadPreferences(prefs)

	leftPanel := container.NewVBox(
		configForm.Container(),
		widget.NewSeparator(),
		controls.Container(),
	)
	centerPanel := container.NewVBox(
		remotePanel.Container(),
	)
	rightPanel := container.NewVBox(
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
		prefs.SetBool("ui.show_files", showFiles)
		// Save window size
		size := win.Canvas().Size()
		prefs.SetFloat("ui.window_width", float64(size.Width))
		prefs.SetFloat("ui.window_height", float64(size.Height))
		win.Close()
	})

	return win
}
