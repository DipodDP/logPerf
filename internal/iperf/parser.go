package iperf

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"iperf-tool/internal/model"
)

// ServerReportStatus indicates the validity of a client-side Server Report.
type ServerReportStatus int

const (
	// ServerReportValid means the summary is present with no WARNING — data is trustworthy.
	ServerReportValid ServerReportStatus = iota
	// ServerReportFabricated means WARNING present — client fabricated the Server Report.
	ServerReportFabricated
	// ServerReportMissing means no Server Report summary line was found.
	ServerReportMissing
	// ServerReportUnknown means no WARNING but likely interrupted (unreliable).
	ServerReportUnknown
)

var (
	// Standard server-side interval: jitter and lost/total columns
	// [  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/511 (52%)
	reServerInterval = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+([\d.]+)\s+ms\s+(\d+)/\s*(\d+)\s+\(([\d.]+)%\)`)

	// Enhanced server-side interval (-e mode): adds latency, PPS, etc.
	// [  1]  0.00-1.00 sec  0.343 MBytes  2.88 Mbits/sec  10.088 ms  266/  511 (52%)  -0.719/ 0.231/ 1.181/ 0.950 ms  511 pps
	reServerEnhanced = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+([\d.]+)\s+ms\s+(\d+)/\s*(\d+)\s+\(([\d.]+)%\)\s+(-?[\d.]+)/\s*(-?[\d.]+)/\s*(-?[\d.]+)/\s*(-?[\d.]+)\s+ms\s+(\d+)\s+pps`)

	// Client-side interval: no jitter/loss columns
	// [  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec
	reClientInterval = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s*$`)

	// Enhanced TCP server-side interval (-e mode): Reads=Dist histogram
	// [  1]  0.00-1.00 sec  2.51 MBytes  21.0 Mbits/sec  585=576:3:0:0:0:0:1:5
	reServerEnhancedTCP = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+\d+=`)

	// Enhanced client-side interval (-e mode): adds Write/Err/Timeo
	// [  1]  0.00-1.00 sec  0.875 MBytes  7.34 Mbits/sec  625/0/0
	reClientEnhanced = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/(\d+)/(\d+)`)

	// Enhanced TCP client (no -V): Write/Err only (e.g. Windows iperf2 client)
	// [  1] 0.00-1.00 sec  8.25 MBytes  69.2 Mbits/sec  67/0
	reClientWriteErr = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/(\d+)\s*$`)

	// Enhanced TCP client with -V flag: Write/Err  Rtry  Cwnd/RTT  NetPwr
	// [  1] 0.00-3.35 sec  28.5 MBytes  71.4 Mbits/sec  229/0          0       NA/98000(49)us    91.10
	reClientTCPVerbose = regexp.MustCompile(
		`^\[\s*(\d+)\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/(\d+)\s+(\d+)`)

	// SUM line (server-side, with loss):
	// [SUM-2]  0.00-10.00 sec  9.77 MBytes  5.59 Mbits/sec  4942/11909 (41%)
	reSumServer = regexp.MustCompile(
		`^\[SUM-?\d*\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+([\d.]+)\s+ms\s+(\d+)/\s*(\d+)\s+\(([\d.]+)%\)`)

	// SUM line (server-side, without jitter — for cases where jitter isn't in the SUM line):
	// [SUM-2]  0.00-10.03 sec  9.77 MBytes  8.17 Mbits/sec   6.405 ms 4942/11909 (41%)
	reSumServerNoJitter = regexp.MustCompile(
		`^\[SUM-?\d*\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec\s+(\d+)/\s*(\d+)\s+\(([\d.]+)%\)`)

	// SUM line (client-side, no loss):
	// [SUM]  0.00-10.00 sec  16.7 MBytes  14.0 Mbits/sec
	reSumClient = regexp.MustCompile(
		`^\[SUM(?:-\d+)?\]\s+([\d.]+)-([\d.]+)\s+sec\s+([\d.]+)\s+MBytes\s+([\d.]+)\s+Mbits/sec`)

	// Server Report marker — client received server-side stats via ACK
	reServerReport = regexp.MustCompile(`(?i)server\s+report`)

	// WARNING: ack of last datagram failed — server report is fabricated
	reACKWarning = regexp.MustCompile(`WARNING.*ack.*last.*datagram`)
)

// parsedLine holds the parsed fields from a single iperf2 output line.
type parsedLine struct {
	streamID     int
	timeStart    float64
	timeEnd      float64
	bytes        int64   // converted from MBytes
	bandwidthBps float64 // Mbits/sec * 1e6
	jitterMs     float64 // 0 if client-side
	lostPackets  int
	totalPackets int
	lostPct      float64
	isSum        bool
	hasUDP       bool // true = server-side (jitter/loss present)
	// Enhanced fields
	latencyAvgMs float64
	latencyMinMs float64
	latencyMaxMs float64
	latencyStdev float64
	pps          int
	writeCount   int // client-side -e
	errCount     int
	timeoCount   int
}

