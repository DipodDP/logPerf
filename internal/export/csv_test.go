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

	// Check header uses semicolon separator and starts with date
	if !strings.HasPrefix(lines[0], "date;") {
		t.Errorf("unexpected header: %s", lines[0])
	}
	if !strings.Contains(lines[0], ";time;") {
		t.Errorf("header should contain time column: %s", lines[0])
	}

	// New columns present in header
	for _, h := range []string{"hostname", "local_ip", "test_duration", "actual_duration", "block_size", "fwd_mbps", "fwd_mb", "rev_mbps", "rev_mb"} {
		if !strings.Contains(lines[0], h) {
			t.Errorf("header should contain %q: %s", h, lines[0])
		}
	}

	// Check data row contains server address and forward bandwidth
	if !strings.Contains(lines[1], "192.168.1.1") {
		t.Errorf("row should contain server address: %s", lines[1])
	}
	if !strings.Contains(lines[1], "940.00") {
		t.Errorf("row should contain Fwd_Mbps: %s", lines[1])
	}
	// Verify semicolon separator used (not comma)
	if strings.Contains(lines[0], ",Date") || strings.Contains(lines[1], ",192") {
		t.Errorf("should use semicolon separator, not comma")
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

func TestWriteCSV_DateTimeSplit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:  time.Date(2026, 2, 18, 14, 32, 7, 0, time.UTC),
		ServerAddr: "192.168.1.1",
		Port:       5201,
	}}
	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "18.02.2026") {
		t.Error("CSV should contain date in DD.MM.YYYY format")
	}
	if !strings.Contains(content, "14:32:07") {
		t.Error("CSV should contain time in HH:MM:SS format")
	}
}

func TestWriteCSV_NewMetaColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:     time.Date(2026, 2, 18, 14, 32, 7, 0, time.UTC),
		ServerAddr:    "192.168.1.1",
		Port:          5201,
		MeasurementID: "20260218-143207-01",
		Mode:          "CLI",
		IperfVersion:  "3.17",
	}}
	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	for _, want := range []string{"measurement_id", "mode", "iperf_version", "20260218-143207-01", "CLI", "3.17"} {
		if !strings.Contains(content, want) {
			t.Errorf("CSV should contain %q", want)
		}
	}
	// RemoteHost column and SSHRemoteHost value must be absent
	if strings.Contains(content, "remote_host") {
		t.Errorf("CSV should not contain remote_host column")
	}
}

func TestWriteIntervalLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intervals.csv")

	baseTime := time.Date(2026, 2, 18, 14, 32, 0, 0, time.UTC)
	result := &model.TestResult{
		Timestamp:     baseTime,
		MeasurementID: "20260218-143200-01",
		ServerAddr:    "192.168.1.1",
		Port:          5201,
		Protocol:      "TCP",
		Parallel:      1,
		Duration:      10,
		Intervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, Bytes: 117500000, BandwidthBps: 940_000_000, Retransmits: 3},
			{TimeStart: 1, TimeEnd: 2, Bytes: 115000000, BandwidthBps: 920_000_000, Retransmits: 1},
		},
	}

	if err := WriteIntervalLog(path, result); err != nil {
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

	// Header: starts with measurement_id, contains wall_time and test param columns
	if !strings.HasPrefix(lines[0], "measurement_id;") {
		t.Errorf("unexpected interval header start: %s", lines[0])
	}
	for _, col := range []string{"wall_time", "fwd_bandwidth_mbps", "fwd_transfer_mb", "fwd_packets", "rev_bandwidth_mbps", "rev_jitter_ms", "rev_lost_packets", "protocol", "streams", "server", "port"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header should contain %q: %s", col, lines[0])
		}
	}
	// direction discriminator column must NOT be present (replaced by fwd/rev prefix columns)
	if strings.Contains(lines[0], ";direction;") {
		t.Errorf("header should not contain direction column: %s", lines[0])
	}
	// duration and interval columns must NOT be present in interval log
	if strings.Contains(lines[0], ";duration;") {
		t.Errorf("header should not contain duration column: %s", lines[0])
	}
	if strings.Contains(lines[0], "interval_start") || strings.Contains(lines[0], "interval_end") {
		t.Errorf("header should not contain interval_start/interval_end: %s", lines[0])
	}

	// First row: wall_time = base + 0s = 14:32:00
	if !strings.Contains(lines[1], "2026-02-18T14:32:00") {
		t.Errorf("row 1 should contain wall_time: %s", lines[1])
	}
	if !strings.Contains(lines[1], "940.00") {
		t.Errorf("row 1 should contain fwd bandwidth: %s", lines[1])
	}
	// Test params echoed into each row
	if !strings.Contains(lines[1], "192.168.1.1") {
		t.Errorf("row 1 should contain server addr: %s", lines[1])
	}
	if !strings.Contains(lines[1], "20260218-143200-01") {
		t.Errorf("row 1 should contain measurement_id: %s", lines[1])
	}

	// Second row: wall_time = base + 1s = 14:32:01
	if !strings.Contains(lines[2], "2026-02-18T14:32:01") {
		t.Errorf("row 2 should contain wall_time: %s", lines[2])
	}
	if !strings.Contains(lines[2], "920.00") {
		t.Errorf("row 2 should contain bandwidth: %s", lines[2])
	}
}

