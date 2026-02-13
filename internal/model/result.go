package model

import "time"

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
