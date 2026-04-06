package iperf

import (
	"math"
	"strings"
	"testing"

	"iperf-tool/internal/model"
)

// Real iperf2 output fixtures from live testing.

const sampleServerOutput = `------------------------------------------------------------
Server listening on UDP port 5201 to 5202
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.89.230.34 port 5201 connected with 100.80.223.29 port 52714
[  2] local 100.89.230.34 port 5202 connected with 100.80.223.29 port 52715
[ ID] Interval            Transfer     Bandwidth        Jitter   Lost/Total  Latency avg/min/max/stdev PPS
[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps
[  2]  0.00-1.00 sec  0.355 MBytes  2.98 Mbits/sec   5.422 ms  247/  500 (49%)  -0.411/ 0.388/ 1.187/ 0.799 ms  500 pps
[  1]  1.00-2.00 sec  0.437 MBytes  3.67 Mbits/sec  18.598 ms  197/  509 (39%)  -0.830/ 0.163/ 1.184/ 1.007 ms  509 pps
[  2]  1.00-2.00 sec  0.367 MBytes  3.08 Mbits/sec   7.630 ms  238/  500 (48%)  -0.509/ 0.332/ 1.177/ 0.854 ms  500 pps
[SUM-2]  0.00-10.03 sec  9.77 MBytes  8.17 Mbits/sec   6.405 ms 4942/11909 (41%)`

const sampleClientOutput = `------------------------------------------------------------
Client connecting to 100.89.230.34, UDP port 5201 to 5202
Sending 1470 byte datagrams, IPG target: 1127.27 us (kalman adjust)
UDP buffer size:  208 KByte (default)
------------------------------------------------------------
[  1] local 100.80.223.29 port 52714 connected with 100.89.230.34 port 5201
[  2] local 100.80.223.29 port 52715 connected with 100.89.230.34 port 5202
[ ID] Interval            Transfer     Bandwidth       Write/Err/Timeo
[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  2]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  1]  1.00-2.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[  2]  1.00-2.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
[SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec  11907/0/0`

const sampleClientWithValidServerReport = `[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec  625/0/0
[  1] Server Report:
[  1]  0.00-10.03 sec  5.14 MBytes  4.30 Mbits/sec  10.088 ms 2461/6129 (40%)`

const sampleFabricatedServerReport = `[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec
[  1] Server Report:
[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec   0.000 ms    0/6250 (0%)
WARNING: did not receive ack of last datagram after 10 tries.`

// sampleFabricatedNoWarning simulates the Tailscale case: ACK arrives but
// jitter is zeroed (fabricated), and no WARNING is printed.
const sampleFabricatedNoWarning = `[  1]  0.00-5.00 sec  6.25 MBytes  10.5 Mbits/sec
[  1] Sent 4461 datagrams
[  2]  0.00-5.00 sec  6.25 MBytes  10.5 Mbits/sec
[  2] Sent 4461 datagrams
[  2] Server Report:
[ ID] Interval       Transfer     Bandwidth        Jitter   Lost/Total Datagrams
[  2]  0.00-4.11 sec  5.50 MBytes  11.2 Mbits/sec   0.000 ms 538/4460 (0%)
[  1] Server Report:
[ ID] Interval       Transfer     Bandwidth        Jitter   Lost/Total Datagrams
[  1]  0.00-4.12 sec  5.50 MBytes  11.2 Mbits/sec   0.000 ms 536/4460 (0%)`

const sampleTCPOutput = `------------------------------------------------------------
Client connecting to 100.89.230.34, TCP port 5201
TCP window size: 0.06 MByte (default)
------------------------------------------------------------
[  1] local 100.80.223.29 port 52800 connected with 100.89.230.34 port 5201
[  2] local 100.80.223.29 port 52801 connected with 100.89.230.34 port 5201
[  1]  0.00-1.00 sec  1.12 MBytes  9.44 Mbits/sec
[  2]  0.00-1.00 sec  1.12 MBytes  9.40 Mbits/sec
[  1]  1.00-2.00 sec  1.10 MBytes  9.23 Mbits/sec
[  2]  1.00-2.00 sec  1.10 MBytes  9.23 Mbits/sec
[SUM]  0.00-10.02 sec  22.4 MBytes  18.7 Mbits/sec`