func TestWriteIntervalLog_Bidir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intervals_bidir.csv")

	baseTime := time.Date(2026, 2, 18, 14, 32, 0, 0, time.UTC)
	result := &model.TestResult{
		Timestamp:  baseTime,
		ServerAddr: "192.168.1.1",
		Port:       5201,
		Protocol:   "TCP",
		Parallel:   2,
		Duration:   10,
		Direction:  "Bidirectional",
		Intervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, Bytes: 117500000, BandwidthBps: 940_000_000, Retransmits: 2},
		},
		ReverseIntervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, Bytes: 50000000, BandwidthBps: 400_000_000, Retransmits: 0},
		},
	}

	if err := WriteIntervalLog(path, result); err != nil {
		t.Fatalf("WriteIntervalLog() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	// header + 1 combined row (fwd+rev) = 2 lines
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 combined row), got %d\n%s", len(lines), content)
	}
	// Single data row must contain both fwd and rev bandwidth
	if !strings.Contains(lines[1], "940.00") {
		t.Errorf("row should contain fwd bandwidth: %s", lines[1])
	}
	if !strings.Contains(lines[1], "400.00") {
		t.Errorf("row should contain rev bandwidth in same row as fwd: %s", lines[1])
	}
}

func TestWriteIntervalLog_UDP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intervals_udp.csv")

	baseTime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	result := &model.TestResult{
		Timestamp:  baseTime,
		ServerAddr: "192.168.1.1",
		Port:       5201,
		Protocol:   "UDP",
		Parallel:   4,
		Direction:  "Bidirectional",
		Intervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, Bytes: 500_000, BandwidthBps: 4_000_000,
				Packets: 100, LostPackets: 2, LostPercent: 2.0, JitterMs: 0.123},
			{TimeStart: 1, TimeEnd: 2, Bytes: 500_000, BandwidthBps: 4_000_000,
				Packets: 100, LostPackets: 1, LostPercent: 1.0, JitterMs: 0.098},
		},
		ReverseIntervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, Bytes: 475_000, BandwidthBps: 3_800_000,
				Packets: 95, LostPackets: 3, LostPercent: 3.16, JitterMs: 0.200},
			{TimeStart: 1, TimeEnd: 2, Bytes: 475_000, BandwidthBps: 3_800_000,
				Packets: 95, LostPackets: 0, LostPercent: 0.0, JitterMs: 0.150},
		},
	}

	if err := WriteIntervalLog(path, result); err != nil {
		t.Fatalf("WriteIntervalLog() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	lines := strings.Split(strings.TrimSpace(content), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d\n%s", len(lines), content)
	}

	// Header must contain jitter and loss columns
	for _, col := range []string{"rev_jitter_ms", "rev_lost_packets", "rev_lost_percent"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header missing %q: %s", col, lines[0])
		}
	}

	// Row 1: rev jitter and loss
	if !strings.Contains(lines[1], "0.200") {
		t.Errorf("row 1 should contain rev jitter 0.200: %s", lines[1])
	}
	if !strings.Contains(lines[1], "3.16") {
		t.Errorf("row 1 should contain rev lost_percent 3.16: %s", lines[1])
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

	for _, h := range []string{"fwd_jitter_ms", "fwd_lost_packets", "fwd_lost_percent"} {
		if !strings.Contains(content, h) {
			t.Errorf("CSV should contain %s header", h)
		}
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
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
		Timestamp:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		ServerAddr:   "192.168.1.1",
		Port:         5201,
		Parallel:     1,
		Duration:     10,
		Protocol:     "TCP",
		SentBps:      940_000_000,
		ReceivedBps:  936_000_000,
		Retransmits:  5,
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

	for _, h := range []string{"ping_baseline_min_ms", "ping_baseline_avg_ms", "ping_baseline_max_ms", "ping_loaded_min_ms", "ping_loaded_avg_ms", "ping_loaded_max_ms"} {
		if !strings.Contains(content, h) {
			t.Errorf("CSV should contain %s header", h)
		}
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Baseline: min=1.0, avg=2.0, max=3.0
	if !strings.Contains(lines[1], "1.00") {
		t.Errorf("row should contain baseline min: %s", lines[1])
	}
	if !strings.Contains(lines[1], "2.00") {
		t.Errorf("row should contain baseline avg: %s", lines[1])
	}
	if !strings.Contains(lines[1], "3.00") {
		t.Errorf("row should contain baseline max: %s", lines[1])
	}
	// Loaded: min=5.0, avg=10.00, max=50.00
	if !strings.Contains(lines[1], "5.00") {
		t.Errorf("row should contain loaded min: %s", lines[1])
	}
	if !strings.Contains(lines[1], "10.00") {
		t.Errorf("row should contain loaded avg: %s", lines[1])
	}
	if !strings.Contains(lines[1], "50.00") {
		t.Errorf("row should contain loaded max: %s", lines[1])
	}
}

func TestWriteCSV_NewColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Parallel:    1,
		Duration:    10,
		Protocol:    "TCP",
		Direction:   "Reverse",
		Bandwidth:   "100M",
		Congestion:  "bbr",
		SentBps:     940_000_000,
		BytesSent:   1175000000,
		Retransmits: 5,
	}}

	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)

	csvLines := strings.Split(strings.TrimSpace(content), "\n")
	if len(csvLines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(csvLines))
	}
	header, dataRow := csvLines[0], csvLines[1]

	// Verify new column names present; old Bytes_Sent/Bytes_Received are gone
	for _, h := range []string{"direction", "stream_bandwidth", "congestion", "fwd_mbps", "fwd_mb", "rev_mbps", "rev_mb"} {
		if !strings.Contains(header, h) {
			t.Errorf("CSV should contain %s header", h)
		}
	}
	for _, dropped := range []string{"Bytes_Sent", "Bytes_Received", "Reverse_Sent_Mbps", "Reverse_Bytes_Sent", "Reverse_Bytes_Received"} {
		if strings.Contains(header, dropped) {
			t.Errorf("header should not contain dropped column %s", dropped)
		}
	}

	if !strings.Contains(dataRow, "Reverse") {
		t.Errorf("row should contain direction: %s", dataRow)
	}
	if !strings.Contains(dataRow, "100M") {
		t.Errorf("row should contain bandwidth target: %s", dataRow)
	}
	if !strings.Contains(dataRow, "bbr") {
		t.Errorf("row should contain congestion: %s", dataRow)
	}
	// Fwd_MB = BytesSent/1e6 = 1175.00
	if !strings.Contains(dataRow, "1175.00") {
		t.Errorf("row should contain Fwd_MB: %s", dataRow)
	}
}