// parseSingleLine attempts to parse a single iperf2 output line.
// Returns nil, false if the line doesn't match any known format.
func parseSingleLine(line string) (*parsedLine, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}

	// 1. Try enhanced server-side (most specific)
	if m := reServerEnhanced.FindStringSubmatch(line); m != nil {
		return parseServerEnhancedMatch(m), true
	}

	// 2. Try standard server-side (jitter + loss)
	if m := reServerInterval.FindStringSubmatch(line); m != nil {
		return parseServerIntervalMatch(m), true
	}

	// 2b. Try enhanced TCP server-side (Reads=Dist histogram, no jitter)
	if m := reServerEnhancedTCP.FindStringSubmatch(line); m != nil {
		return parseServerEnhancedTCPMatch(m), true
	}

	// 3. Try enhanced client-side (Write/Err/Timeo)
	if m := reClientEnhanced.FindStringSubmatch(line); m != nil {
		return parseClientEnhancedMatch(m), true
	}

	// 3b. Try TCP verbose (-V) client-side (Write/Err  Rtry)
	if m := reClientTCPVerbose.FindStringSubmatch(line); m != nil {
		return parseClientTCPVerboseMatch(m), true
	}

	// 3c. Try plain Write/Err client-side (no Rtry, no Timeo)
	if m := reClientWriteErr.FindStringSubmatch(line); m != nil {
		p := &parsedLine{}
		p.streamID, _ = strconv.Atoi(m[1])
		p.timeStart, _ = strconv.ParseFloat(m[2], 64)
		p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
		mb, _ := strconv.ParseFloat(m[4], 64)
		p.bytes = int64(mb * 1_000_000)
		bw, _ := strconv.ParseFloat(m[5], 64)
		p.bandwidthBps = bw * 1_000_000
		p.writeCount, _ = strconv.Atoi(m[6])
		p.errCount, _ = strconv.Atoi(m[7])
		return p, true
	}

	// 4. Try standard client-side (bandwidth only)
	if m := reClientInterval.FindStringSubmatch(line); m != nil {
		return parseClientIntervalMatch(m), true
	}

	// 5. Try SUM lines (parallel-stream aggregates) — used by streaming display
	if strings.HasPrefix(line, "[SUM") {
		if p := parseSumLine(line); p != nil {
			return p, true
		}
	}

	return nil, false
}

func parseServerEnhancedMatch(m []string) *parsedLine {
	p := &parsedLine{hasUDP: true}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	p.jitterMs, _ = strconv.ParseFloat(m[6], 64)
	p.lostPackets, _ = strconv.Atoi(m[7])
	p.totalPackets, _ = strconv.Atoi(m[8])
	p.lostPct, _ = strconv.ParseFloat(m[9], 64)
	p.latencyAvgMs, _ = strconv.ParseFloat(m[10], 64)
	p.latencyMinMs, _ = strconv.ParseFloat(m[11], 64)
	p.latencyMaxMs, _ = strconv.ParseFloat(m[12], 64)
	p.latencyStdev, _ = strconv.ParseFloat(m[13], 64)
	p.pps, _ = strconv.Atoi(m[14])
	return p
}

func parseServerIntervalMatch(m []string) *parsedLine {
	p := &parsedLine{hasUDP: true}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	p.jitterMs, _ = strconv.ParseFloat(m[6], 64)
	p.lostPackets, _ = strconv.Atoi(m[7])
	p.totalPackets, _ = strconv.Atoi(m[8])
	p.lostPct, _ = strconv.ParseFloat(m[9], 64)
	return p
}

func parseServerEnhancedTCPMatch(m []string) *parsedLine {
	p := &parsedLine{}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	return p
}

func parseClientEnhancedMatch(m []string) *parsedLine {
	p := &parsedLine{}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	p.writeCount, _ = strconv.Atoi(m[6])
	p.errCount, _ = strconv.Atoi(m[7])
	p.timeoCount, _ = strconv.Atoi(m[8])
	return p
}

func parseClientIntervalMatch(m []string) *parsedLine {
	p := &parsedLine{}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	return p
}

func parseClientTCPVerboseMatch(m []string) *parsedLine {
	p := &parsedLine{}
	p.streamID, _ = strconv.Atoi(m[1])
	p.timeStart, _ = strconv.ParseFloat(m[2], 64)
	p.timeEnd, _ = strconv.ParseFloat(m[3], 64)
	mb, _ := strconv.ParseFloat(m[4], 64)
	p.bytes = int64(mb * 1_000_000)
	bw, _ := strconv.ParseFloat(m[5], 64)
	p.bandwidthBps = bw * 1_000_000
	p.writeCount, _ = strconv.Atoi(m[6])
	p.errCount, _ = strconv.Atoi(m[7])
	return p
}

