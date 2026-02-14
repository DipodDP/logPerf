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

func TestWriteCSV_UDP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "127.0.0.1",
		Port:        5201,
		Parallel:    1,
		Duration:    3,
		Protocol:    "UDP",
		SentBps:     1_048_576,
		JitterMs:    0.025,
		LostPackets: 3,
		LostPercent: 6.25,
		Packets:     48,
	}}

	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)

	// Check new headers present
	if !strings.Contains(content, "Jitter_ms") {
		t.Error("CSV should contain Jitter_ms header")
	}
	if !strings.Contains(content, "Lost_Packets") {
		t.Error("CSV should contain Lost_Packets header")
	}
	if !strings.Contains(content, "Lost_Percent") {
		t.Error("CSV should contain Lost_Percent header")
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Check data row contains UDP values
	if !strings.Contains(lines[1], "0.025") {
		t.Errorf("row should contain jitter: %s", lines[1])
	}
	if !strings.Contains(lines[1], "6.25") {
		t.Errorf("row should contain lost percent: %s", lines[1])
	}
}

func TestWriteCSV_WithPing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Parallel:    1,
		Duration:    10,
		Protocol:    "TCP",
		SentBps:     940_000_000,
		ReceivedBps: 936_000_000,
		Retransmits: 5,
		PingBaseline: &model.PingResult{MinMs: 1.0, AvgMs: 2.0, MaxMs: 3.0},
		PingLoaded:   &model.PingResult{MinMs: 5.0, AvgMs: 10.0, MaxMs: 50.0},
	}}

	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, "Ping_Baseline_Avg_ms") {
		t.Error("CSV should contain Ping_Baseline_Avg_ms header")
	}
	if !strings.Contains(content, "Ping_Loaded_Avg_ms") {
		t.Error("CSV should contain Ping_Loaded_Avg_ms header")
	}
	if !strings.Contains(content, "Ping_Loaded_Max_ms") {
		t.Error("CSV should contain Ping_Loaded_Max_ms header")
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Check baseline avg (2.00) and loaded avg (10.00) are present
	if !strings.Contains(lines[1], "2.00") {
		t.Errorf("row should contain baseline avg: %s", lines[1])
	}
	if !strings.Contains(lines[1], "10.00") {
		t.Errorf("row should contain loaded avg: %s", lines[1])
	}
	if !strings.Contains(lines[1], "50.00") {
		t.Errorf("row should contain loaded max: %s", lines[1])
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
