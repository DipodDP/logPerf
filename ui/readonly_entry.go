package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// readOnlyEntry is an Entry that allows selection and copy but rejects all edits.
type readOnlyEntry struct {
	widget.Entry
}

func newReadOnlyEntry() *readOnlyEntry {
	e := &readOnlyEntry{}
	e.MultiLine = true
	e.TextStyle = fyne.TextStyle{Monospace: true}
	e.ExtendBaseWidget(e)
	return e
}

// TypedRune blocks all character input.
func (e *readOnlyEntry) TypedRune(_ rune) {}

// TypedKey allows only navigation and selection keys, blocks editing keys.
func (e *readOnlyEntry) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyBackspace, fyne.KeyDelete, fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
		return // block editing keys
	}
	e.Entry.TypedKey(ev)
}

// TypedShortcut allows copy and select-all, blocks cut and paste.
func (e *readOnlyEntry) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		e.Entry.TypedShortcut(s)
	case *desktop.CustomShortcut:
		e.Entry.TypedShortcut(s)
	}
	// Block paste, cut, and other modifying shortcuts
}