func TestParseServerOutput(t *testing.T) {
	result, err := ParseOutput(sampleServerOutput, true)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	// Should have intervals (2 per time bucket for 2 streams)
	if len(result.Intervals) == 0 {
		t.Error("expected intervals to be parsed")
	}

	// SUM line should set summary values
	if result.FwdReceivedBps == 0 {
		t.Error("FwdReceivedBps should be > 0 from SUM line")
	}
	if result.FwdLostPackets == 0 {
		t.Error("FwdLostPackets should be > 0")
	}
	if result.FwdPackets == 0 {
		t.Error("FwdPackets should be > 0")
	}
}

func TestParseServerEnhanced(t *testing.T) {
	// Parse a single enhanced server line
	line := "[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps"
	p, ok := parseSingleLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if p.streamID != 1 {
		t.Errorf("streamID = %d, want 1", p.streamID)
	}
	if math.Abs(p.jitterMs-10.088) > 0.001 {
		t.Errorf("jitterMs = %f, want 10.088", p.jitterMs)
	}
	if p.lostPackets != 266 {
		t.Errorf("lostPackets = %d, want 266", p.lostPackets)
	}
	if p.totalPackets != 511 {
		t.Errorf("totalPackets = %d, want 511", p.totalPackets)
	}
	if p.pps != 511 {
		t.Errorf("pps = %d, want 511", p.pps)
	}
	if math.Abs(p.latencyMinMs-0.231) > 0.001 {
		t.Errorf("latencyMinMs = %f, want 0.231", p.latencyMinMs)
	}
}

func TestParseClientOutput(t *testing.T) {
	result, err := ParseOutput(sampleClientOutput, false)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if result.SentBps == 0 {
		t.Error("SentBps should be > 0")
	}
	// SUM line: 14.0 Mbits/sec
	if math.Abs(result.SentBps-14.0e6) > 1e6 {
		t.Errorf("SentBps = %f, want ~14000000", result.SentBps)
	}

	if len(result.Intervals) == 0 {
		t.Error("expected intervals")
	}
}

func TestParseClientEnhanced(t *testing.T) {
	line := "[  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0"
	p, ok := parseSingleLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if p.writeCount != 625 {
		t.Errorf("writeCount = %d, want 625", p.writeCount)
	}
	if p.errCount != 0 {
		t.Errorf("errCount = %d, want 0", p.errCount)
	}
}

func TestParseTCPOutput(t *testing.T) {
	result, err := ParseOutput(sampleTCPOutput, false)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	// SUM: 18.7 Mbits/sec
	if math.Abs(result.SentBps-18.7e6) > 1e6 {
		t.Errorf("SentBps = %f, want ~18700000", result.SentBps)
	}

	// TCP has no jitter/loss
	if result.JitterMs != 0 {
		t.Errorf("JitterMs = %f, want 0 for TCP", result.JitterMs)
	}
}

func TestParseEmptyOutput(t *testing.T) {
	_, err := ParseOutput("", false)
	if err == nil {
		t.Error("expected error for empty output")
	}
}

func TestParseNoData(t *testing.T) {
	_, err := ParseOutput("Server listening on port 5201\nno data lines here\n", false)
	if err == nil {
		t.Error("expected error for output with no parseable data")
	}
}

func TestParseFabricatedServerReport(t *testing.T) {
	status := ValidateServerReport(sampleFabricatedServerReport)
	if status != ServerReportFabricated {
		t.Errorf("status = %d, want ServerReportFabricated (%d)", status, ServerReportFabricated)
	}
}

func TestValidateServerReport_FabricatedNoWarning(t *testing.T) {
	// Tailscale case: ACK arrives but jitter is 0.000 ms — no WARNING printed.
	status := ValidateServerReport(sampleFabricatedNoWarning)
	if status != ServerReportFabricated {
		t.Errorf("status = %d, want ServerReportFabricated (%d)", status, ServerReportFabricated)
	}
}

func TestValidateServerReport_Valid(t *testing.T) {
	status := ValidateServerReport(sampleClientWithValidServerReport)
	if status != ServerReportValid {
		t.Errorf("status = %d, want ServerReportValid (%d)", status, ServerReportValid)
	}
}

func TestValidateServerReport_Missing(t *testing.T) {
	status := ValidateServerReport("[  1]  0.00-10.00 sec  8.75 MBytes  7.34 Mbits/sec")
	if status != ServerReportMissing {
		t.Errorf("status = %d, want ServerReportMissing (%d)", status, ServerReportMissing)
	}
}

