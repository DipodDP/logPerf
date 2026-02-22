package iperf

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iperf-tool/internal/model"
)

// iperfOutput represents the top-level iperf3 JSON output structure.
type iperfOutput struct {
	Start iperfStart `json:"start"`
	End   iperfEnd   `json:"end"`
	Error string     `json:"error"`
}

type iperfStart struct {
	Connected []iperfConnected `json:"connected"`
	TestStart iperfTestStart   `json:"test_start"`
	Timestamp iperfTimestamp   `json:"timestamp"`
}

type iperfConnected struct {
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
}

type iperfTestStart struct {
	Protocol   string `json:"protocol"`
	NumStreams  int    `json:"num_streams"`
	Duration   int    `json:"duration"`
	// iperf3 doesn't include interval in test_start; we parse it from omit/reporting
}

type iperfTimestamp struct {
	TimeSecs int64 `json:"timesecs"`
}

type iperfEnd struct {
	Sum                     iperfSum         `json:"sum"`
	SumSent                 iperfSum         `json:"sum_sent"`
	SumReceived             iperfSum         `json:"sum_received"`
	SumSentBidirReverse     iperfSum         `json:"sum_sent_bidir_reverse"`
	SumReceivedBidirReverse iperfSum         `json:"sum_received_bidir_reverse"`
	Streams                 []iperfStreamEnd `json:"streams"`
	ServerOutputJson        *iperfOutput     `json:"server_output_json"` // present with --get-server-output
}

type iperfSum struct {
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	JitterMs      float64 `json:"jitter_ms"`
	LostPackets   int     `json:"lost_packets"`
	Packets       int     `json:"packets"`
	LostPercent   float64 `json:"lost_percent"`
	Seconds       float64 `json:"seconds"`
}

type iperfStreamEnd struct {
	Sender   iperfStreamSide  `json:"sender"`
	Receiver iperfStreamSide  `json:"receiver"`
	UDP      *iperfStreamUDP  `json:"udp"`
}

type iperfStreamUDP struct {
	Socket        int     `json:"socket"`
	BitsPerSecond float64 `json:"bits_per_second"`
	JitterMs      float64 `json:"jitter_ms"`
	LostPackets   int     `json:"lost_packets"`
	Packets       int     `json:"packets"`
	LostPercent   float64 `json:"lost_percent"`
	Sender        bool    `json:"sender"` // true = forward (client→server), false = reverse
}

