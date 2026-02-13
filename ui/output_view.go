package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// OutputView displays live scrolling output from iperf3.
type OutputView struct {
	text      *widget.Entry
	scrollBox *container.Scroll
}

// NewOutputView creates a new scrollable output view.
func NewOutputView() *OutputView {
	ov := &OutputView{}

	ov.text = widget.NewMultiLineEntry()
	ov.text.Wrapping = fyne.TextWrapWord
	ov.text.Disable() // read-only

	ov.scrollBox = container.NewVScroll(ov.text)
	ov.scrollBox.SetMinSize(fyne.NewSize(800, 250))

	return ov
}

// Container returns the output view's container.
func (ov *OutputView) Container() *container.Scroll {
	return ov.scrollBox
}

// AppendLine adds a line to the output view, safe to call from any goroutine.
func (ov *OutputView) AppendLine(line string) {
	fyne.Do(func() {
		current := ov.text.Text
		if current != "" {
			current += "\n"
		}
		ov.text.SetText(current + line)
		ov.scrollBox.ScrollToBottom()
	})
}

// Clear empties the output view, safe to call from any goroutine.
func (ov *OutputView) Clear() {
	fyne.Do(func() {
		ov.text.SetText("")
	})
}