func TestParseIntervalLine(t *testing.T) {
	line := "[  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps"
	iv, err := ParseIntervalLine(line)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if iv == nil {
		t.Fatal("expected non-nil interval")
	}
	if iv.StreamID != 1 {
		t.Errorf("StreamID = %d, want 1", iv.StreamID)
	}
	if math.Abs(iv.BandwidthBps-2.88e6) > 1e4 {
		t.Errorf("BandwidthBps = %f, want ~2880000", iv.BandwidthBps)
	}
	if iv.LostPackets != 266 {
		t.Errorf("LostPackets = %d, want 266", iv.LostPackets)
	}
}

func TestParseIntervalLine_NonMatch(t *testing.T) {
	iv, err := ParseIntervalLine("Server listening on UDP port 5201")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iv != nil {
		t.Error("expected nil for non-matching line")
	}
}

func TestMergeBidirResults(t *testing.T) {
	fwdClient := &model.TestResult{
		SentBps:   14e6,
		BytesSent: 16_700_000,
	}
	fwdServer := &model.TestResult{
		ReceivedBps: 8.17e6,
		JitterMs:    6.405,
		LostPackets: 4942,
		Packets:     11909,
		LostPercent: 41.5,
	}
	revClient := &model.TestResult{
		SentBps:   13.9e6,
		BytesSent: 16_600_000,
	}
	revServer := &model.TestResult{
		ReceivedBps: 4.03e6,
		JitterMs:    6.275,
		LostPackets: 7566,
		Packets:     11877,
		LostPercent: 63.7,
		Intervals: []model.IntervalResult{
			{TimeStart: 0, TimeEnd: 1, BandwidthBps: 4e6},
		},
	}

	merged := MergeBidirResults(fwdClient, fwdServer, revClient, revServer)

	if merged.Direction != "Bidirectional" {
		t.Errorf("Direction = %q, want Bidirectional", merged.Direction)
	}
	if merged.SentBps != 14e6 {
		t.Errorf("SentBps = %f, want 14e6", merged.SentBps)
	}
	if merged.FwdReceivedBps != 8.17e6 {
		t.Errorf("FwdReceivedBps = %f, want 8.17e6", merged.FwdReceivedBps)
	}
	if merged.FwdJitterMs != 6.405 {
		t.Errorf("FwdJitterMs = %f, want 6.405", merged.FwdJitterMs)
	}
	if merged.FwdLostPackets != 4942 {
		t.Errorf("FwdLostPackets = %d, want 4942", merged.FwdLostPackets)
	}
	if merged.ReverseSentBps != 13.9e6 {
		t.Errorf("ReverseSentBps = %f, want 13.9e6", merged.ReverseSentBps)
	}
	if merged.ReverseReceivedBps != 4.03e6 {
		t.Errorf("ReverseReceivedBps = %f, want 4.03e6", merged.ReverseReceivedBps)
	}
	if merged.ReverseJitterMs != 6.275 {
		t.Errorf("ReverseJitterMs = %f, want 6.275", merged.ReverseJitterMs)
	}
	if len(merged.ReverseIntervals) != 1 {
		t.Errorf("ReverseIntervals = %d, want 1", len(merged.ReverseIntervals))
	}
}

func TestMergeUnidirResults(t *testing.T) {
	client := &model.TestResult{
		SentBps:   7.34e6,
		BytesSent: 8_750_000,
	}
	server := &model.TestResult{
		ReceivedBps: 4.30e6,
		JitterMs:    10.088,
		LostPackets: 2461,
		Packets:     6129,
		LostPercent: 40.2,
	}

	merged := MergeUnidirResults(client, server)

	if merged.SentBps != 7.34e6 {
		t.Errorf("SentBps = %f, want 7.34e6", merged.SentBps)
	}
	if merged.FwdReceivedBps != 4.30e6 {
		t.Errorf("FwdReceivedBps = %f, want 4.30e6", merged.FwdReceivedBps)
	}
	if merged.FwdJitterMs != 10.088 {
		t.Errorf("FwdJitterMs = %f, want 10.088", merged.FwdJitterMs)
	}
	if merged.FwdLostPackets != 2461 {
		t.Errorf("FwdLostPackets = %d, want 2461", merged.FwdLostPackets)
	}
}

