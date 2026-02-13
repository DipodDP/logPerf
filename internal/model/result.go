package model

import (
	"math"
	"time"
)

// StreamResult holds per-stream throughput data.
type StreamResult struct {
	ID          int
	SentBps     float64
	ReceivedBps float64
	Retransmits int
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
	Streams     []StreamResult
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
	var sentSum, recvSum float64
	for _, s := range r.Streams {
		sentSum += s.SentBps
		recvSum += s.ReceivedBps
	}
	tolerance := 0.001
	sentOK = r.SentBps == 0 || math.Abs(sentSum-r.SentBps)/r.SentBps <= tolerance
	recvOK = r.ReceivedBps == 0 || math.Abs(recvSum-r.ReceivedBps)/r.ReceivedBps <= tolerance
	return sentOK, recvOK
}
