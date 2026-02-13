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
	SumSent     iperfSum `json:"sum_sent"`
	SumReceived iperfSum `json:"sum_received"`
}

type iperfSum struct {
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   int     `json:"retransmits"`
}

// ParseResult parses raw iperf3 JSON output into a TestResult.
func ParseResult(jsonData []byte) (*model.TestResult, error) {
	var out iperfOutput
	if err := json.Unmarshal(jsonData, &out); err != nil {
		return nil, fmt.Errorf("parse iperf3 JSON: %w", err)
	}

	result := &model.TestResult{
		Timestamp:   time.Now(),
		SentBps:     out.End.SumSent.BitsPerSecond,
		ReceivedBps: out.End.SumReceived.BitsPerSecond,
		Retransmits: out.End.SumSent.Retransmits,
		Protocol:    out.Start.TestStart.Protocol,
		Parallel:    out.Start.TestStart.NumStreams,
		Duration:    out.Start.TestStart.Duration,
	}

	if out.Start.Timestamp.TimeSecs > 0 {
		result.Timestamp = time.Unix(out.Start.Timestamp.TimeSecs, 0)
	}

	if len(out.Start.Connected) > 0 {
		result.ServerAddr = out.Start.Connected[0].RemoteHost
		result.Port = out.Start.Connected[0].RemotePort
	}

	if out.Error != "" {
		result.Error = out.Error
	}

	return result, nil
}