type iperfStreamSide struct {
	Socket        int     `json:"socket"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	Sender        bool    `json:"sender"`
}

// streamEvent represents a single line from iperf3 --json-stream output.
type streamEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// intervalData represents the data payload of a "interval" stream event.
type intervalData struct {
	Streams        []intervalStream `json:"streams"`
	Sum            intervalSum      `json:"sum"`
	SumBidirReverse *intervalSum   `json:"sum_bidir_reverse"` // present in bidir mode only
}

type intervalStream struct {
	Socket        int     `json:"socket"`
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Seconds       float64 `json:"seconds"`
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	Packets       int     `json:"packets"`
	LostPackets   int     `json:"lost_packets"`
	LostPercent   float64 `json:"lost_percent"`
	JitterMs      float64 `json:"jitter_ms"`
	Omitted       bool    `json:"omitted"`
}

type intervalSum struct {
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Seconds       float64 `json:"seconds"`
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	Packets       int     `json:"packets"`
	LostPackets   int     `json:"lost_packets"`
	LostPercent   float64 `json:"lost_percent"`
	JitterMs      float64 `json:"jitter_ms"`
	Omitted       bool    `json:"omitted"`
	Sender        bool    `json:"sender"` // true = forward, false = reverse (bidir mode)
}

// ParseStreamEvent parses a single line of --json-stream output.
func ParseStreamEvent(line []byte) (*streamEvent, error) {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, fmt.Errorf("parse stream event: %w", err)
	}
	if ev.Event == "" {
		return nil, fmt.Errorf("stream event missing 'event' field")
	}
	return &ev, nil
}

// ParseIntervalData parses the data from an "interval" stream event.
// Returns the forward interval and, in bidir mode, the reverse interval
// (from sum_bidir_reverse). The reverse return value is nil in normal mode.
func ParseIntervalData(data json.RawMessage) (fwd *model.IntervalResult, rev *model.IntervalResult, err error) {
	var id intervalData
	if err = json.Unmarshal(data, &id); err != nil {
		return nil, nil, fmt.Errorf("parse interval data: %w", err)
	}
	fwd = sumToInterval(id.Sum)
	if id.SumBidirReverse != nil {
		rev = sumToInterval(*id.SumBidirReverse)
	}
	// For UDP, sum.jitter_ms is 0 in --json-stream mode because the client is
	// the sender and jitter is measured by the receiver. Aggregate from
	// per-stream entries instead (non-bidir streams all have Sender=true on
	// the client side, so we take any stream with jitter_ms > 0).
	if fwd.JitterMs == 0 {
		var total float64
		n := 0
		for _, s := range id.Streams {
			if s.JitterMs > 0 {
				total += s.JitterMs
				n++
			}
		}
		if n > 0 {
			fwd.JitterMs = total / float64(n)
		}
	}
	return fwd, rev, nil
}

func sumToInterval(s intervalSum) *model.IntervalResult {
	return &model.IntervalResult{
		TimeStart:    s.Start,
		TimeEnd:      s.End,
		Bytes:        s.Bytes,
		BandwidthBps: s.BitsPerSecond,
		Retransmits:  s.Retransmits,
		Packets:      s.Packets,
		LostPackets:  s.LostPackets,
		LostPercent:  s.LostPercent,
		JitterMs:     s.JitterMs,
		Omitted:      s.Omitted,
	}
}

// ParseEndData parses the data from an "end" stream event into a TestResult.
func ParseEndData(data json.RawMessage) (*model.TestResult, error) {
	var end iperfEnd
	if err := json.Unmarshal(data, &end); err != nil {
		return nil, fmt.Errorf("parse end data: %w", err)
	}
	// Use the larger of sent/received seconds as actual duration (one will be 0 in reverse mode).
	actualDur := end.SumSent.Seconds
	if end.SumReceived.Seconds > actualDur {
		actualDur = end.SumReceived.Seconds
	}
	// UDP quality metrics (jitter, loss) are in sum_sent for normal mode.
	// In reverse mode the client is the receiver so loss lives in sum_received.
	// Use whichever has a non-zero packet count.
	lostSrc := end.SumSent
	if lostSrc.Packets == 0 && end.SumReceived.Packets > 0 {
		lostSrc = end.SumReceived
	}
	result := &model.TestResult{
		Timestamp:            time.Now(),
		ActualDuration:       actualDur,
		SentBps:              end.SumSent.BitsPerSecond,
		ReceivedBps:          end.SumReceived.BitsPerSecond,
		Retransmits:          end.SumSent.Retransmits,
		JitterMs:             end.SumReceived.JitterMs,
		LostPackets:          lostSrc.LostPackets,
		LostPercent:          lostSrc.LostPercent,
		Packets:              lostSrc.Packets,
		BytesSent:            end.SumSent.Bytes,
		BytesReceived:        end.SumReceived.Bytes,
		ReverseSentBps:       end.SumSentBidirReverse.BitsPerSecond,
		ReverseReceivedBps:   end.SumReceivedBidirReverse.BitsPerSecond,
		ReverseRetransmits:   end.SumSentBidirReverse.Retransmits,
		ReverseBytesSent:     end.SumSentBidirReverse.Bytes,
		ReverseBytesReceived: end.SumReceivedBidirReverse.Bytes,
		ReverseLostPackets:   end.SumReceivedBidirReverse.LostPackets,
		ReverseLostPercent:   end.SumReceivedBidirReverse.LostPercent,
		ReversePackets:       end.SumReceivedBidirReverse.Packets,
		ReverseJitterMs:      end.SumReceivedBidirReverse.JitterMs,
	}
	for i, s := range end.Streams {
		if s.UDP != nil {
			bps := s.UDP.BitsPerSecond
			// In UDP bidir, RX streams (sender=false) report received bandwidth
			// via the TCP-style Receiver field; UDP.BitsPerSecond is 0 for them.
			if !s.UDP.Sender && bps == 0 {
				bps = s.Receiver.BitsPerSecond
			}
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				Socket:      s.UDP.Socket,
				SentBps:     bps,
				JitterMs:    s.UDP.JitterMs,
				LostPackets: s.UDP.LostPackets,
				LostPercent: udpLostPct(s.UDP.LostPackets, s.UDP.Packets, s.UDP.LostPercent),
				Packets:     s.UDP.Packets,
				Sender:      s.UDP.Sender,
			})
		} else {
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				Socket:      s.Sender.Socket,
				SentBps:     s.Sender.BitsPerSecond,
				ReceivedBps: s.Receiver.BitsPerSecond,
				Retransmits: s.Sender.Retransmits,
				Sender:      s.Sender.Sender,
			})
		}
	}
	fillReverseSummaryFromStreams(result)
	fillUDPBidirFwdJitter(result, end)
	fillUDPFwdLostFromServer(result, end)
	return result, nil
}

// ParseStartData extracts connection and test metadata from a "start" stream event.
func ParseStartData(data json.RawMessage, result *model.TestResult) error {
	var start iperfStart
	if err := json.Unmarshal(data, &start); err != nil {
		return fmt.Errorf("parse start data: %w", err)
	}
	if len(start.Connected) > 0 {
		result.ServerAddr = start.Connected[0].RemoteHost
		result.Port = start.Connected[0].RemotePort
	}
	result.Protocol = strings.ToUpper(start.TestStart.Protocol)
	result.Parallel = start.TestStart.NumStreams
	result.Duration = start.TestStart.Duration
	if start.Timestamp.TimeSecs > 0 {
		result.Timestamp = time.Unix(start.Timestamp.TimeSecs, 0)
	}
	return nil
}

// ParseResult parses raw iperf3 JSON output into a TestResult.
func ParseResult(jsonData []byte) (*model.TestResult, error) {
	var out iperfOutput
	if err := json.Unmarshal(jsonData, &out); err != nil {
		return nil, fmt.Errorf("parse iperf3 JSON: %w", err)
	}

	actualDur := out.End.SumSent.Seconds
	if out.End.SumReceived.Seconds > actualDur {
		actualDur = out.End.SumReceived.Seconds
	}
	lostSrc := out.End.SumSent
	if lostSrc.Packets == 0 && out.End.SumReceived.Packets > 0 {
		lostSrc = out.End.SumReceived
	}
	result := &model.TestResult{
		Timestamp:            time.Now(),
		ActualDuration:       actualDur,
		SentBps:              out.End.SumSent.BitsPerSecond,
		ReceivedBps:          out.End.SumReceived.BitsPerSecond,
		Retransmits:          out.End.SumSent.Retransmits,
		JitterMs:             out.End.SumReceived.JitterMs,
		LostPackets:          lostSrc.LostPackets,
		LostPercent:          lostSrc.LostPercent,
		Packets:              lostSrc.Packets,
		BytesSent:            out.End.SumSent.Bytes,
		BytesReceived:        out.End.SumReceived.Bytes,
		ReverseSentBps:       out.End.SumSentBidirReverse.BitsPerSecond,
		ReverseReceivedBps:   out.End.SumReceivedBidirReverse.BitsPerSecond,
		ReverseRetransmits:   out.End.SumSentBidirReverse.Retransmits,
		ReverseBytesSent:     out.End.SumSentBidirReverse.Bytes,
		ReverseBytesReceived: out.End.SumReceivedBidirReverse.Bytes,
		ReverseLostPackets:   out.End.SumReceivedBidirReverse.LostPackets,
		ReverseLostPercent:   out.End.SumReceivedBidirReverse.LostPercent,
		ReversePackets:       out.End.SumReceivedBidirReverse.Packets,
		ReverseJitterMs:      out.End.SumReceivedBidirReverse.JitterMs,
		Protocol:             strings.ToUpper(out.Start.TestStart.Protocol),
		Parallel:             out.Start.TestStart.NumStreams,
		Duration:             out.Start.TestStart.Duration,
	}

	if out.Start.Timestamp.TimeSecs > 0 {
		result.Timestamp = time.Unix(out.Start.Timestamp.TimeSecs, 0)
	}

	if len(out.Start.Connected) > 0 {
		result.ServerAddr = out.Start.Connected[0].RemoteHost
		result.Port = out.Start.Connected[0].RemotePort
	}

	for i, s := range out.End.Streams {
		if s.UDP != nil {
			bps := s.UDP.BitsPerSecond
			if !s.UDP.Sender && bps == 0 {
				bps = s.Receiver.BitsPerSecond
			}
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				Socket:      s.UDP.Socket,
				SentBps:     bps,
				JitterMs:    s.UDP.JitterMs,
				LostPackets: s.UDP.LostPackets,
				LostPercent: udpLostPct(s.UDP.LostPackets, s.UDP.Packets, s.UDP.LostPercent),
				Packets:     s.UDP.Packets,
				Sender:      s.UDP.Sender,
			})
		} else {
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				Socket:      s.Sender.Socket,
				SentBps:     s.Sender.BitsPerSecond,
				ReceivedBps: s.Receiver.BitsPerSecond,
				Retransmits: s.Sender.Retransmits,
				Sender:      s.Sender.Sender,
			})
		}
	}

	if out.Error != "" {
		result.Error = out.Error
	}

	fillReverseSummaryFromStreams(result)
	fillUDPBidirFwdJitter(result, out.End)
	fillUDPFwdLostFromServer(result, out.End)
	return result, nil
}

// fillReverseSummaryFromStreams computes reverse summary fields from per-stream
// data when the JSON didn't include sum_sent_bidir_reverse / sum_received_bidir_reverse
// (e.g. in --json-stream mode). Only acts when reverse summary is missing but
// reverse streams (Sender=false) are present.
// fillUDPBidirFwdJitter extracts the forward-direction jitter for UDP bidir tests.
// In batch (-J) mode, sender=true UDP stream entries contain server-measured jitter.
// In --json-stream mode, client-side stream entries have jitter_ms=0 for the sender;
// instead the server's data is embedded in server_output_json within the end event.
// We check both sources: client-side streams first, then server_output_json.
func fillUDPBidirFwdJitter(r *model.TestResult, end iperfEnd) {
	if r.FwdJitterMs != 0 {
		return // already set
	}
	// Try client-side streams (works in batch -J mode with --get-server-output).
	var total float64
	n := 0
	for _, s := range end.Streams {
		if s.UDP != nil && s.UDP.Sender && s.UDP.JitterMs > 0 {
			total += s.UDP.JitterMs
			n++
		}
	}
	if n > 0 {
		r.FwdJitterMs = total / float64(n)
		return
	}
	// Fall back to server_output_json (present in --json-stream + --get-server-output).
	// The server receives the forward direction, so its sum_received.jitter_ms is
	// the forward jitter as measured by the server.
	if end.ServerOutputJson != nil {
		srvEnd := end.ServerOutputJson.End
		if srvEnd.SumReceived.JitterMs > 0 {
			r.FwdJitterMs = srvEnd.SumReceived.JitterMs
			return
		}
		// Also try per-stream entries on the server side.
		total = 0
		n = 0
		for _, s := range srvEnd.Streams {
			if s.UDP != nil && s.UDP.JitterMs > 0 {
				total += s.UDP.JitterMs
				n++
			}
		}
		if n > 0 {
			r.FwdJitterMs = total / float64(n)
		}
	}
}

// fillUDPFwdLostFromServer overlays jitter/lost data onto Fwd UDP streams
// from server_output_json. The server measured loss as the receiver of the
// forward direction. Client and server socket IDs are independent, so we
// match by position: the Nth Fwd stream on the client corresponds to the Nth
// UDP stream entry in the server output.
func fillUDPFwdLostFromServer(r *model.TestResult, end iperfEnd) {
	if end.ServerOutputJson == nil {
		return
	}
	// Collect server UDP stream entries where the server is the receiver
	// (sender=false), i.e. the forward direction (client→server).
	// In bidir mode the server also has sender=true streams for the reverse
	// direction; those must be excluded.
	var srvStreams []iperfStreamUDP
	for _, s := range end.ServerOutputJson.End.Streams {
		if s.UDP != nil && !s.UDP.Sender {
			srvStreams = append(srvStreams, *s.UDP)
		}
	}
	// Populate summary-level fwd metrics from server's sum_received.
	// For UDP, bits_per_second in sum_received reports the sender's offered rate,
	// not the actually-delivered rate. Compute actual throughput from bytes/seconds.
	srvSum := end.ServerOutputJson.End.SumReceived
	if srvSum.Bytes > 0 && srvSum.Seconds > 0 {
		r.FwdReceivedBps = float64(srvSum.Bytes) * 8 / srvSum.Seconds
	}
	if srvSum.Packets > 0 {
		r.FwdLostPackets = srvSum.LostPackets
		r.FwdPackets = srvSum.Packets
		r.FwdLostPercent = udpLostPct(srvSum.LostPackets, srvSum.Packets, srvSum.LostPercent)
	}

	if len(srvStreams) == 0 {
		return
	}
	// Overlay onto Fwd streams by position.
	j := 0
	for i := range r.Streams {
		if !r.Streams[i].Sender {
			continue
		}
		if j >= len(srvStreams) {
			break
		}
		srv := srvStreams[j]
		j++
		r.Streams[i].LostPackets = srv.LostPackets
		r.Streams[i].Packets = srv.Packets
		if srv.Packets > 0 {
			r.Streams[i].LostPercent = float64(srv.LostPackets) / float64(srv.Packets) * 100
		}
	}
}

func fillReverseSummaryFromStreams(r *model.TestResult) {
	if r.ReverseSentBps != 0 {
		return // already populated from JSON
	}
	var sentBps, recvBps float64
	var retransmits int
	var lostPackets, packets int
	var lostPercent, jitterMs float64
	var udpCount int
	hasReverse := false
	for _, s := range r.Streams {
		if !s.Sender {
			hasReverse = true
			sentBps += s.SentBps
			recvBps += s.ReceivedBps
			retransmits += s.Retransmits
			if s.JitterMs > 0 || s.Packets > 0 {
				lostPackets += s.LostPackets
				packets += s.Packets
				lostPercent += s.LostPercent
				jitterMs += s.JitterMs
				udpCount++
			}
		}
	}
	if !hasReverse {
		return
	}
	// In --json-stream bidir mode, reverse streams have SentBps=0 (the client
	// is receiving, not sending). Use ReceivedBps as the throughput metric.
	if sentBps == 0 && recvBps > 0 {
		r.ReverseSentBps = recvBps
		r.ReverseReceivedBps = recvBps
	} else {
		r.ReverseSentBps = sentBps
		r.ReverseReceivedBps = recvBps
	}
	r.ReverseRetransmits = retransmits
	if udpCount > 0 {
		r.ReverseLostPackets = lostPackets
		r.ReversePackets = packets
		r.ReverseLostPercent = lostPercent / float64(udpCount)
		r.ReverseJitterMs = jitterMs / float64(udpCount)
	}
}

// udpLostPct returns the best available lost percent for a UDP stream.
// iperf3 sometimes reports lost_percent=0 even with non-zero lost counts;
// recompute from counts in that case.
func udpLostPct(lost, packets int, reported float64) float64 {
	if reported != 0 {
		return reported
	}
	if packets > 0 && lost > 0 {
		return float64(lost) / float64(packets) * 100
	}
	return 0
}

// serverTextBw matches iperf3 text receiver lines that report bandwidth.
// Matches both SUM (multi-stream) and per-stream (single-stream) lines:
//   [SUM]  0.00-10.00  sec  468 MBytes  392 Mbits/sec  receiver
//   [SUM][RX-S]  0.00-10.00  sec  468 MBytes  392 Mbits/sec  receiver
//   [  5][RX-S]  0.00-10.04  sec  101 MBytes  84.7 Mbits/sec  receiver
// Capture groups: 1=id ("SUM" or socket number), 2=role ("RX-S"/"TX-S" or ""), 3=value, 4=unit
var serverTextBw = regexp.MustCompile(
	`^\[\s*(SUM|\d+)\](?:\[([A-Z-]+)\])?\s+[\d.]+-[\d.]+\s+sec\s+[\d.]+\s+\w+Bytes\s+([\d.]+)\s*(G|M|K)?bits/sec.*\breceiver\b`)

// serverTextLost matches iperf3 text summary lines that contain Lost/Total counts.
// Matches both non-bidir:  [SUM]  0.00-10.00  sec ... 0.314 ms  46522/89300 (52%)  receiver
// and bidir RX-S:          [SUM][RX-S]  0.00-10.00  sec ... 0.314 ms  46522/89300 (52%)  receiver
// and per-stream:          [  5]  0.00-10.00  sec ... 0.314 ms  11631/22309 (52%)  receiver
// Capture groups: 1=socket-id (or "SUM"), 2=role ("RX-S","TX-S", or ""), 3=lost, 4=total
var serverTextLost = regexp.MustCompile(
	`^\[\s*(SUM|\d+)\](?:\[([A-Z-]+)\])?\s+[\d.]+-[\d.]+\s+sec\s+.*?\s+(\d+)/(\d+)\s+\([^)]+\)\s+receiver`)

// serverTextStream matches per-stream receiver lines (non-bidir, no role tag):
// [  5]   0.00-5.25   sec  ...  0.372 ms  11631/22309 (52%)  receiver
var serverTextStream = regexp.MustCompile(
	`^\[\s*(\d+)\]\s+[\d.]+-[\d.]+\s+sec\s+.*?\s+(\d+)/(\d+)\s+\([^)]+\)\s+receiver`)

// ParseServerOutputText extracts UDP forward-direction loss from the iperf3
// server text output (the server_output_text stream event). It overlays
// FwdLostPackets/FwdPackets on the result and per-stream Fwd streams.
// isBidir must be true when the test was run with --bidir.
// It is safe to call when text is empty or the result has no UDP streams.
func ParseServerOutputText(text string, r *model.TestResult, isBidir bool) {
	if text == "" {
		return
	}

	// Extract server-received forward bandwidth.
	// Prefer [SUM] line (multi-stream); fall back to summing per-stream lines (single-stream).
	if r.FwdReceivedBps == 0 {
		var sumBps float64
		sumFound := false
		var streamBps float64
		streamCount := 0
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			m := serverTextBw.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			id := m[1]   // "SUM" or socket number string
			role := m[2] // "RX-S", "TX-S", or ""
			isFwd := (!isBidir && role == "") || (isBidir && role == "RX-S")
			if !isFwd {
				continue
			}
			val, err := strconv.ParseFloat(m[3], 64)
			if err != nil {
				continue
			}
			switch m[4] {
			case "G":
				val *= 1e9
			case "M":
				val *= 1e6
			case "K":
				val *= 1e3
			}
			if id == "SUM" {
				sumBps = val
				sumFound = true
				break // SUM is authoritative
			}
			streamBps += val
			streamCount++
		}
		if sumFound {
			r.FwdReceivedBps = sumBps
		} else if streamCount > 0 {
			r.FwdReceivedBps = streamBps
		}
	}

	if r.Protocol != "UDP" {
		return
	}

	var sumLost, sumPkts int
	sumFound := false
	// per-stream: socket-id → (lost, total)
	streamLost := map[int][2]int{}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := serverTextLost.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := m[1]
		role := m[2]  // "RX-S", "TX-S", or ""
		lost, _ := strconv.Atoi(m[3])
		total, _ := strconv.Atoi(m[4])

		// In bidir mode, fwd loss is on [RX-S] lines (server receiving fwd traffic).
		// In normal mode, fwd loss is on untagged receiver lines.
		isFwd := (!isBidir && role == "") || (isBidir && role == "RX-S")
		if !isFwd {
			continue
		}

		if id == "SUM" {
			sumLost = lost
			sumPkts = total
			sumFound = true
		} else {
			sock, err := strconv.Atoi(id)
			if err == nil {
				streamLost[sock] = [2]int{lost, total}
			}
		}
	}

	if !sumFound && len(streamLost) > 0 {
		// No SUM line (single stream) — aggregate all per-stream entries.
		for _, counts := range streamLost {
			sumLost += counts[0]
			sumPkts += counts[1]
		}
		sumFound = sumPkts > 0
	}
	if sumFound && sumPkts > 0 {
		r.FwdLostPackets = sumLost
		r.FwdPackets = sumPkts
		r.FwdLostPercent = udpLostPct(sumLost, sumPkts, 0)
	}

	if len(streamLost) == 0 {
		return
	}
	// Match server socket IDs to client Fwd streams by position (socket IDs differ).
	// Build ordered list of server socket IDs as they appeared.
	var srvOrder []int
	seen := map[int]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		sm := serverTextStream.FindStringSubmatch(line)
		if sm == nil {
			// try bidir RX-S per-stream
			m2 := serverTextLost.FindStringSubmatch(line)
			if m2 == nil || m2[1] == "SUM" {
				continue
			}
			role := m2[2]
			isFwd := (!isBidir && role == "") || (isBidir && role == "RX-S")
			if !isFwd {
				continue
			}
			sock, err := strconv.Atoi(m2[1])
			if err == nil && !seen[sock] {
				seen[sock] = true
				srvOrder = append(srvOrder, sock)
			}
			continue
		}
		sock, _ := strconv.Atoi(sm[1])
		if !seen[sock] {
			seen[sock] = true
			srvOrder = append(srvOrder, sock)
		}
	}

	j := 0
	for i := range r.Streams {
		if !r.Streams[i].Sender {
			continue
		}
		if j >= len(srvOrder) {
			break
		}
		sock := srvOrder[j]
		j++
		if counts, ok := streamLost[sock]; ok {
			r.Streams[i].LostPackets = counts[0]
			r.Streams[i].Packets = counts[1]
			if counts[1] > 0 {
				r.Streams[i].LostPercent = float64(counts[0]) / float64(counts[1]) * 100
			}
		}
	}
}