// ParseOutput parses the full text output of an iperf2 test run into a TestResult.
// isServerSide should be true when parsing server output (has jitter/loss data),
// false when parsing client output.
func ParseOutput(text string, isServerSide bool) (*model.TestResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty iperf2 output")
	}

	lines := strings.Split(text, "\n")

	// Detect Server Report section and ACK warning in client output
	serverReportIdx := -1
	ackWarning := false
	for i, line := range lines {
		if reServerReport.MatchString(line) && serverReportIdx == -1 {
			serverReportIdx = i
		}
		if reACKWarning.MatchString(line) {
			ackWarning = true
		}
	}

	// Parse all interval lines (excluding the Server Report section if fabricated)
	type bucket struct {
		lines []*parsedLine
	}
	intervals := map[float64]*bucket{}
	var allParsed []*parsedLine
	var sumLine *parsedLine

	// Determine which lines to parse as "primary" data
	// If we're parsing client output with a valid Server Report, split into
	// two zones: pre-report (client intervals) and post-report (server data).
	inServerReport := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track when we enter the server report section
		if !isServerSide && serverReportIdx >= 0 && i >= serverReportIdx {
			if reServerReport.MatchString(trimmed) {
				inServerReport = true
				continue
			}
		}

		// Skip Server Report data if it's fabricated
		if inServerReport && ackWarning {
			continue
		}

		// Try SUM lines first
		if strings.HasPrefix(trimmed, "[SUM") {
			if sl := parseSumLine(trimmed); sl != nil {
				sumLine = sl
				continue
			}
		}

		p, ok := parseSingleLine(trimmed)
		if !ok {
			continue
		}

		// If we're in the server report section and it's valid, mark as server data
		if inServerReport && !ackWarning {
			p.hasUDP = true // server report lines have jitter/loss
		}

		allParsed = append(allParsed, p)

		// Group by time start for interval aggregation
		key := roundTime(p.timeStart)
		b, exists := intervals[key]
		if !exists {
			b = &bucket{}
			intervals[key] = b
		}
		b.lines = append(b.lines, p)
	}

	if len(allParsed) == 0 && sumLine == nil {
		return nil, fmt.Errorf("no parseable iperf2 data found in output")
	}

	result := &model.TestResult{
		Timestamp: time.Now(),
	}

	// Build interval results by aggregating per-stream data at each time bucket
	var sortedKeys []float64
	for k := range intervals {
		sortedKeys = insertSorted(sortedKeys, k)
	}

	for _, key := range sortedKeys {
		b := intervals[key]
		iv := aggregateInterval(b.lines, isServerSide || inServerReport)
		if iv != nil {
			result.Intervals = append(result.Intervals, *iv)
		}
	}

	// Build summary from SUM line or from aggregating all per-stream final intervals
	if sumLine != nil {
		applySumToResult(result, sumLine, isServerSide)
	} else if len(allParsed) > 0 {
		buildSummaryFromParsed(result, allParsed, isServerSide)
	}

	// Set actual duration from the last interval
	if len(result.Intervals) > 0 {
		result.ActualDuration = result.Intervals[len(result.Intervals)-1].TimeEnd
	}

	return result, nil
}

// parseSumLine parses a [SUM] or [SUM-N] line.
func parseSumLine(line string) *parsedLine {
	// Try server SUM with jitter
	if m := reSumServer.FindStringSubmatch(line); m != nil {
		p := &parsedLine{isSum: true, hasUDP: true}
		p.timeStart, _ = strconv.ParseFloat(m[1], 64)
		p.timeEnd, _ = strconv.ParseFloat(m[2], 64)
		mb, _ := strconv.ParseFloat(m[3], 64)
		p.bytes = int64(mb * 1_000_000)
		bw, _ := strconv.ParseFloat(m[4], 64)
		p.bandwidthBps = bw * 1_000_000
		p.jitterMs, _ = strconv.ParseFloat(m[5], 64)
		p.lostPackets, _ = strconv.Atoi(m[6])
		p.totalPackets, _ = strconv.Atoi(m[7])
		p.lostPct, _ = strconv.ParseFloat(m[8], 64)
		return p
	}

	// Try server SUM without jitter in the regex (jitter might be embedded differently)
	if m := reSumServerNoJitter.FindStringSubmatch(line); m != nil {
		p := &parsedLine{isSum: true, hasUDP: true}
		p.timeStart, _ = strconv.ParseFloat(m[1], 64)
		p.timeEnd, _ = strconv.ParseFloat(m[2], 64)
		mb, _ := strconv.ParseFloat(m[3], 64)
		p.bytes = int64(mb * 1_000_000)
		bw, _ := strconv.ParseFloat(m[4], 64)
		p.bandwidthBps = bw * 1_000_000
		p.lostPackets, _ = strconv.Atoi(m[5])
		p.totalPackets, _ = strconv.Atoi(m[6])
		p.lostPct, _ = strconv.ParseFloat(m[7], 64)
		return p
	}

	// Try client SUM (no loss)
	if m := reSumClient.FindStringSubmatch(line); m != nil {
		p := &parsedLine{isSum: true}
		p.timeStart, _ = strconv.ParseFloat(m[1], 64)
		p.timeEnd, _ = strconv.ParseFloat(m[2], 64)
		mb, _ := strconv.ParseFloat(m[3], 64)
		p.bytes = int64(mb * 1_000_000)
		bw, _ := strconv.ParseFloat(m[4], 64)
		p.bandwidthBps = bw * 1_000_000
		return p
	}

	return nil
}