func TestWriteCSV_Bidir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")

	results := []model.TestResult{{
		Timestamp:            time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		ServerAddr:           "192.168.1.1",
		Port:                 5201,
		Parallel:             2,
		Duration:             10,
		Protocol:             "TCP",
		Direction:            "Bidirectional",
		SentBps:              400_000_000,
		Retransmits:          2,
		BytesSent:            500_000_000,
		ReverseSentBps:       480_000_000,
		ReverseRetransmits:   5,
		ReverseBytesReceived: 590_000_000,
	}}

	if err := WriteCSV(path, results); err != nil {
		t.Fatalf("WriteCSV() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)

	// New column names
	for _, h := range []string{"fwd_mbps", "fwd_mb", "rev_mbps", "rev_mb", "rev_retransmits"} {
		if !strings.Contains(content, h) {
			t.Errorf("CSV should contain %s header", h)
		}
	}
	// Old redundant/null columns must be absent
	for _, dropped := range []string{"Reverse_Sent_Mbps", "Reverse_Received_Mbps", "Reverse_Bytes_Sent", "Reverse_Bytes_Received"} {
		if strings.Contains(strings.Split(strings.TrimSpace(content), "\n")[0], dropped) {
			t.Errorf("header should not contain dropped column %s", dropped)
		}
	}

	csvLines := strings.Split(strings.TrimSpace(content), "\n")
	if len(csvLines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(csvLines))
	}
	dataRow := csvLines[1]

	// Fwd_Mbps = SentBps/1e6 = 400.00
	if !strings.Contains(dataRow, "400.00") {
		t.Errorf("row should contain Fwd_Mbps: %s", dataRow)
	}
	// Fwd_MB = BytesSent/1e6 = 500.00
	if !strings.Contains(dataRow, "500.00") {
		t.Errorf("row should contain Fwd_MB: %s", dataRow)
	}
	// Rev_Mbps = ReverseSentBps/1e6 = 480.00
	if !strings.Contains(dataRow, "480.00") {
		t.Errorf("row should contain Rev_Mbps: %s", dataRow)
	}
	// Rev_MB = TotalRevMB() = ReverseBytesReceived/1e6 = 590.00
	if !strings.Contains(dataRow, "590.00") {
		t.Errorf("row should contain Rev_MB (from ReverseBytesReceived): %s", dataRow)
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
