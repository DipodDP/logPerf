package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iperf-tool/internal/model"
)

var baseTXTTime = time.Date(2026, 2, 18, 14, 32, 7, 0, time.UTC)

func TestWriteTXT_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:     baseTXTTime,
			ServerAddr:    "192.168.1.1",
			Port:          5201,
			Protocol:      "TCP",
			Parallel:      1,
			Duration:      10,
			SentBps:       100_000_000,
			ReceivedBps:   95_000_000,
			BytesSent:     125_000_000,
			BytesReceived: 118_000_000,
			Retransmits:   3,
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)
	for _, want := range []string{
		divider,
		"192.168.1.1:5201",
		"TCP",
		"100.00 Mbps",
		"95.00 Mbps",
		"Retransmits:     3",
		"--- Test Parameters ---",
		"Summary",
		// Summary format (aligned labels matching UI FormatResult)
		"Sent:            100.00 Mbps",
		"Received:        95.00 Mbps",
		// transferred shows sent/received MB
		"125.00 MB sent",
		"118.00 MB received",
		// End marker
		"END OF MEASUREMENT",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("TXT missing %q\nFull content:\n%s", want, content)
		}
	}

	// Removed sections should not appear
	for _, notWant := range []string{
		"--- Quality Summary ---",
		"--- Footer ---",
		"--- Summary ---",
	} {
		if strings.Contains(content, notWant) {
			t.Errorf("TXT should not contain %q\nFull content:\n%s", notWant, content)
		}
	}
}