// aggregateInterval combines per-stream data at a time bucket into a single IntervalResult.
func aggregateInterval(lines []*parsedLine, isServer bool) *model.IntervalResult {
	if len(lines) == 0 {
		return nil
	}

	iv := &model.IntervalResult{
		TimeStart: lines[0].timeStart,
		TimeEnd:   lines[0].timeEnd,
	}

	var totalBw float64
	var totalBytes int64
	var totalLost, totalPkts int
	var jitterSum float64
	jitterCount := 0

	for _, p := range lines {
		totalBw += p.bandwidthBps
		totalBytes += p.bytes
		if p.hasUDP {
			totalLost += p.lostPackets
			totalPkts += p.totalPackets
			if p.jitterMs > 0 {
				jitterSum += p.jitterMs
				jitterCount++
			}
		}
	}

	iv.BandwidthBps = totalBw
	iv.Bytes = totalBytes
	iv.LostPackets = totalLost
	iv.Packets = totalPkts
	if totalPkts > 0 {
		iv.LostPercent = float64(totalLost) / float64(totalPkts) * 100
	}
	if jitterCount > 0 {
		iv.JitterMs = jitterSum / float64(jitterCount)
	}

	return iv
}

// applySumToResult populates the TestResult summary fields from a SUM line.
func applySumToResult(result *model.TestResult, sum *parsedLine, isServerSide bool) {
	result.ActualDuration = sum.timeEnd

	if isServerSide {
		result.ReceivedBps = sum.bandwidthBps
		result.FwdReceivedBps = sum.bandwidthBps
		result.BytesReceived = sum.bytes
		if sum.hasUDP {
			result.JitterMs = sum.jitterMs
			result.FwdJitterMs = sum.jitterMs
			result.LostPackets = sum.lostPackets
			result.FwdLostPackets = sum.lostPackets
			result.Packets = sum.totalPackets
			result.FwdPackets = sum.totalPackets
			result.LostPercent = sum.lostPct
			result.FwdLostPercent = sum.lostPct
		}
	} else {
		result.SentBps = sum.bandwidthBps
		result.BytesSent = sum.bytes
		if sum.hasUDP {
			result.JitterMs = sum.jitterMs
			result.LostPackets = sum.lostPackets
			result.Packets = sum.totalPackets
			result.LostPercent = sum.lostPct
		}
	}
}

// buildSummaryFromParsed builds summary from per-stream final intervals
// (when no SUM line is present — e.g. single stream).
func buildSummaryFromParsed(result *model.TestResult, parsed []*parsedLine, isServerSide bool) {
	// Find the final interval for each stream (longest timeEnd)
	finalByStream := map[int]*parsedLine{}
	for _, p := range parsed {
		existing, ok := finalByStream[p.streamID]
		if !ok || p.timeEnd > existing.timeEnd {
			// Check if this looks like a summary line (covers the full test duration)
			isSummary := p.timeStart == 0 && p.timeEnd > 1
			if existingIsSummary := existing != nil && existing.timeStart == 0 && existing.timeEnd > 1; existingIsSummary {
				// Both are summaries, use the one with longer duration
				if p.timeEnd > existing.timeEnd {
					finalByStream[p.streamID] = p
				}
			} else if isSummary {
				finalByStream[p.streamID] = p
			} else if !ok {
				finalByStream[p.streamID] = p
			}
		}
	}

	var totalBw float64
	var totalBytes int64
	var totalLost, totalPkts int
	var jitterSum float64
	jitterCount := 0

	for _, p := range finalByStream {
		totalBw += p.bandwidthBps
		totalBytes += p.bytes
		if p.hasUDP {
			totalLost += p.lostPackets
			totalPkts += p.totalPackets
			if p.jitterMs > 0 {
				jitterSum += p.jitterMs
				jitterCount++
			}
		}
	}

	if isServerSide {
		result.ReceivedBps = totalBw
		result.FwdReceivedBps = totalBw
		result.BytesReceived = totalBytes
		if totalPkts > 0 {
			result.JitterMs = jitterSum / float64(max(jitterCount, 1))
			result.FwdJitterMs = result.JitterMs
			result.LostPackets = totalLost
			result.FwdLostPackets = totalLost
			result.Packets = totalPkts
			result.FwdPackets = totalPkts
			result.LostPercent = float64(totalLost) / float64(totalPkts) * 100
			result.FwdLostPercent = result.LostPercent
		}
	} else {
		result.SentBps = totalBw
		result.BytesSent = totalBytes
		if totalPkts > 0 {
			result.JitterMs = jitterSum / float64(max(jitterCount, 1))
			result.LostPackets = totalLost
			result.Packets = totalPkts
			result.LostPercent = float64(totalLost) / float64(totalPkts) * 100
		}
	}
}