func TestParseSumLineServer(t *testing.T) {
	line := "[SUM-2]  0.00-10.03 sec  9.77 MBytes  8.17 Mbits/sec   6.405 ms 4942/11909 (41%)"
	p := parseSumLine(line)
	if p == nil {
		t.Fatal("expected non-nil parsed sum line")
	}
	if !p.isSum {
		t.Error("expected isSum=true")
	}
	if p.lostPackets != 4942 {
		t.Errorf("lostPackets = %d, want 4942", p.lostPackets)
	}
	if p.totalPackets != 11909 {
		t.Errorf("totalPackets = %d, want 11909", p.totalPackets)
	}
}

func TestParseSumLineClient(t *testing.T) {
	line := "[SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec  11907/0/0"
	// This won't match reSumServer/reSumServerNoJitter, should match reSumClient
	p := parseSumLine(line)
	if p == nil {
		t.Fatal("expected non-nil parsed sum line")
	}
	if math.Abs(p.bandwidthBps-14.0e6) > 1e4 {
		t.Errorf("bandwidthBps = %f, want ~14e6", p.bandwidthBps)
	}
}

func TestParseClientInterval(t *testing.T) {
	line := "[  1]  0.00-1.00 sec  1.12 MBytes  9.44 Mbits/sec"
	p, ok := parseSingleLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if p.hasUDP {
		t.Error("TCP client line should not have UDP data")
	}
	if math.Abs(p.bandwidthBps-9.44e6) > 1e4 {
		t.Errorf("bandwidthBps = %f, want ~9440000", p.bandwidthBps)
	}
}

func TestParseOutput_ClientWithFabricatedReport(t *testing.T) {
	// When ACK warning is present, server report lines should be discarded
	result, err := ParseOutput(sampleFabricatedServerReport, false)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Should only have client-side data (no server jitter/loss)
	if result.SentBps == 0 {
		t.Error("SentBps should be > 0 from client data")
	}
}

func TestParseServerReportFromClient(t *testing.T) {
	result, err := parseServerReportFromClient(sampleClientWithValidServerReport)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Server report: 4.30 Mbits/sec, 10.088 ms jitter, 2461/6129 (40%)
	if result.JitterMs == 0 {
		t.Error("expected jitter from server report")
	}
}

func TestParseStandardServerInterval(t *testing.T) {
	// Standard (non-enhanced) server interval
	line := "[  1]  0.00-1.00 sec  0.197 MBytes  1.65 Mbits/sec   8.432 ms  359/  500 (72%)"
	p, ok := parseSingleLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if !p.hasUDP {
		t.Error("server line with jitter should have hasUDP=true")
	}
	if math.Abs(p.jitterMs-8.432) > 0.001 {
		t.Errorf("jitterMs = %f, want 8.432", p.jitterMs)
	}
	if p.lostPackets != 359 {
		t.Errorf("lostPackets = %d, want 359", p.lostPackets)
	}
}

func TestRoundTime(t *testing.T) {
	if roundTime(0.001) != 0 {
		t.Error("roundTime(0.001) should be 0")
	}
	if roundTime(1.006) != 1.01 {
		t.Errorf("roundTime(1.006) = %f, want 1.01", roundTime(1.006))
	}
	if roundTime(0.0) != 0 {
		t.Error("roundTime(0.0) should be 0")
	}
}

func TestInsertSorted(t *testing.T) {
	s := insertSorted(nil, 3.0)
	s = insertSorted(s, 1.0)
	s = insertSorted(s, 2.0)
	if len(s) != 3 || s[0] != 1.0 || s[1] != 2.0 || s[2] != 3.0 {
		t.Errorf("insertSorted result = %v, want [1 2 3]", s)
	}
}

const sampleDualtestTCPOutput = `------------------------------------------------------------
Client connecting to 192.168.1.1, TCP port 5201
TCP window size: 0.12 MByte (default)
------------------------------------------------------------
[  1] local 192.168.1.2 port 55442 connected with 192.168.1.1 port 5201
------------------------------------------------------------
Server listening on TCP port 5201
------------------------------------------------------------
[  2] local 192.168.1.2 port 5201 connected with 192.168.1.1 port 55443
[  1]  0.00-1.00 sec  11.2 MBytes  94.37 Mbits/sec
[  2]  0.00-1.00 sec  10.5 MBytes  88.08 Mbits/sec
[  1]  1.00-2.00 sec  11.5 MBytes  96.47 Mbits/sec
[  2]  1.00-2.00 sec  10.8 MBytes  90.58 Mbits/sec
[  1]  0.00-2.00 sec  22.7 MBytes  95.23 Mbits/sec
[  2]  0.00-2.00 sec  21.3 MBytes  89.33 Mbits/sec`

