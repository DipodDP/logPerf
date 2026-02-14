package model

import (
	"math"
	"time"
)

// IntervalResult holds a single interval measurement from an iperf3 test.
type IntervalResult struct {
	TimeStart    float64 // seconds from test start
	TimeEnd      float64
	Bytes        int64
	BandwidthBps float64
	Retransmits  int
	Packets      int
	Omitted      bool
}

// BandwidthMbps returns the interval bandwidth in Mbps.
func (r *IntervalResult) BandwidthMbps() float64 {
	return r.BandwidthBps / 1_000_000
}

// TransferMB returns the transferred data in megabytes.
func (r *IntervalResult) TransferMB() float64 {
	return float64(r.Bytes) / 1_000_000
}

// StreamResult holds per-stream throughput data.
type StreamResult struct {
	ID          int
	SentBps     float64
	ReceivedBps float64
	Retransmits int
	JitterMs    float64
	LostPackets int
	LostPercent float64
	Packets     int
}

// SentMbps returns the sent throughput in Mbps.
func (s *StreamResult) SentMbps() float64 {
	return s.SentBps / 1_000_000
}

// ReceivedMbps returns the received throughput in Mbps.
func (s *StreamResult) ReceivedMbps() float64 {
	return s.ReceivedBps / 1_000_000
}

// TestResult holds the parsed output of a single iperf3 test run.
type TestResult struct {
	Timestamp   time.Time
	ServerAddr  string
	Port        int
	Parallel    int
	Duration    int
	Interval    int
	Protocol    string
	SentBps     float64
	ReceivedBps float64
	Retransmits int
	JitterMs    float64
	LostPackets int
	LostPercent float64
	Packets     int
	Streams     []StreamResult
	Intervals   []IntervalResult
	Error       string
}

// SentMbps returns the sent throughput in Mbps.
func (r *TestResult) SentMbps() float64 {
	return r.SentBps / 1_000_000
}

// ReceivedMbps returns the received throughput in Mbps.
func (r *TestResult) ReceivedMbps() float64 {
	return r.ReceivedBps / 1_000_000
}

// Status returns "OK" or the error string.
func (r *TestResult) Status() string {
	if r.Error != "" {
		return r.Error
	}
	return "OK"
}

// VerifyStreamTotals checks that the sum of per-stream bps matches summary
// values within 0.1% tolerance. Returns (sentOK, recvOK).
func (r *TestResult) VerifyStreamTotals() (sentOK, recvOK bool) {
	if len(r.Streams) == 0 {
		return true, true
	}
	tolerance := 0.001

	// UDP streams have a single "udp" entry with no separate sent/received.
	// Compare the stream's SentBps against the summary SentBps.
	if r.Protocol == "UDP" {
		var sentSum float64
		for _, s := range r.Streams {
			sentSum += s.SentBps
		}
		sentOK = r.SentBps == 0 || math.Abs(sentSum-r.SentBps)/r.SentBps <= tolerance
		return sentOK, true
	}

	var sentSum, recvSum float64
	for _, s := range r.Streams {
		sentSum += s.SentBps
		recvSum += s.ReceivedBps
	}
	sentOK = r.SentBps == 0 || math.Abs(sentSum-r.SentBps)/r.SentBps <= tolerance
	recvOK = r.ReceivedBps == 0 || math.Abs(recvSum-r.ReceivedBps)/r.ReceivedBps <= tolerance
	return sentOK, recvOK
}
