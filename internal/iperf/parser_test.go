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
		},
		"streams": [
			{
				"sender": {"socket": 5, "bits_per_second": 470000000.0, "retransmits": 20},
				"receiver": {"socket": 5, "bits_per_second": 468000000.0}
			},
			{
				"sender": {"socket": 6, "bits_per_second": 470000000.0, "retransmits": 22},
				"receiver": {"socket": 6, "bits_per_second": 468000000.0}
			}
		]
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

	// Check per-stream data
	if len(result.Streams) != 2 {
		t.Fatalf("Streams count = %d, want 2", len(result.Streams))
	}
	if result.Streams[0].ID != 1 {
		t.Errorf("Streams[0].ID = %d, want 1", result.Streams[0].ID)
	}
	if math.Abs(result.Streams[0].SentBps-470000000.0) > 1 {
		t.Errorf("Streams[0].SentBps = %f, want 470000000", result.Streams[0].SentBps)
	}
	if math.Abs(result.Streams[0].ReceivedBps-468000000.0) > 1 {
		t.Errorf("Streams[0].ReceivedBps = %f, want 468000000", result.Streams[0].ReceivedBps)
	}
	if result.Streams[0].Retransmits != 20 {
		t.Errorf("Streams[0].Retransmits = %d, want 20", result.Streams[0].Retransmits)
	}
	if result.Streams[1].Retransmits != 22 {
		t.Errorf("Streams[1].Retransmits = %d, want 22", result.Streams[1].Retransmits)
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