func TestWriteTXT_ReverseMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	// In reverse mode: client receives, BytesSent=0, BytesReceived=N
	results := []model.TestResult{
		{
			Timestamp:        baseTXTTime,
			ServerAddr:       "192.168.1.1",
			Port:             5201,
			Protocol:         "TCP",
			Direction:        "Reverse",
			Parallel:         1,
			Duration:         10,
			ReceivedBps:      300_000_000,
			BytesReceived:    374_870_000,
			ReverseSentBps:   300_000_000,
			ReverseBytesSent: 439_880_000,
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	// Reverse goes through hasReceiver branch: shows Sent/Received/Retransmits
	if !strings.Contains(content, "Received:") {
		t.Errorf("reverse mode should show Received line\n%s", content)
	}
	if !strings.Contains(content, "300.00 Mbps") {
		t.Errorf("reverse mode should show 300.00 Mbps\n%s", content)
	}
	// Transferred line shows both sent and received bytes (mirrors UI behavior)
	if !strings.Contains(content, "374.87 MB received") {
		t.Errorf("reverse mode should show MB received\n%s", content)
	}
}

func TestWriteTXT_BidirMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:            baseTXTTime,
			ServerAddr:           "192.168.1.1",
			Port:                 5201,
			Protocol:             "TCP",
			Direction:            "Bidirectional",
			Parallel:             2,
			Duration:             10,
			SentBps:              400_000_000,
			ReceivedBps:          396_000_000,
			Retransmits:          2,
			BytesSent:            500_000_000,
			BytesReceived:        495_000_000,
			ReverseSentBps:       480_000_000,
			ReverseReceivedBps:   472_000_000,
			ReverseRetransmits:   5,
			ReverseBytesSent:     600_000_000,
			ReverseBytesReceived: 590_000_000,
			Intervals: []model.IntervalResult{
				{TimeStart: 0, TimeEnd: 1, Bytes: 50_000_000, BandwidthBps: 400_000_000, Retransmits: 1},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	// Results table with Fwd/Rev columns header
	if !strings.Contains(content, "Fwd Mbps") {
		t.Error("bidir results table should have Fwd Mbps column")
	}
	if !strings.Contains(content, "Rev Mbps") {
		t.Error("bidir results table should have Rev Mbps column")
	}
	// Summary should use Send:/Receive: style (mirrors UI FormatResult)
	if !strings.Contains(content, "Send:") {
		t.Error("bidir summary should show Send: line")
	}
	if !strings.Contains(content, "Receive:") {
		t.Error("bidir summary should show Receive: line")
	}
	// retransmits shown inline: (retransmits: N)
	if !strings.Contains(content, "(retransmits: 2)") {
		t.Errorf("bidir summary missing fwd retransmits\n%s", content)
	}
	if !strings.Contains(content, "(retransmits: 5)") {
		t.Errorf("bidir summary missing rev retransmits\n%s", content)
	}
	// Forward bandwidth (400.00 Mbps)
	if !strings.Contains(content, "400.00 Mbps") {
		t.Errorf("bidir fwd sent Mbps missing\n%s", content)
	}
	// Reverse bandwidth: ReverseActualMbps prefers ReverseReceivedBps (472) over ReverseSentBps (480)
	if !strings.Contains(content, "472.00 Mbps") {
		t.Errorf("bidir rev actual Mbps missing\n%s", content)
	}
}

func TestWriteTXT_BidirUDPMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:      baseTXTTime,
			ServerAddr:     "192.168.1.1",
			Port:           5201,
			Protocol:       "UDP",
			Direction:      "Bidirectional",
			Parallel:       4,
			Duration:       10,
			SentBps:        4_000_000,
			ReverseSentBps: 3_800_000,
			Packets:        200,
			LostPackets:    4,
			LostPercent:    2.0,
			JitterMs:       0.050,
			Intervals: []model.IntervalResult{
				{TimeStart: 0, TimeEnd: 1, Bytes: 500_000, BandwidthBps: 4_000_000},
			},
			Streams: []model.StreamResult{
				{ID: 1, SentBps: 1_000_000},
				{ID: 2, SentBps: 1_000_000},
				{ID: 3, SentBps: 1_000_000},
				{ID: 4, SentBps: 1_000_000},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	// Interval table: should have Fwd/Rev Mbps but NOT Retr columns
	if !strings.Contains(content, "Fwd Mbps") {
		t.Error("bidir UDP table should have Fwd Mbps column")
	}
	if strings.Contains(content, "Fwd Retr") {
		t.Error("bidir UDP table should NOT have Fwd Retr column")
	}
	if strings.Contains(content, "Rev Retr") {
		t.Error("bidir UDP table should NOT have Rev Retr column")
	}

	// Summary: Client Send/Server Send style (mirrors UI FormatResult)
	if !strings.Contains(content, "Client Send:") {
		t.Error("missing Client Send: line")
	}
	if !strings.Contains(content, "Server Send:") {
		t.Error("missing Server Send: line")
	}
	if strings.Contains(content, "Retransmits:") {
		t.Error("UDP bidir summary should not show Retransmits")
	}

	// Per-stream: inline detail format with [Fwd]/[Rev] labels
	if !strings.Contains(content, "Per-Stream Results") {
		t.Error("UDP bidir stream section should show Per-Stream Results header")
	}
	if !strings.Contains(content, "Stream 1 [") {
		t.Error("UDP bidir stream section should show stream direction labels")
	}
}

func TestWriteTXT_PerStreamSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:   baseTXTTime,
			ServerAddr:  "192.168.1.1",
			Port:        5201,
			Protocol:    "TCP",
			Parallel:    2,
			Duration:    10,
			SentBps:     50_000_000,
			ReceivedBps: 48_000_000,
			Streams: []model.StreamResult{
				{ID: 1, SentBps: 25_000_000, ReceivedBps: 24_000_000, Sender: true},
				{ID: 2, SentBps: 25_000_000, ReceivedBps: 24_000_000, Sender: true},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "Per-Stream Results") {
		t.Error("missing per-stream section header")
	}
	// Stream entries appear as inline detail lines
	if !strings.Contains(content, "Stream 1:") {
		t.Error("missing Stream 1 entry")
	}
	if !strings.Contains(content, "Stream 2:") {
		t.Error("missing Stream 2 entry")
	}
}

func TestWriteTXT_SingleStream_NoPerStreamSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:  baseTXTTime,
			ServerAddr: "192.168.1.1",
			Port:       5201,
			Protocol:   "TCP",
			Parallel:   1,
			Duration:   10,
			SentBps:    100_000_000,
			Streams:    []model.StreamResult{{ID: 4, SentBps: 100_000_000, Sender: true}},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "Per-Stream Results") {
		t.Error("single-stream should not show per-stream section")
	}
}

