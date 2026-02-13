package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iperf-tool/internal/model"
)

func sampleResults() []model.TestResult {
	return []model.TestResult{
		{
			Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			ServerAddr:  "192.168.1.1",
			Port:        5201,
			Parallel:    4,
			Duration:    10,
			Protocol:    "TCP",
			SentBps:     940_000_000,
			ReceivedBps: 936_000_000,
			Retransmits: 42,
		},
	}
}

func TestWriteCSV_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	if err := WriteCSV(path, sampleResults()); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 row), got %d", len(lines))
	}

	// Check header
	if !strings.HasPrefix(lines[0], "Timestamp,") {
		t.Errorf("unexpected header: %s", lines[0])
	}

	// Check data row
	if !strings.Contains(lines[1], "192.168.1.1") {
		t.Errorf("row should contain server address: %s", lines[1])
	}
	if !strings.Contains(lines[1], "940.00") {
		t.Errorf("row should contain sent Mbps: %s", lines[1])
	}
}

func TestWriteCSV_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	// Write first batch
	if err := WriteCSV(path, sampleResults()); err != nil {
		t.Fatalf("first WriteCSV() error: %v", err)
	}

	// Append second batch
	if err := WriteCSV(path, sampleResults()); err != nil {
		t.Fatalf("second WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Should have 1 header + 2 data rows
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestWriteIntervalLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intervals.csv")

	intervals := []model.IntervalResult{
		{TimeStart: 0, TimeEnd: 1, Bytes: 117500000, BandwidthBps: 940_000_000, Retransmits: 3},
		{TimeStart: 1, TimeEnd: 2, Bytes: 115000000, BandwidthBps: 920_000_000, Retransmits: 1},
	}

	if err := WriteIntervalLog(path, intervals); err != nil {
		t.Fatalf("WriteIntervalLog() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}

	if !strings.HasPrefix(lines[0], "interval_start,") {
		t.Errorf("unexpected header: %s", lines[0])
	}
	if !strings.Contains(lines[1], "940.00") {
		t.Errorf("row 1 should contain bandwidth: %s", lines[1])
	}
	if !strings.Contains(lines[2], "920.00") {
		t.Errorf("row 2 should contain bandwidth: %s", lines[2])
	}
}

func TestWriteCSV_WithError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:  time.Now(),
		ServerAddr: "10.0.0.1",
		Port:       5201,
		Error:      "connection refused",
	}}

	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "connection refused") {
		t.Error("CSV should contain error message")
	}
}