func TestParseDualtestOutput_TCP(t *testing.T) {
	result, err := ParseDualtestOutput(sampleDualtestTCPOutput)
	if err != nil {
		t.Fatalf("ParseDualtestOutput() error: %v", err)
	}

	if result.Direction != "Bidirectional" {
		t.Errorf("Direction = %q, want Bidirectional", result.Direction)
	}

	// Stream 1 is forward (client), should produce 2 per-second intervals
	if len(result.Intervals) != 2 {
		t.Errorf("expected 2 forward intervals, got %d", len(result.Intervals))
	}

	// Stream 2 is reverse (server), should produce 2 per-second intervals
	if len(result.ReverseIntervals) != 2 {
		t.Errorf("expected 2 reverse intervals, got %d", len(result.ReverseIntervals))
	}

	// Forward summary should reflect stream 1 summary line (95.23 Mbits/sec)
	if result.SentBps == 0 {
		t.Error("SentBps should be > 0 from forward stream")
	}

	// Reverse summary should reflect stream 2 (89.33 Mbits/sec)
	if result.ReverseReceivedBps == 0 {
		t.Error("ReverseReceivedBps should be > 0 from reverse stream")
	}
}

func TestParseDualtestOutput_EmptyInput(t *testing.T) {
	_, err := ParseDualtestOutput("")
	if err == nil {
		t.Error("expected error for empty dualtest output")
	}
}

func TestParseDualtestOutput_ForwardReverseInterleaved(t *testing.T) {
	// Verify that interleaved stream IDs are correctly separated
	result, err := ParseDualtestOutput(sampleDualtestTCPOutput)
	if err != nil {
		t.Fatalf("ParseDualtestOutput() error: %v", err)
	}

	// Forward intervals from stream 1: 0.00-1.00 and 1.00-2.00
	if len(result.Intervals) >= 1 {
		if result.Intervals[0].TimeStart != 0.0 {
			t.Errorf("first forward interval TimeStart = %f, want 0.0", result.Intervals[0].TimeStart)
		}
	}
	if len(result.Intervals) >= 2 {
		if result.Intervals[1].TimeStart != 1.0 {
			t.Errorf("second forward interval TimeStart = %f, want 1.0", result.Intervals[1].TimeStart)
		}
	}

	// Reverse intervals from stream 2: 0.00-1.00 and 1.00-2.00
	if len(result.ReverseIntervals) >= 1 {
		if result.ReverseIntervals[0].TimeStart != 0.0 {
			t.Errorf("first reverse interval TimeStart = %f, want 0.0", result.ReverseIntervals[0].TimeStart)
		}
	}
	if len(result.ReverseIntervals) >= 2 {
		if result.ReverseIntervals[1].TimeStart != 1.0 {
			t.Errorf("second reverse interval TimeStart = %f, want 1.0", result.ReverseIntervals[1].TimeStart)
		}
	}
}

func TestParseOutput_ReverseServerOutput(t *testing.T) {
	// Local server output for reverse direction
	input := strings.Join([]string{
		"[  1]  0.00-1.00 sec  0.197 MBytes  1.65 Mbits/sec   8.432 ms  359/  500 (72%)",
		"[  2]  0.00-1.00 sec  0.225 MBytes  1.89 Mbits/sec   4.117 ms  339/  500 (68%)",
		"[  1]  1.00-2.00 sec  0.200 MBytes  1.68 Mbits/sec   7.000 ms  350/  500 (70%)",
		"[  2]  1.00-2.00 sec  0.230 MBytes  1.93 Mbits/sec   3.800 ms  335/  500 (67%)",
	}, "\n")

	result, err := ParseOutput(input, true)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.Intervals) != 2 {
		t.Errorf("expected 2 intervals, got %d", len(result.Intervals))
	}
	// Each interval should aggregate 2 streams
	if result.Intervals[0].LostPackets == 0 {
		t.Error("expected lost packets in aggregated interval")
	}
}