// PairBidirIntervals returns an onInterval callback that buffers half-pairs
// (forward-only or reverse-only) until the matching opposite half arrives, then
// emits a combined (fwd, rev) row to `emit`. Pairing is keyed by rounded
// TimeStart so floating-point jitter between fwd/rev clocks doesn't break it.
// Lone halves older than two intervals are flushed unpaired so the user always
// sees data. Flush() drains any remaining buffered halves.
//
// The returned callback and Flush are safe for concurrent use.
func PairBidirIntervals(emit func(fwd, rev *model.IntervalResult)) (onInterval func(fwd, rev *model.IntervalResult), flush func()) {
	type slot struct {
		fwd, rev *model.IntervalResult
	}
	var (
		mu      sync.Mutex
		pending = map[int64]*slot{}
		order   []int64 // insertion order of keys
	)

	keyOf := func(iv *model.IntervalResult) int64 {
		// Round to nearest 100ms — interval starts align to whole seconds
		// for typical -i 1, but we tolerate small skew.
		return int64(iv.TimeStart*10 + 0.5)
	}

	onInterval = func(fwd, rev *model.IntervalResult) {
		if fwd == nil && rev == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		ref := fwd
		if ref == nil {
			ref = rev
		}
		k := keyOf(ref)
		s, ok := pending[k]
		if !ok {
			s = &slot{}
			pending[k] = s
			order = append(order, k)
		}
		if fwd != nil {
			s.fwd = fwd
		}
		if rev != nil {
			s.rev = rev
		}
		if s.fwd != nil && s.rev != nil {
			emit(s.fwd, s.rev)
			delete(pending, k)
			// remove k from order
			for i, kk := range order {
				if kk == k {
					order = append(order[:i], order[i+1:]...)
					break
				}
			}
		}
	}

	flush = func() {
		mu.Lock()
		defer mu.Unlock()
		for _, k := range order {
			if s := pending[k]; s != nil {
				emit(s.fwd, s.rev)
			}
		}
		pending = map[int64]*slot{}
		order = nil
	}
	return
}

// IntervalAggregator accumulates per-stream interval lines that share a
// TimeStart into a single combined IntervalResult. It is used when iperf2 is
// run with -P N>1 — each interval produces N per-stream rows (and a SUM row),
// which the streaming display would otherwise emit individually. The aggregator
// holds the current bucket until a new TimeStart arrives or Flush is called.
//
// Per-stream cumulative "totals" (where TimeStart resets to 0 after we have
// already seen TimeStart > 0 from the same stream) are dropped. SUM lines are
// also dropped — when present they would double-count.
//
// emit is invoked from the same goroutine that calls Add (or Flush) so it does
// not need extra synchronization here; callers must serialize Add/Flush.
type IntervalAggregator struct {
	emit       func(*model.IntervalResult)
	maxStart   map[int]float64
	curStart   float64
	curEnd     float64
	curBytes   int64
	curBwBps   float64
	curJitter  float64
	curJitterN int
	curLost    int
	curPkts    int
	curStreams int
	hasCur     bool
}

// NewIntervalAggregator builds an aggregator that emits combined intervals.
func NewIntervalAggregator(emit func(*model.IntervalResult)) *IntervalAggregator {
	return &IntervalAggregator{emit: emit, maxStart: make(map[int]float64)}
}

// Add ingests one parsed interval. SUM lines and per-stream cumulative totals
// are dropped. Per-stream lines are summed into the current bucket; when a new
// TimeStart arrives, the previous bucket is emitted first.
func (a *IntervalAggregator) Add(iv *model.IntervalResult) {
	if iv == nil {
		return
	}
	// Drop per-stream cumulative totals (TimeStart resets to 0 after >0 seen).
	if iv.StreamID > 0 {
		prev, seen := a.maxStart[iv.StreamID]
		if seen && iv.TimeStart == 0 && prev > 0 {
			return
		}
		if iv.TimeStart > prev {
			a.maxStart[iv.StreamID] = iv.TimeStart
		}
	} else {
		// StreamID == 0 means SUM line (parseSumLine leaves StreamID zero).
		// Drop it — we sum per-stream lines ourselves to avoid double counting.
		return
	}

	if a.hasCur && iv.TimeStart != a.curStart {
		a.flushCurrent()
	}
	if !a.hasCur {
		a.curStart = iv.TimeStart
		a.curEnd = iv.TimeEnd
		a.hasCur = true
	}
	a.curBytes += iv.Bytes
	a.curBwBps += iv.BandwidthBps
	a.curLost += iv.LostPackets
	a.curPkts += iv.Packets
	if iv.JitterMs > 0 {
		a.curJitter += iv.JitterMs
		a.curJitterN++
	}
	if iv.TimeEnd > a.curEnd {
		a.curEnd = iv.TimeEnd
	}
	a.curStreams++
}

