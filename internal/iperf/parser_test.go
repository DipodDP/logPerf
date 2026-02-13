package iperf

import (
	"math"
	"testing"
)

const sampleJSON = `{
	"start": {
		"connected": [{
			"socket": 5,
			"local_host": "192.168.1.100",
			"local_port": 43210,
			"remote_host": "192.168.1.1",
			"remote_port": 5201
		}],
		"test_start": {
			"protocol": "TCP",
			"num_streams": 4,
			"duration": 10,
			"blksize": 131072,
			"omit": 0
		},
		"timestamp": {
			"time": "Mon, 01 Jan 2024 12:00:00 GMT",
			"timesecs": 1704110400
		}
	},
	"intervals": [],
	"end": {
		"sum_sent": {
			"start": 0,
			"end": 10.0,
			"seconds": 10.0,
			"bytes": 1175000000,
			"bits_per_second": 940000000.0,
			"retransmits": 42,
			"sender": true
		},
		"sum_received": {
			"start": 0,
			"end": 10.0,
			"seconds": 10.0,
			"bytes": 1170000000,
			"bits_per_second": 936000000.0,
			"sender": false
		}
	}
}`

func TestParseResult(t *testing.T) {
	result, err := ParseResult([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseResult() error: %v", err)
	}

	if result.ServerAddr != "192.168.1.1" {
		t.Errorf("ServerAddr = %q, want %q", result.ServerAddr, "192.168.1.1")
	}
	if result.Port != 5201 {
		t.Errorf("Port = %d, want %d", result.Port, 5201)
	}
	if result.Parallel != 4 {
		t.Errorf("Parallel = %d, want %d", result.Parallel, 4)
	}
	if result.Duration != 10 {
		t.Errorf("Duration = %d, want %d", result.Duration, 10)
	}
	if result.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want %q", result.Protocol, "TCP")
	}
	if math.Abs(result.SentBps-940000000.0) > 1 {
		t.Errorf("SentBps = %f, want 940000000", result.SentBps)
	}
	if math.Abs(result.ReceivedBps-936000000.0) > 1 {
		t.Errorf("ReceivedBps = %f, want 936000000", result.ReceivedBps)
	}
	if result.Retransmits != 42 {
		t.Errorf("Retransmits = %d, want %d", result.Retransmits, 42)
	}
	if result.Error != "" {
		t.Errorf("Error = %q, want empty", result.Error)
	}

	// Check Mbps helpers
	if math.Abs(result.SentMbps()-940.0) > 0.01 {
		t.Errorf("SentMbps() = %f, want 940.0", result.SentMbps())
	}
}

const sampleErrorJSON = `{
	"start": {},
	"intervals": [],
	"end": {},
	"error": "error - unable to connect to server: Connection refused"
}`

func TestParseResultWithError(t *testing.T) {
	result, err := ParseResult([]byte(sampleErrorJSON))
	if err != nil {
		t.Fatalf("ParseResult() error: %v", err)
	}

	if result.Error == "" {
		t.Error("expected error field to be populated")
	}
	if result.Status() != "error - unable to connect to server: Connection refused" {
		t.Errorf("Status() = %q", result.Status())
	}
}

func TestParseResultInvalidJSON(t *testing.T) {
	_, err := ParseResult([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
