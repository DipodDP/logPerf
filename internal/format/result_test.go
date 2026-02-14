package format

import (
	"strings"
	"testing"
	"time"

	"iperf-tool/internal/model"
)

func TestFormatResultSingleStream(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    1,
		Duration:    10,
		SentBps:     940_000_000,
		ReceivedBps: 936_000_000,
		Retransmits: 5,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 940_000_000, ReceivedBps: 936_000_000, Retransmits: 5},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "=== Test Results ===") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "192.168.1.1:5201") {
		t.Error("missing server address")
	}
	// Single stream should NOT show per-stream section
	if strings.Contains(out, "Per-Stream Results") {
		t.Error("single stream should not show per-stream section")
	}
	if !strings.Contains(out, "940.00 Mbps") {
		t.Error("missing sent Mbps")
	}
	if !strings.Contains(out, "Retransmits:     5") {
		t.Error("missing retransmits")
	}
}

func TestFormatResultMultiStream(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "10.0.0.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    2,
		Duration:    10,
		SentBps:     200_000_000,
		ReceivedBps: 190_000_000,
		Retransmits: 0,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 100_000_000, ReceivedBps: 95_000_000, Retransmits: 0},
			{ID: 2, SentBps: 100_000_000, ReceivedBps: 95_000_000, Retransmits: 0},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "2 streams") {
		t.Error("missing parallel streams info")
	}
	if !strings.Contains(out, "Per-Stream Results") {
		t.Error("multi-stream should show per-stream section")
	}
	if !strings.Contains(out, "Stream 1:") {
		t.Error("missing Stream 1")
	}
	if !strings.Contains(out, "Stream 2:") {
		t.Error("missing Stream 2")
	}
	if strings.Contains(out, "WARNING") {
		t.Error("should not have warning when totals match")
	}
}

func TestFormatResultMismatchWarning(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "10.0.0.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    2,
		Duration:    10,
		SentBps:     200_000_000,
		ReceivedBps: 190_000_000,
		Retransmits: 0,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 50_000_000, ReceivedBps: 45_000_000, Retransmits: 0},
			{ID: 2, SentBps: 50_000_000, ReceivedBps: 45_000_000, Retransmits: 0},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "WARNING") {
		t.Error("expected warning for mismatched stream totals")
	}
}

func TestFormatIntervalHeader(t *testing.T) {
	header := FormatIntervalHeader()
	if !strings.Contains(header, "Interval") {
		t.Error("header should contain 'Interval'")
	}
	if !strings.Contains(header, "Bandwidth") {
		t.Error("header should contain 'Bandwidth'")
	}
}

func TestFormatInterval(t *testing.T) {
	interval := &model.IntervalResult{
		TimeStart:    0,
		TimeEnd:      1,
		Bytes:        117500000,
		BandwidthBps: 940_000_000,
		Retransmits:  3,
		Omitted:      false,
	}

	out := FormatInterval(interval)

	if !strings.Contains(out, "0.0") {
		t.Error("should contain start time")
	}
	if !strings.Contains(out, "1.0 sec") {
		t.Error("should contain end time")
	}
	if !strings.Contains(out, "940.00 Mbps") {
		t.Errorf("should contain bandwidth, got: %s", out)
	}
	if !strings.Contains(out, "3 retransmits") {
		t.Error("should contain retransmits")
	}
}

func TestFormatResultUDP(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "UDP",
		Parallel:    1,
		Duration:    3,
		SentBps:     1_048_576,
		JitterMs:    0.025,
		LostPackets: 3,
		LostPercent: 6.25,
		Packets:     48,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 1_048_576, JitterMs: 0.025, LostPackets: 3, LostPercent: 6.25, Packets: 48},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "UDP") {
		t.Error("missing protocol UDP")
	}
	if !strings.Contains(out, "Jitter:") {
		t.Error("missing Jitter line")
	}
	if !strings.Contains(out, "0.025 ms") {
		t.Error("missing jitter value")
	}
	if !strings.Contains(out, "Packet Loss:") {
		t.Error("missing Packet Loss line")
	}
	if !strings.Contains(out, "3/48") {
		t.Error("missing lost/total packets")
	}
	if !strings.Contains(out, "6.25%") {
		t.Error("missing loss percentage")
	}
	if strings.Contains(out, "Retransmits") {
		t.Error("UDP should not show Retransmits")
	}
	if strings.Contains(out, "Received:") {
		t.Error("UDP should not show Received")
	}
	if strings.Contains(out, "WARNING") {
		t.Error("should not have warning when UDP stream totals match")
	}
}

func TestFormatResultUDPMultiStream(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "10.0.0.1",
		Port:        5201,
		Protocol:    "UDP",
		Parallel:    2,
		Duration:    3,
		SentBps:     2_000_000,
		JitterMs:    0.050,
		LostPackets: 5,
		LostPercent: 5.0,
		Packets:     100,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 1_000_000, JitterMs: 0.040, LostPackets: 2, LostPercent: 4.0, Packets: 50},
			{ID: 2, SentBps: 1_000_000, JitterMs: 0.060, LostPackets: 3, LostPercent: 6.0, Packets: 50},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "Per-Stream Results") {
		t.Error("multi-stream UDP should show per-stream section")
	}
	if !strings.Contains(out, "Stream 1:") {
		t.Error("missing Stream 1")
	}
	if !strings.Contains(out, "Jitter: 0.040 ms") {
		t.Error("missing stream 1 jitter")
	}
	if !strings.Contains(out, "Lost: 2/50") {
		t.Error("missing stream 1 loss")
	}
}

func TestFormatResultSenderOnly(t *testing.T) {
	// In --json-stream mode, receiver data is not available (sender-side only)
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "10.0.0.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    4,
		Duration:    10,
		SentBps:     44_590_000,
		ReceivedBps: 0,
		Retransmits: 0,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 11_850_000},
			{ID: 2, SentBps: 8_480_000},
			{ID: 3, SentBps: 12_130_000},
			{ID: 4, SentBps: 12_130_000},
		},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "Bandwidth:") {
		t.Error("sender-only should show 'Bandwidth:' label")
	}
	if strings.Contains(out, "Received:") {
		t.Error("sender-only should not show 'Received:'")
	}
	if strings.Contains(out, "Sent:") {
		t.Error("sender-only should not show 'Sent:' (use 'Bandwidth:' instead)")
	}
	// Per-stream should show just bandwidth, not Sent/Received
	if strings.Contains(out, "Sent:") {
		t.Error("per-stream sender-only should not show 'Sent:' label")
	}
	if strings.Contains(out, "WARNING") {
		t.Error("should not have warning when receiver data is absent")
	}
}

func TestFormatResultError(t *testing.T) {
	r := &model.TestResult{
		Timestamp:  time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr: "10.0.0.1",
		Port:       5201,
		Protocol:   "TCP",
		Duration:   10,
		Error:      "connection refused",
	}

	out := FormatResult(r)

	if !strings.Contains(out, "Error: connection refused") {
		t.Error("missing error message")
	}
	if strings.Contains(out, "Summary") {
		t.Error("should not show summary on error")
	}
}