func TestWriteTXT_AppendMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	r := model.TestResult{
		Timestamp:   baseTXTTime,
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    1,
		Duration:    10,
		SentBps:     100_000_000,
		ReceivedBps: 95_000_000,
	}

	// Write twice — should append
	if err := WriteTXT(path, []model.TestResult{r}); err != nil {
		t.Fatal(err)
	}
	if err := WriteTXT(path, []model.TestResult{r}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	// Each block: 1 opening divider + 1 before END + 1 after END = 3 per block × 2 blocks = 6
	// (section dashes use sectionDash, not divider, so only top-level dividers are counted)
	count := strings.Count(string(data), divider)
	// No latency: 1 (open) + 1 (before END) + 1 (after END) = 3 per block × 2 = 6
	if count != 6 {
		t.Errorf("expected 6 dividers (2 blocks × 3), got %d\n%s", count, string(data))
	}
}

func TestWriteTXT_MultipleResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{Timestamp: baseTXTTime, ServerAddr: "192.168.1.1", Port: 5201, Protocol: "TCP", Parallel: 1, Duration: 10},
		{Timestamp: baseTXTTime.Add(60e9), ServerAddr: "192.168.1.1", Port: 5201, Protocol: "TCP", Parallel: 1, Duration: 10},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	// 2 results × 3 dividers (open + END-before + END-after) = 6
	count := strings.Count(string(data), divider)
	if count != 6 {
		t.Errorf("expected 6 dividers for 2 results, got %d", count)
	}
}

func TestWriteTXT_ErrorResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:  baseTXTTime,
			ServerAddr: "10.0.0.1",
			Port:       5201,
			Protocol:   "TCP",
			Parallel:   1,
			Duration:   10,
			Error:      "connection refused",
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "Error: connection refused") {
		t.Error("missing error line")
	}
	// No interval section on error
	if strings.Contains(content, "Results") && strings.Contains(content, "Timestamp") {
		t.Error("error result should not have results interval table")
	}
	// Should still close with END OF MEASUREMENT
	if !strings.Contains(content, "END OF MEASUREMENT") {
		t.Error("error result should still end with END OF MEASUREMENT")
	}
}

func TestWriteTXT_WithLatency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:    baseTXTTime,
			ServerAddr:   "192.168.1.1",
			Port:         5201,
			Protocol:     "TCP",
			Parallel:     1,
			Duration:     10,
			SentBps:      100_000_000,
			PingBaseline: &model.PingResult{MinMs: 1.0, AvgMs: 2.0, MaxMs: 3.0, PacketsSent: 20},
			PingLoaded:   &model.PingResult{MinMs: 5.0, AvgMs: 10.0, MaxMs: 50.0, PacketsSent: 20},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "LATENCY ANALYSIS") {
		t.Error("missing LATENCY ANALYSIS header")
	}
	if !strings.Contains(content, "Baseline:") {
		t.Error("missing baseline line")
	}
	if !strings.Contains(content, "Under load:") {
		t.Error("missing under load line")
	}
	if !strings.Contains(content, "Increase:") {
		t.Error("missing Increase: line")
	}
	if !strings.Contains(content, "END OF MEASUREMENT") {
		t.Error("missing END OF MEASUREMENT")
	}
	// Old sections should not appear
	if strings.Contains(content, "--- Latency ---") {
		t.Error("old --- Latency --- section should be removed")
	}
	if strings.Contains(content, "Bufferbloat indicator:") {
		t.Error("Bufferbloat indicator removed with quality summary")
	}
}

