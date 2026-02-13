package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// BuildMainWindow creates and configures the main application window.
func BuildMainWindow(app fyne.App) fyne.Window {
	win := app.NewWindow("iperf3 Test Tool")
	win.Resize(fyne.NewSize(900, 650))

	configForm := NewConfigForm()
	outputView := NewOutputView()
	historyView := NewHistoryView()
	remotePanel := NewRemotePanel()
	controls := NewControls(configForm, outputView, historyView, remotePanel)

	prefs := app.Preferences()
	configForm.LoadPreferences(prefs)
	remotePanel.LoadPreferences(prefs)

	leftPanel := container.NewVBox(
		configForm.Container(),
		controls.Container(),
	)

	rightPanel := container.NewVBox(
		remotePanel.Container(),
	)

	topRow := container.NewHSplit(leftPanel, rightPanel)
	topRow.SetOffset(0.6)

	outputTab := container.NewTabItem("Live Output", outputView.Container())
	historyTab := container.NewTabItem("History", historyView.Container())
	tabs := container.NewAppTabs(outputTab, historyTab)

	content := container.NewVSplit(topRow, tabs)
	content.SetOffset(0.45)

	win.SetContent(content)

	win.SetCloseIntercept(func() {
		configForm.SavePreferences(prefs)
		remotePanel.SavePreferences(prefs)
		win.Close()
	})

	return win
}
