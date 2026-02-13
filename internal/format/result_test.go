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
