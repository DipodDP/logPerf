package ui

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"iperf-tool/internal/model"
)

var historyColumns = []string{"Time", "Server", "Sent Mbps", "Received Mbps", "Duration", "Status"}

// HistoryView displays a table of past test results.
type HistoryView struct {
	mu      sync.Mutex
	results []model.TestResult
	table   *widget.Table
}

// NewHistoryView creates a new history table view.
func NewHistoryView() *HistoryView {
	hv := &HistoryView{}

	hv.table = widget.NewTable(
		hv.tableSize,
		hv.createCell,
		hv.updateCell,
	)

	hv.table.SetColumnWidth(0, 160) // Time
	hv.table.SetColumnWidth(1, 140) // Server
	hv.table.SetColumnWidth(2, 100) // Sent
	hv.table.SetColumnWidth(3, 120) // Received
	hv.table.SetColumnWidth(4, 80)  // Duration
	hv.table.SetColumnWidth(5, 120) // Status

	return hv
}

// Container returns the table widget.
func (hv *HistoryView) Container() *widget.Table {
	return hv.table
}

// AddResult appends a test result to the history.
func (hv *HistoryView) AddResult(r model.TestResult) {
	hv.mu.Lock()
	hv.results = append(hv.results, r)
	hv.mu.Unlock()
	hv.table.Refresh()
}

// Results returns a copy of all stored results.
func (hv *HistoryView) Results() []model.TestResult {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	out := make([]model.TestResult, len(hv.results))
	copy(out, hv.results)
	return out
}

func (hv *HistoryView) tableSize() (rows int, cols int) {
	hv.mu.Lock()
	defer hv.mu.Unlock()
	return len(hv.results) + 1, len(historyColumns) // +1 for header
}

func (hv *HistoryView) createCell() fyne.CanvasObject {
	return widget.NewLabel("")
}

func (hv *HistoryView) updateCell(id widget.TableCellID, obj fyne.CanvasObject) {
	label := obj.(*widget.Label)

	if id.Row == 0 {
		label.SetText(historyColumns[id.Col])
		label.TextStyle = fyne.TextStyle{Bold: true}
		return
	}

	hv.mu.Lock()
	defer hv.mu.Unlock()

	idx := id.Row - 1
	if idx >= len(hv.results) {
		label.SetText("")
		return
	}

	r := hv.results[idx]
	label.TextStyle = fyne.TextStyle{}

	switch id.Col {
	case 0:
		label.SetText(r.Timestamp.Format("2006-01-02 15:04:05"))
	case 1:
		label.SetText(r.ServerAddr)
	case 2:
		label.SetText(fmt.Sprintf("%.2f", r.SentMbps()))
	case 3:
		label.SetText(fmt.Sprintf("%.2f", r.ReceivedMbps()))
	case 4:
		label.SetText(fmt.Sprintf("%ds", r.Duration))
	case 5:
		label.SetText(r.Status())
	}
}
