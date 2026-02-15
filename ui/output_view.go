package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// OutputView displays live scrolling output from iperf3.
// Text is selectable and copiable.
type OutputView struct {
	text      *ReadOnlyEntry
	scrollBox *container.Scroll
}

// NewOutputView creates a new scrollable output view.
func NewOutputView() *OutputView {
	ov := &OutputView{}

	ov.text = NewReadOnlyEntry()
	ov.text.Wrapping = fyne.TextWrapWord

	ov.scrollBox = container.NewVScroll(ov.text)
	ov.scrollBox.SetMinSize(NewOutputViewMinSize())

	return ov
}

// Container returns the output view's container.
func (ov *OutputView) Container() *fyne.Container {
	return container.NewMax(ov.scrollBox)
}

// AppendLine adds a line to the output view, safe to call from any goroutine.
func (ov *OutputView) AppendLine(line string) {
	fyne.Do(func() {
		current := ov.text.Text
		if current != "" {
			current += "\n"
		}
		newText := current + line
		ov.text.SetText(newText)
		// Move cursor to end so the Entry's internal scroll shows the bottom.
		ov.text.CursorRow = ov.text.CursorRow + len(newText)
		ov.text.CursorColumn = 0
		// Defer ScrollToBottom so layout recalculates content height first.
		fyne.Do(func() {
			ov.scrollBox.ScrollToBottom()
		})
	})
}

// Clear empties the output view, safe to call from any goroutine.
func (ov *OutputView) Clear() {
	fyne.Do(func() {
		ov.text.SetText("")
	})
}
