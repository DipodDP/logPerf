package main

import (
	"fyne.io/fyne/v2/app"

	"iperf-tool/ui"
)

func main() {
	a := app.New()
	win := ui.BuildMainWindow(a)
	win.ShowAndRun()
}