// Flush emits the current bucket if any data is pending.
func (a *IntervalAggregator) Flush() {
	if a.hasCur {
		a.flushCurrent()
	}
}

func (a *IntervalAggregator) flushCurrent() {
	out := &model.IntervalResult{
		TimeStart:    a.curStart,
		TimeEnd:      a.curEnd,
		Bytes:        a.curBytes,
		BandwidthBps: a.curBwBps,
		LostPackets:  a.curLost,
		Packets:      a.curPkts,
	}
	if a.curJitterN > 0 {
		out.JitterMs = a.curJitter / float64(a.curJitterN)
	}
	if a.curPkts > 0 {
		out.LostPercent = float64(a.curLost) / float64(a.curPkts) * 100
	}
	a.emit(out)
	a.hasCur = false
	a.curBytes = 0
	a.curBwBps = 0
	a.curJitter = 0
	a.curJitterN = 0
	a.curLost = 0
	a.curPkts = 0
	a.curStreams = 0
}

// IntervalFilter rejects per-stream and SUM "totals" lines that iperf2 prints
// after the regular interval rows. The heuristic: once a stream has emitted an
// interval whose TimeStart > 0, any subsequent line from the same stream whose
// TimeStart == 0 is the cumulative total and is dropped.
type IntervalFilter struct {
	maxStart map[int]float64
}

// NewIntervalFilter constructs an empty filter ready for use.
func NewIntervalFilter() *IntervalFilter {
	return &IntervalFilter{maxStart: make(map[int]float64)}
}

// Accept returns true if the interval should be forwarded to the caller.
// It also updates internal per-stream state.
func (f *IntervalFilter) Accept(iv *model.IntervalResult) bool {
	if iv == nil {
		return false
	}
	prev, seen := f.maxStart[iv.StreamID]
	if seen && iv.TimeStart == 0 && prev > 0 {
		// Per-stream cumulative total — drop.
		return false
	}
	if iv.TimeStart > prev {
		f.maxStart[iv.StreamID] = iv.TimeStart
	}
	return true
}

// ParseIntervalLine parses a single iperf2 output line into an IntervalResult
// for real-time display during piped output. Returns nil, nil if the line
// doesn't match any known interval format.
func ParseIntervalLine(line string) (*model.IntervalResult, error) {
	p, ok := parseSingleLine(line)
	if !ok {
		return nil, nil
	}

	return &model.IntervalResult{
		TimeStart:    p.timeStart,
		TimeEnd:      p.timeEnd,
		Bytes:        p.bytes,
		BandwidthBps: p.bandwidthBps,
		LostPackets:  p.lostPackets,
		Packets:      p.totalPackets,
		LostPercent:  p.lostPct,
		JitterMs:     p.jitterMs,
		StreamID:     p.streamID,
	}, nil
}

// ValidateServerReport checks the client output text for Server Report validity.
func ValidateServerReport(text string) ServerReportStatus {
	hasReport := false
	hasWarning := false

	for _, line := range strings.Split(text, "\n") {
		if reServerReport.MatchString(line) {
			hasReport = true
		}
		if reACKWarning.MatchString(line) {
			hasWarning = true
		}
	}

	if !hasReport {
		return ServerReportMissing
	}
	if hasWarning {
		return ServerReportFabricated
	}
	// Check for fabricated data: 0.000 ms jitter in a Server Report data line is
	// a strong indicator even without an explicit WARNING (e.g. Tailscale, SIGTERM).
	// After a "Server Report:" header, check data lines for jitter == 0.000 ms.
	lines := strings.Split(text, "\n")
	inReport := false
	allZeroJitter := true
	reportDataLines := 0
	for _, line := range lines {
		if reServerReport.MatchString(line) {
			inReport = true
			continue
		}
		if inReport {
			m := reServerInterval.FindStringSubmatch(line)
			if m == nil {
				m = reServerEnhanced.FindStringSubmatch(line)
			}
			if m != nil {
				reportDataLines++
				jitter, _ := strconv.ParseFloat(m[6], 64)
				if jitter != 0.0 {
					allZeroJitter = false
				}
			}
		}
	}
	if reportDataLines > 0 && allZeroJitter {
		return ServerReportFabricated
	}
	return ServerReportValid
}