func TestWriteTXT_WithTCPIntervals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:      baseTXTTime,
			ServerAddr:     "192.168.1.1",
			Port:           5201,
			Protocol:       "TCP",
			Parallel:       1,
			Duration:       2,
			SentBps:        100_000_000,
			ActualDuration: 2.0,
			Intervals: []model.IntervalResult{
				{TimeStart: 0, TimeEnd: 1, Bytes: 12_500_000, BandwidthBps: 100_000_000, Retransmits: 1},
				{TimeStart: 1, TimeEnd: 2, Bytes: 12_500_000, BandwidthBps: 100_000_000, Retransmits: 2},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	// New Results table
	if !strings.Contains(content, "Results") {
		t.Error("missing Results section")
	}
	if !strings.Contains(content, "100.00 Mbps") {
		t.Error("missing interval bandwidth")
	}
	if !strings.Contains(content, "Timestamp") {
		t.Error("interval table should have Timestamp column header")
	}
	// JSON-stream mode recorded in header
	if !strings.Contains(content, "JSON-stream") {
		t.Error("header should note JSON-stream mode")
	}
	// Actual duration in summary
	if !strings.Contains(content, "Actual duration: 2.0 s") {
		t.Error("summary should show actual duration")
	}
	// Old sections removed
	if strings.Contains(content, "--- Intervals ---") {
		t.Error("old --- Intervals --- section should be removed")
	}
	if strings.Contains(content, "--- Footer ---") {
		t.Error("old --- Footer --- section should be removed")
	}
	if strings.Contains(content, "Retr/s") {
		t.Error("Retr/s column should be removed")
	}
}

func TestWriteTXT_WithUDPIntervals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:   baseTXTTime,
			ServerAddr:  "192.168.1.1",
			Port:        5201,
			Protocol:    "UDP",
			Parallel:    1,
			Duration:    2,
			SentBps:     10_000_000,
			ReceivedBps: 9_500_000,
			Packets:     100,
			LostPackets: 3,
			LostPercent: 3.0,
			JitterMs:    0.5,
			Intervals: []model.IntervalResult{
				{TimeStart: 0, TimeEnd: 1, Bytes: 1_250_000, BandwidthBps: 10_000_000,
					Packets: 50, LostPackets: 1, LostPercent: 2.0, JitterMs: 0.4},
				{TimeStart: 1, TimeEnd: 2, Bytes: 1_250_000, BandwidthBps: 10_000_000,
					Packets: 50, LostPackets: 2, LostPercent: 4.0, JitterMs: 0.6},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "Results") {
		t.Error("missing Results section")
	}
	if !strings.Contains(content, "Lost") {
		t.Error("UDP interval table should have Lost column")
	}
	if !strings.Contains(content, "Jitter") {
		t.Error("UDP interval table should have Jitter column")
	}
	// UDP summary (mirrors UI FormatResult)
	if !strings.Contains(content, "Jitter:") {
		t.Error("UDP summary should show jitter line")
	}
	if !strings.Contains(content, "0.500 ms") {
		t.Error("UDP summary should show jitter value")
	}
	if !strings.Contains(content, "Packet Loss:") {
		t.Error("UDP summary should show Packet Loss line")
	}
	if !strings.Contains(content, "3/100") {
		t.Error("UDP summary should show lost/total packets")
	}
	// Quality summary removed
	if strings.Contains(content, "Packet Error Rate") {
		t.Error("quality summary (Packet Error Rate) should be removed")
	}
}

func TestWriteTXT_MeasurementID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:     baseTXTTime,
			ServerAddr:    "192.168.1.1",
			Port:          5201,
			Protocol:      "TCP",
			Parallel:      1,
			Duration:      10,
			MeasurementID: "20260218-143207-01",
			Mode:          "CLI",
			LocalHostname: "myhost",
			IperfVersion:  "3.17",
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "Measurement ID: 20260218-143207-01") {
		t.Error("missing measurement ID")
	}
	if !strings.Contains(content, "Mode:            CLI") {
		t.Error("missing mode")
	}
	if !strings.Contains(content, "Hostname:        myhost") {
		t.Error("missing hostname")
	}
	if !strings.Contains(content, "iperf3 version:  3.17") {
		t.Error("missing iperf version")
	}
}

func TestWriteTXT_EmptyResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	if err := WriteTXT(path, nil); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	// File should exist but be empty (no blocks written)
	if len(data) != 0 {
		t.Errorf("expected empty file for nil results, got %d bytes: %q", len(data), string(data))
	}
}
