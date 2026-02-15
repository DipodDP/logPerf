package ui

import "fyne.io/fyne/v2"

// Window dimensions
const (
	WindowWidth  = 580
	WindowHeight = 620
)

// Split ratios
const (
	MainSplitRatio = 0.45 // 45% top (grid), 55% bottom (output)
)

// OutputView dimensions
const (
	OutputViewMinWidth  = 200
	OutputViewMinHeight = 100
)

// NewWindowSize returns the default window size
func NewWindowSize() fyne.Size {
	return fyne.NewSize(WindowWidth, WindowHeight)
}

// NewOutputViewMinSize returns the minimum size for the output view
func NewOutputViewMinSize() fyne.Size {
	return fyne.NewSize(OutputViewMinWidth, OutputViewMinHeight)
}