// MergeBidirResults merges four partial results into a single bidirectional result.
// fwdClient: client-side forward data (send stats)
// fwdServer: server-side forward data (receive stats — jitter, loss)
// revClient: remote client reverse data (send stats)
// revServer: local server reverse data (receive stats — jitter, loss)
func MergeBidirResults(fwdClient, fwdServer, revClient, revServer *model.TestResult) *model.TestResult {
	result := *fwdClient
	result.Direction = "Bidirectional"
	result.Timestamp = time.Now()

	// Forward server-measured data (receiving side)
	if fwdServer != nil {
		result.FwdReceivedBps = fwdServer.FwdReceivedBps
		if result.FwdReceivedBps == 0 {
			result.FwdReceivedBps = fwdServer.ReceivedBps
		}
		result.BytesReceived = fwdServer.BytesReceived
		result.FwdLostPackets = fwdServer.FwdLostPackets
		if result.FwdLostPackets == 0 {
			result.FwdLostPackets = fwdServer.LostPackets
		}
		result.FwdPackets = fwdServer.FwdPackets
		if result.FwdPackets == 0 {
			result.FwdPackets = fwdServer.Packets
		}
		result.FwdJitterMs = fwdServer.FwdJitterMs
		if result.FwdJitterMs == 0 {
			result.FwdJitterMs = fwdServer.JitterMs
		}
		result.FwdLostPercent = fwdServer.FwdLostPercent
		if result.FwdLostPercent == 0 {
			result.FwdLostPercent = fwdServer.LostPercent
		}
	}

	// Reverse client data (sending side)
	if revClient != nil {
		result.ReverseSentBps = revClient.SentBps
		result.ReverseBytesSent = revClient.BytesSent
	}

	// Reverse server-measured data (receiving side)
	if revServer != nil {
		result.ReverseReceivedBps = revServer.FwdReceivedBps
		if result.ReverseReceivedBps == 0 {
			result.ReverseReceivedBps = revServer.ReceivedBps
		}
		result.ReverseBytesReceived = revServer.BytesReceived
		result.ReverseJitterMs = revServer.FwdJitterMs
		if result.ReverseJitterMs == 0 {
			result.ReverseJitterMs = revServer.JitterMs
		}
		result.ReverseLostPackets = revServer.FwdLostPackets
		if result.ReverseLostPackets == 0 {
			result.ReverseLostPackets = revServer.LostPackets
		}
		result.ReversePackets = revServer.FwdPackets
		if result.ReversePackets == 0 {
			result.ReversePackets = revServer.Packets
		}
		result.ReverseLostPercent = revServer.FwdLostPercent
		if result.ReverseLostPercent == 0 {
			result.ReverseLostPercent = revServer.LostPercent
		}
		result.ReverseIntervals = revServer.Intervals
	}

	return &result
}

// MergeUnidirResults merges client and server results for a unidirectional test.
// client: send-side data, server: receive-side data (jitter, loss).
func MergeUnidirResults(client, server *model.TestResult) *model.TestResult {
	result := *client

	if server != nil {
		result.FwdReceivedBps = server.FwdReceivedBps
		if result.FwdReceivedBps == 0 {
			result.FwdReceivedBps = server.ReceivedBps
		}
		if result.FwdReceivedBps == 0 {
			result.FwdReceivedBps = server.SentBps // server-side "sent" = received data
		}
		result.ReceivedBps = result.FwdReceivedBps
		result.FwdLostPackets = server.FwdLostPackets
		if result.FwdLostPackets == 0 {
			result.FwdLostPackets = server.LostPackets
		}
		result.FwdPackets = server.FwdPackets
		if result.FwdPackets == 0 {
			result.FwdPackets = server.Packets
		}
		result.FwdJitterMs = server.FwdJitterMs
		if result.FwdJitterMs == 0 {
			result.FwdJitterMs = server.JitterMs
		}
		result.FwdLostPercent = server.FwdLostPercent
		if result.FwdLostPercent == 0 {
			result.FwdLostPercent = server.LostPercent
		}
		// Set BytesReceived from server-side data for CSV/display
		if result.BytesReceived == 0 {
			result.BytesReceived = server.BytesReceived
			if result.BytesReceived == 0 {
				result.BytesReceived = server.BytesSent
			}
		}
		// Set jitter/loss on legacy fields for non-bidir display
		if result.JitterMs == 0 {
			result.JitterMs = result.FwdJitterMs
		}
		if result.LostPackets == 0 && result.FwdLostPackets > 0 {
			result.LostPackets = result.FwdLostPackets
			result.LostPercent = result.FwdLostPercent
			result.Packets = result.FwdPackets
		}
	}

	return &result
}

