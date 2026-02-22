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
	header := FormatIntervalHeader(false)
	if strings.Contains(header, "Interval") {
		t.Error("header should not contain 'Interval' range column")
	}
	if !strings.Contains(header, "Bandwidth") {
		t.Error("header should contain 'Bandwidth'")
	}

	udpHeader := FormatIntervalHeader(true)
	if !strings.Contains(udpHeader, "Mbps") {
		t.Error("UDP header should contain 'Mbps'")
	}
	if strings.Contains(udpHeader, "Retransmits") {
		t.Error("UDP header should not contain 'Retransmits'")
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

	out := FormatInterval(interval, false)

	if strings.Contains(out, "0.0") && strings.Contains(out, "sec") {
		t.Error("should not contain interval range [X.X-Y.Y sec]")
	}
	if !strings.Contains(out, "940.00 Mbps") {
		t.Errorf("should contain bandwidth, got: %s", out)
	}
	if !strings.Contains(out, "3 retransmits") {
		t.Error("should contain retransmits")
	}
}

func TestFormatIntervalUDP(t *testing.T) {
	interval := &model.IntervalResult{
		TimeStart:    0,
		TimeEnd:      1,
		Bytes:        125000,
		BandwidthBps: 1_000_000,
		Packets:      50,
		LostPackets:  2,
		JitterMs:     0.123,
	}

	out := FormatInterval(interval, true)

	if !strings.Contains(out, "1.00 Mbps") {
		t.Errorf("should contain bandwidth, got: %s", out)
	}
	if !strings.Contains(out, "50 pkts") {
		t.Error("should contain packet count")
	}
	if strings.Contains(out, "lost") {
		t.Error("UDP interval should not show lost count")
	}
	if strings.Contains(out, "0.123 ms") {
		t.Error("UDP interval should not show jitter")
	}
	if strings.Contains(out, "retransmits") {
		t.Error("UDP interval should not mention retransmits")
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

func TestFormatResultWithPing(t *testing.T) {
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
		PingBaseline: &model.PingResult{MinMs: 1.23, AvgMs: 2.34, MaxMs: 3.45},
		PingLoaded:   &model.PingResult{MinMs: 5.67, AvgMs: 12.34, MaxMs: 45.67},
	}

	out := FormatResult(r)

	if !strings.Contains(out, "--- Latency ---") {
		t.Error("missing Latency section")
	}
	if !strings.Contains(out, "Baseline:") {
		t.Error("missing Baseline line")
	}
	if !strings.Contains(out, "2.34") {
		t.Error("missing baseline avg")
	}
	if !strings.Contains(out, "Under load:") {
		t.Error("missing Under load line")
	}
	if !strings.Contains(out, "12.34") {
		t.Error("missing loaded avg")
	}
}

func TestFormatResultDirection(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "TCP",
		Parallel:    1,
		Duration:    10,
		Direction:   "Reverse",
		SentBps:     940_000_000,
		ReceivedBps: 936_000_000,
		Retransmits: 5,
		Streams:     []model.StreamResult{{ID: 1, SentBps: 940_000_000, ReceivedBps: 936_000_000}},
	}

	out := FormatResult(r)
	if !strings.Contains(out, "Direction:       Reverse") {
		t.Error("missing direction line")
	}
}

func TestFormatResultDirectionNormal(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "TCP",
		Duration:    10,
		SentBps:     940_000_000,
		ReceivedBps: 936_000_000,
		Streams:     []model.StreamResult{{ID: 1, SentBps: 940_000_000, ReceivedBps: 936_000_000}},
	}

	out := FormatResult(r)
	if strings.Contains(out, "Direction:") {
		t.Error("should not show Direction for normal mode")
	}
}

func TestFormatResultCongestionAndBandwidth(t *testing.T) {
	r := &model.TestResult{
		Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:  "192.168.1.1",
		Port:        5201,
		Protocol:    "TCP",
		Duration:    10,
		Congestion:  "bbr",
		Bandwidth:   "100M",
		SentBps:     940_000_000,
		ReceivedBps: 936_000_000,
		Streams:     []model.StreamResult{{ID: 1, SentBps: 940_000_000, ReceivedBps: 936_000_000}},
	}

	out := FormatResult(r)
	if !strings.Contains(out, "Congestion:      bbr") {
		t.Error("missing congestion line")
	}
	if !strings.Contains(out, "Bandwidth Target: 100M") {
		t.Error("missing bandwidth target line")
	}
}

func TestFormatResultBytesTransferred(t *testing.T) {
	r := &model.TestResult{
		Timestamp:     time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:    "192.168.1.1",
		Port:          5201,
		Protocol:      "TCP",
		Duration:      10,
		SentBps:       940_000_000,
		ReceivedBps:   936_000_000,
		BytesSent:     1175000000,
		BytesReceived: 1170000000,
		Streams:       []model.StreamResult{{ID: 1, SentBps: 940_000_000, ReceivedBps: 936_000_000}},
	}

	out := FormatResult(r)
	if !strings.Contains(out, "Transferred:") {
		t.Error("missing Transferred line")
	}
	if !strings.Contains(out, "1175.00 MB sent") {
		t.Error("missing sent MB value")
	}
	if !strings.Contains(out, "1170.00 MB received") {
		t.Error("missing received MB value")
	}
}

