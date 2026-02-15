package iperf

import (
	"encoding/json"
	"fmt"
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
	SumSent                 iperfSum         `json:"sum_sent"`
	SumReceived             iperfSum         `json:"sum_received"`
	SumSentBidirReverse     iperfSum         `json:"sum_sent_bidir_reverse"`
	SumReceivedBidirReverse iperfSum         `json:"sum_received_bidir_reverse"`
	Streams                 []iperfStreamEnd `json:"streams"`
}

type iperfSum struct {
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	JitterMs      float64 `json:"jitter_ms"`
	LostPackets   int     `json:"lost_packets"`
	Packets       int     `json:"packets"`
	LostPercent   float64 `json:"lost_percent"`
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
	Streams []intervalStream `json:"streams"`
	Sum     intervalSum      `json:"sum"`
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
	Omitted       bool    `json:"omitted"`
}

type intervalSum struct {
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Seconds       float64 `json:"seconds"`
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
	Omitted       bool    `json:"omitted"`
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

// ParseIntervalData parses the data from an "interval" stream event into an IntervalResult.
func ParseIntervalData(data json.RawMessage) (*model.IntervalResult, error) {
	var id intervalData
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parse interval data: %w", err)
	}
	return &model.IntervalResult{
		TimeStart:    id.Sum.Start,
		TimeEnd:      id.Sum.End,
		Bytes:        id.Sum.Bytes,
		BandwidthBps: id.Sum.BitsPerSecond,
		Retransmits:  id.Sum.Retransmits,
		Omitted:      id.Sum.Omitted,
	}, nil
}

// ParseEndData parses the data from an "end" stream event into a TestResult.
func ParseEndData(data json.RawMessage) (*model.TestResult, error) {
	var end iperfEnd
	if err := json.Unmarshal(data, &end); err != nil {
		return nil, fmt.Errorf("parse end data: %w", err)
	}
	result := &model.TestResult{
		Timestamp:            time.Now(),
		SentBps:              end.SumSent.BitsPerSecond,
		ReceivedBps:          end.SumReceived.BitsPerSecond,
		Retransmits:          end.SumSent.Retransmits,
		JitterMs:             end.SumSent.JitterMs,
		LostPackets:          end.SumSent.LostPackets,
		LostPercent:          end.SumSent.LostPercent,
		Packets:              end.SumSent.Packets,
		BytesSent:            end.SumSent.Bytes,
		BytesReceived:        end.SumReceived.Bytes,
		ReverseSentBps:       end.SumSentBidirReverse.BitsPerSecond,
		ReverseReceivedBps:   end.SumReceivedBidirReverse.BitsPerSecond,
		ReverseRetransmits:   end.SumSentBidirReverse.Retransmits,
		ReverseBytesSent:     end.SumSentBidirReverse.Bytes,
		ReverseBytesReceived: end.SumReceivedBidirReverse.Bytes,
	}
	for i, s := range end.Streams {
		if s.UDP != nil {
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				SentBps:     s.UDP.BitsPerSecond,
				JitterMs:    s.UDP.JitterMs,
				LostPackets: s.UDP.LostPackets,
				LostPercent: s.UDP.LostPercent,
				Packets:     s.UDP.Packets,
			})
		} else {
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				SentBps:     s.Sender.BitsPerSecond,
				ReceivedBps: s.Receiver.BitsPerSecond,
				Retransmits: s.Sender.Retransmits,
				Sender:      s.Sender.Sender,
			})
		}
	}
	fillReverseSummaryFromStreams(result)
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
	result.Protocol = start.TestStart.Protocol
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

	result := &model.TestResult{
		Timestamp:            time.Now(),
		SentBps:              out.End.SumSent.BitsPerSecond,
		ReceivedBps:          out.End.SumReceived.BitsPerSecond,
		Retransmits:          out.End.SumSent.Retransmits,
		JitterMs:             out.End.SumSent.JitterMs,
		LostPackets:          out.End.SumSent.LostPackets,
		LostPercent:          out.End.SumSent.LostPercent,
		Packets:              out.End.SumSent.Packets,
		BytesSent:            out.End.SumSent.Bytes,
		BytesReceived:        out.End.SumReceived.Bytes,
		ReverseSentBps:       out.End.SumSentBidirReverse.BitsPerSecond,
		ReverseReceivedBps:   out.End.SumReceivedBidirReverse.BitsPerSecond,
		ReverseRetransmits:   out.End.SumSentBidirReverse.Retransmits,
		ReverseBytesSent:     out.End.SumSentBidirReverse.Bytes,
		ReverseBytesReceived: out.End.SumReceivedBidirReverse.Bytes,
		Protocol:             out.Start.TestStart.Protocol,
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
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
				SentBps:     s.UDP.BitsPerSecond,
				JitterMs:    s.UDP.JitterMs,
				LostPackets: s.UDP.LostPackets,
				LostPercent: s.UDP.LostPercent,
				Packets:     s.UDP.Packets,
			})
		} else {
			result.Streams = append(result.Streams, model.StreamResult{
				ID:          i + 1,
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
	return result, nil
}

// fillReverseSummaryFromStreams computes reverse summary fields from per-stream
// data when the JSON didn't include sum_sent_bidir_reverse / sum_received_bidir_reverse
// (e.g. in --json-stream mode). Only acts when reverse summary is missing but
// reverse streams (Sender=false) are present.
func fillReverseSummaryFromStreams(r *model.TestResult) {
	if r.ReverseSentBps != 0 {
		return // already populated from JSON
	}
	var sentBps, recvBps float64
	var retransmits int
	hasReverse := false
	for _, s := range r.Streams {
		if !s.Sender {
			hasReverse = true
			sentBps += s.SentBps
			recvBps += s.ReceivedBps
			retransmits += s.Retransmits
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
}