// ParseDualtestOutput parses the output of iperf2 -d (dualtest) mode.
// In dualtest mode, iperf2 runs both directions simultaneously. The output
// interleaves two stream IDs: one for the forward (client→server) direction
// and one for the reverse (server→client) direction.
//
// The forward stream is the client (sender) — lines have no jitter/loss.
// The reverse stream is the local server (receiver) — lines may have jitter/loss for UDP.
// Stream IDs are used to separate the two directions.
func ParseDualtestOutput(text string) (*model.TestResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty dualtest output")
	}

	lines := strings.Split(text, "\n")

	// First pass: collect all parsed lines and discover stream IDs
	type lineInfo struct {
		parsed *parsedLine
		raw    string
	}
	var allLines []lineInfo
	streamIDs := map[int]bool{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[SUM") {
			continue // skip SUM lines, we'll build our own summaries
		}
		p, ok := parseSingleLine(trimmed)
		if !ok {
			continue
		}
		allLines = append(allLines, lineInfo{parsed: p, raw: trimmed})
		streamIDs[p.streamID] = true
	}

	if len(allLines) == 0 {
		return nil, fmt.Errorf("no parseable data in dualtest output")
	}

	// In dualtest mode, there are typically 2 stream IDs.
	// The forward direction (client sending) has client-side lines (no UDP fields).
	// The reverse direction (receiving) has server-side lines (with UDP fields for UDP tests).
	// For TCP, both sides look similar — we separate by stream ID order:
	// the first stream ID seen is the forward (client-initiated) direction.

	// Determine which stream IDs are forward vs reverse.
	// Forward (outgoing): the client stream — first ID encountered, typically lower.
	// Reverse (incoming): the server-initiated stream — second ID, typically higher.
	var fwdID, revID int
	fwdSet := false
	for _, li := range allLines {
		if !fwdSet {
			fwdID = li.parsed.streamID
			fwdSet = true
		} else if li.parsed.streamID != fwdID {
			revID = li.parsed.streamID
			break
		}
	}

	// If only one stream ID found, treat all as forward
	if revID == 0 && fwdSet {
		revID = -1 // sentinel — no reverse data
	}

	// Separate lines by direction
	var fwdLines, revLines []*parsedLine
	for _, li := range allLines {
		if li.parsed.streamID == fwdID {
			fwdLines = append(fwdLines, li.parsed)
		} else {
			revLines = append(revLines, li.parsed)
		}
	}

	// Build interval results for each direction
	fwdIntervals := buildIntervals(fwdLines, false)
	revIntervals := buildIntervals(revLines, true)

	result := &model.TestResult{
		Timestamp:        time.Now(),
		Direction:        "Bidirectional",
		Intervals:        fwdIntervals,
		ReverseIntervals: revIntervals,
	}

	// Build summaries from the final (longest) intervals
	if len(fwdLines) > 0 {
		buildSummaryFromParsed(result, fwdLines, false)
	}

	// Reverse summary — the reverse stream is the local server (receiver side)
	if len(revLines) > 0 {
		var revResult model.TestResult
		buildSummaryFromParsed(&revResult, revLines, true)
		result.ReverseReceivedBps = revResult.ReceivedBps
		if result.ReverseReceivedBps == 0 {
			result.ReverseReceivedBps = revResult.FwdReceivedBps
		}
		result.ReverseBytesReceived = revResult.BytesReceived
		result.ReverseJitterMs = revResult.JitterMs
		if result.ReverseJitterMs == 0 {
			result.ReverseJitterMs = revResult.FwdJitterMs
		}
		result.ReverseLostPackets = revResult.LostPackets
		if result.ReverseLostPackets == 0 {
			result.ReverseLostPackets = revResult.FwdLostPackets
		}
		result.ReversePackets = revResult.Packets
		if result.ReversePackets == 0 {
			result.ReversePackets = revResult.FwdPackets
		}
		result.ReverseLostPercent = revResult.LostPercent
		if result.ReverseLostPercent == 0 {
			result.ReverseLostPercent = revResult.FwdLostPercent
		}
	}

	// Set actual duration
	if len(result.Intervals) > 0 {
		result.ActualDuration = result.Intervals[len(result.Intervals)-1].TimeEnd
	} else if len(result.ReverseIntervals) > 0 {
		result.ActualDuration = result.ReverseIntervals[len(result.ReverseIntervals)-1].TimeEnd
	}

	return result, nil
}

// buildIntervals groups parsed lines by time bucket and aggregates them into intervals.
func buildIntervals(lines []*parsedLine, isServer bool) []model.IntervalResult {
	type bucket struct {
		lines []*parsedLine
	}
	buckets := map[float64]*bucket{}
	var keys []float64

	for _, p := range lines {
		key := roundTime(p.timeStart)
		b, exists := buckets[key]
		if !exists {
			b = &bucket{}
			buckets[key] = b
			keys = insertSorted(keys, key)
		}
		b.lines = append(b.lines, p)
	}

	var intervals []model.IntervalResult
	for _, key := range keys {
		b := buckets[key]
		iv := aggregateInterval(b.lines, isServer)
		if iv != nil {
			intervals = append(intervals, *iv)
		}
	}
	return intervals
}

// roundTime rounds a float64 time to the nearest 0.01 for use as a map key.
func roundTime(t float64) float64 {
	return math.Round(t*100) / 100
}

// insertSorted inserts v into a sorted slice and returns the new slice.
func insertSorted(s []float64, v float64) []float64 {
	i := 0
	for i < len(s) && s[i] < v {
		i++
	}
	s = append(s, 0)
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}