func TestFormatResultBidir(t *testing.T) {
	r := &model.TestResult{
		Timestamp:            time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:           "192.168.1.1",
		Port:                 5201,
		Protocol:             "TCP",
		Parallel:             4,
		Duration:             10,
		Direction:            "Bidirectional",
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
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 200_000_000, ReceivedBps: 198_000_000, Retransmits: 1, Sender: true},
			{ID: 2, SentBps: 200_000_000, ReceivedBps: 198_000_000, Retransmits: 1, Sender: true},
			{ID: 3, SentBps: 240_000_000, ReceivedBps: 236_000_000, Retransmits: 3, Sender: false},
			{ID: 4, SentBps: 240_000_000, ReceivedBps: 236_000_000, Retransmits: 2, Sender: false},
		},
	}

	out := FormatResult(r)

	// Per-stream labels
	if !strings.Contains(out, "Stream 1 [Fwd]:") {
		t.Error("missing TX label for stream 1")
	}
	if !strings.Contains(out, "Stream 3 [Rev]:") {
		t.Error("missing RX label for stream 3")
	}

	// Summary
	if !strings.Contains(out, "Send:") {
		t.Error("missing Send summary")
	}
	if !strings.Contains(out, "Receive:") {
		t.Error("missing Receive summary")
	}
	if !strings.Contains(out, "400.00 Mbps") {
		t.Error("missing forward sent Mbps")
	}
	// ReverseActualMbps prefers ReverseReceivedBps (472) over ReverseSentBps (480)
	if !strings.Contains(out, "472.00 Mbps") {
		t.Error("missing reverse actual Mbps")
	}
	if !strings.Contains(out, "(retransmits: 2)") {
		t.Error("missing forward retransmits")
	}
	if !strings.Contains(out, "(retransmits: 5)") {
		t.Error("missing reverse retransmits")
	}
	// BytesSent=500MB, BytesReceived=495MB → C→S full line
	if !strings.Contains(out, "C→S transferred: 500.00 MB sent / 495.00 MB received") {
		t.Errorf("missing C→S transferred line, got:\n%s", out)
	}
	// ReverseBytesSent=600MB, ReverseBytesReceived=590MB → S→C full line
	if !strings.Contains(out, "S→C transferred: 600.00 MB sent / 590.00 MB received") {
		t.Errorf("missing S→C transferred line, got:\n%s", out)
	}
	if !strings.Contains(out, "Direction:       Bidirectional") {
		t.Error("missing direction line")
	}
	// Should NOT have WARNING since forward streams match
	if strings.Contains(out, "WARNING") {
		t.Error("should not have warning for bidir with matching forward streams")
	}
}

func TestFormatResultBidirStreamModeFallback(t *testing.T) {
	// In --json-stream bidir mode, sum_sent_bidir_reverse may be absent.
	// The format should fall back to ReceivedBps for the Receive summary.
	// Simulates SIGTERM in --json-stream bidir mode: sum_sent_bidir_reverse.bytes=0
	// but sum_received_bidir_reverse.bytes has the real count.
	r := &model.TestResult{
		Timestamp:            time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:           "192.168.1.1",
		Port:                 5201,
		Protocol:             "TCP",
		Parallel:             4,
		Duration:             10,
		Direction:            "Bidirectional",
		SentBps:              400_000_000,
		ReceivedBps:          0,
		Retransmits:          2,
		BytesSent:            500_000_000,
		BytesReceived:        0,
		ReverseSentBps:       480_000_000,
		ReverseBytesSent:     0,                // zeroed by iperf3 on SIGTERM
		ReverseBytesReceived: 600_000_000,      // receiver side has the real count
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 200_000_000, Sender: true},
			{ID: 2, SentBps: 200_000_000, Sender: true},
			{ID: 3, Sender: false},
			{ID: 4, Sender: false},
		},
	}

	out := FormatResult(r)

	// Receive line should show ReverseSentBps
	if !strings.Contains(out, "Receive:         480.00 Mbps") {
		t.Errorf("expected Receive to show ReverseSentBps, got: %s", out)
	}
	// ReverseBytesSent=0 → S→C line shows only received side (fallback to ReverseBytesReceived)
	if !strings.Contains(out, "S→C transferred: 600.00 MB received") {
		t.Errorf("expected S→C fallback to ReverseBytesReceived, got: %s", out)
	}
}

func TestFormatResultBidirUDP(t *testing.T) {
	r := &model.TestResult{
		Timestamp:      time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
		ServerAddr:     "192.168.1.1",
		Port:           5201,
		Protocol:       "UDP",
		Parallel:       4,
		Duration:       10,
		Direction:      "Bidirectional",
		SentBps:        4_000_000,
		ReverseSentBps: 3_800_000,
		Packets:        200,
		LostPackets:    4,
		LostPercent:    2.0,
		JitterMs:       0.050,
		Streams: []model.StreamResult{
			{ID: 1, SentBps: 1_000_000, Packets: 50},
			{ID: 2, SentBps: 1_000_000, Packets: 50},
			{ID: 3, SentBps: 1_000_000, Packets: 50},
			{ID: 4, SentBps: 1_000_000, Packets: 50},
		},
	}

	out := FormatResult(r)

	// Summary should not mention retransmits for UDP bidir
	if strings.Contains(out, "retransmits") {
		t.Error("UDP bidir summary should not mention retransmits")
	}
	// Per-stream section: all streams shown (UDP bidir uses [Fwd]/[Rev] prefix)
	if !strings.Contains(out, "Stream 1 [") {
		t.Error("missing Stream 1")
	}
	if !strings.Contains(out, "Stream 4 [") {
		t.Error("missing Stream 4")
	}
	// Should show Client Send/Server Send bandwidth
	if !strings.Contains(out, "Client Send:") {
		t.Error("missing Client Send line")
	}
	if !strings.Contains(out, "Server Send:") {
		t.Error("missing Server Send line")
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
