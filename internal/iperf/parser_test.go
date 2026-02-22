package iperf

import (
	"math"
	"testing"

	"iperf-tool/internal/model"
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

func TestParseResultBytes(t *testing.T) {
	result, err := ParseResult([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseResult() error: %v", err)
	}
	if result.BytesSent != 1175000000 {
		t.Errorf("BytesSent = %d, want 1175000000", result.BytesSent)
	}
	if result.BytesReceived != 1170000000 {
		t.Errorf("BytesReceived = %d, want 1170000000", result.BytesReceived)
	}
}

func TestParseEndDataBytes(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleEndEvent))
	result, err := ParseEndData(ev.Data)
	if err != nil {
		t.Fatalf("ParseEndData() error: %v", err)
	}
	if result.BytesSent != 1175000000 {
		t.Errorf("BytesSent = %d, want 1175000000", result.BytesSent)
	}
	if result.BytesReceived != 1170000000 {
		t.Errorf("BytesReceived = %d, want 1170000000", result.BytesReceived)
	}
}

func TestParseResultInvalidJSON(t *testing.T) {
	_, err := ParseResult([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

const sampleIntervalEvent = `{"event":"interval","data":{"streams":[{"socket":5,"start":0,"end":1,"seconds":1,"bytes":117500000,"bits_per_second":940000000,"retransmits":3,"omitted":false}],"sum":{"start":0,"end":1,"seconds":1,"bytes":117500000,"bits_per_second":940000000,"retransmits":3,"omitted":false}}}`

func TestParseStreamEvent(t *testing.T) {
	ev, err := ParseStreamEvent([]byte(sampleIntervalEvent))
	if err != nil {
		t.Fatalf("ParseStreamEvent() error: %v", err)
	}
	if ev.Event != "interval" {
		t.Errorf("Event = %q, want %q", ev.Event, "interval")
	}
	if ev.Data == nil {
		t.Fatal("Data should not be nil")
	}
}

func TestParseStreamEventInvalid(t *testing.T) {
	_, err := ParseStreamEvent([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseStreamEventMissingEvent(t *testing.T) {
	_, err := ParseStreamEvent([]byte(`{"data":{}}`))
	if err == nil {
		t.Error("expected error for missing event field")
	}
}

func TestParseIntervalData(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleIntervalEvent))
	interval, rev, err := ParseIntervalData(ev.Data)
	if err != nil {
		t.Fatalf("ParseIntervalData() error: %v", err)
	}
	if rev != nil {
		t.Error("rev should be nil for non-bidir interval")
	}
	if interval.TimeStart != 0 {
		t.Errorf("TimeStart = %f, want 0", interval.TimeStart)
	}
	if interval.TimeEnd != 1 {
		t.Errorf("TimeEnd = %f, want 1", interval.TimeEnd)
	}
	if interval.Bytes != 117500000 {
		t.Errorf("Bytes = %d, want 117500000", interval.Bytes)
	}
	if math.Abs(interval.BandwidthBps-940000000) > 1 {
		t.Errorf("BandwidthBps = %f, want 940000000", interval.BandwidthBps)
	}
	if interval.Retransmits != 3 {
		t.Errorf("Retransmits = %d, want 3", interval.Retransmits)
	}
	if interval.Omitted {
		t.Error("Omitted should be false")
	}
	if math.Abs(interval.BandwidthMbps()-940.0) > 0.01 {
		t.Errorf("BandwidthMbps() = %f, want 940.0", interval.BandwidthMbps())
	}
}

const sampleStartEvent = `{"event":"start","data":{"connected":[{"socket":5,"local_host":"192.168.1.100","local_port":43210,"remote_host":"192.168.1.1","remote_port":5201}],"test_start":{"protocol":"TCP","num_streams":1,"duration":10},"timestamp":{"timesecs":1704110400}}}`

func TestParseStartData(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleStartEvent))
	result := &model.TestResult{}
	if err := ParseStartData(ev.Data, result); err != nil {
		t.Fatalf("ParseStartData() error: %v", err)
	}
	if result.ServerAddr != "192.168.1.1" {
		t.Errorf("ServerAddr = %q, want %q", result.ServerAddr, "192.168.1.1")
	}
	if result.Port != 5201 {
		t.Errorf("Port = %d, want 5201", result.Port)
	}
	if result.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want %q", result.Protocol, "TCP")
	}
	if result.Parallel != 1 {
		t.Errorf("Parallel = %d, want 1", result.Parallel)
	}
	if result.Duration != 10 {
		t.Errorf("Duration = %d, want 10", result.Duration)
	}
}

const sampleEndEvent = `{"event":"end","data":{"sum_sent":{"start":0,"end":10,"seconds":10,"bytes":1175000000,"bits_per_second":940000000,"retransmits":42,"sender":true},"sum_received":{"start":0,"end":10,"seconds":10,"bytes":1170000000,"bits_per_second":936000000,"sender":false},"streams":[{"sender":{"socket":5,"bits_per_second":940000000,"retransmits":42},"receiver":{"socket":5,"bits_per_second":936000000}}]}}`

func TestParseEndData(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleEndEvent))
	result, err := ParseEndData(ev.Data)
	if err != nil {
		t.Fatalf("ParseEndData() error: %v", err)
	}
	if math.Abs(result.SentBps-940000000) > 1 {
		t.Errorf("SentBps = %f, want 940000000", result.SentBps)
	}
	if math.Abs(result.ReceivedBps-936000000) > 1 {
		t.Errorf("ReceivedBps = %f, want 936000000", result.ReceivedBps)
	}
	if result.Retransmits != 42 {
		t.Errorf("Retransmits = %d, want 42", result.Retransmits)
	}
	if len(result.Streams) != 1 {
		t.Fatalf("Streams count = %d, want 1", len(result.Streams))
	}
}

const sampleUDPJSON = `{
	"start": {
		"connected": [{
			"socket": 5,
			"local_host": "127.0.0.1",
			"local_port": 43210,
			"remote_host": "127.0.0.1",
			"remote_port": 5201
		}],
		"test_start": {
			"protocol": "UDP",
			"num_streams": 1,
			"duration": 3,
			"blksize": 8192,
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
			"end": 3.0,
			"seconds": 3.0,
			"bytes": 393216,
			"bits_per_second": 1048576.0,
			"jitter_ms": 0,
			"lost_packets": 3,
			"packets": 48,
			"lost_percent": 6.25,
			"sender": true
		},
		"sum_received": {
			"start": 0,
			"end": 3.0,
			"seconds": 3.0,
			"bytes": 368640,
			"bits_per_second": 983040.0,
			"jitter_ms": 0.025,
			"sender": false
		},
		"streams": [
			{
				"udp": {
					"socket": 5,
					"bits_per_second": 1048576.0,
					"jitter_ms": 0.025,
					"lost_packets": 3,
					"packets": 48,
					"lost_percent": 6.25
				}
			}
		]
	}
}`

func TestParseResultUDP(t *testing.T) {
	result, err := ParseResult([]byte(sampleUDPJSON))
	if err != nil {
		t.Fatalf("ParseResult() error: %v", err)
	}

	if result.Protocol != "UDP" {
		t.Errorf("Protocol = %q, want %q", result.Protocol, "UDP")
	}
	if math.Abs(result.SentBps-1048576.0) > 1 {
		t.Errorf("SentBps = %f, want 1048576", result.SentBps)
	}
	if math.Abs(result.JitterMs-0.025) > 0.001 {
		t.Errorf("JitterMs = %f, want 0.025", result.JitterMs)
	}
	if result.LostPackets != 3 {
		t.Errorf("LostPackets = %d, want 3", result.LostPackets)
	}
	if result.Packets != 48 {
		t.Errorf("Packets = %d, want 48", result.Packets)
	}
	if math.Abs(result.LostPercent-6.25) > 0.01 {
		t.Errorf("LostPercent = %f, want 6.25", result.LostPercent)
	}

	// Check per-stream UDP data
	if len(result.Streams) != 1 {
		t.Fatalf("Streams count = %d, want 1", len(result.Streams))
	}
	s := result.Streams[0]
	if math.Abs(s.SentBps-1048576.0) > 1 {
		t.Errorf("Stream SentBps = %f, want 1048576", s.SentBps)
	}
	if math.Abs(s.JitterMs-0.025) > 0.001 {
		t.Errorf("Stream JitterMs = %f, want 0.025", s.JitterMs)
	}
	if s.LostPackets != 3 {
		t.Errorf("Stream LostPackets = %d, want 3", s.LostPackets)
	}
	if s.Packets != 48 {
		t.Errorf("Stream Packets = %d, want 48", s.Packets)
	}

	// VerifyStreamTotals should pass for UDP
	result.Protocol = "UDP"
	sentOK, recvOK := result.VerifyStreamTotals()
	if !sentOK {
		t.Error("VerifyStreamTotals sentOK should be true for UDP")
	}
	if !recvOK {
		t.Error("VerifyStreamTotals recvOK should be true for UDP")
	}
}

const sampleUDPEndEvent = `{"event":"end","data":{"sum_sent":{"start":0,"end":3,"seconds":3,"bytes":393216,"bits_per_second":1048576,"jitter_ms":0,"lost_packets":3,"packets":48,"lost_percent":6.25,"sender":true},"sum_received":{"start":0,"end":3,"seconds":3,"bytes":368640,"bits_per_second":983040,"jitter_ms":0.025,"sender":false},"streams":[{"udp":{"socket":5,"bits_per_second":1048576,"jitter_ms":0.025,"lost_packets":3,"packets":48,"lost_percent":6.25}}]}}`

func TestParseEndDataUDP(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleUDPEndEvent))
	result, err := ParseEndData(ev.Data)
	if err != nil {
		t.Fatalf("ParseEndData() error: %v", err)
	}
	if math.Abs(result.SentBps-1048576) > 1 {
		t.Errorf("SentBps = %f, want 1048576", result.SentBps)
	}
	if math.Abs(result.JitterMs-0.025) > 0.001 {
		t.Errorf("JitterMs = %f, want 0.025", result.JitterMs)
	}
	if result.LostPackets != 3 {
		t.Errorf("LostPackets = %d, want 3", result.LostPackets)
	}
	if result.Packets != 48 {
		t.Errorf("Packets = %d, want 48", result.Packets)
	}
	if len(result.Streams) != 1 {
		t.Fatalf("Streams count = %d, want 1", len(result.Streams))
	}
	s := result.Streams[0]
	if s.ReceivedBps != 0 {
		t.Errorf("Stream ReceivedBps = %f, want 0 (UDP has no receiver)", s.ReceivedBps)
	}
	if math.Abs(s.JitterMs-0.025) > 0.001 {
		t.Errorf("Stream JitterMs = %f, want 0.025", s.JitterMs)
	}
}

const sampleBidirJSON = `{
	"start": {
		"connected": [
			{"socket": 5, "local_host": "192.168.1.100", "local_port": 43210, "remote_host": "192.168.1.1", "remote_port": 5201},
			{"socket": 6, "local_host": "192.168.1.100", "local_port": 43211, "remote_host": "192.168.1.1", "remote_port": 5201}
		],
		"test_start": {
			"protocol": "TCP",
			"num_streams": 2,
			"duration": 10
		},
		"timestamp": {"timesecs": 1704110400}
	},
	"intervals": [],
	"end": {
		"sum_sent": {
			"bytes": 500000000,
			"bits_per_second": 400000000.0,
			"retransmits": 2,
			"sender": true
		},
		"sum_received": {
			"bytes": 495000000,
			"bits_per_second": 396000000.0,
			"sender": false
		},
		"sum_sent_bidir_reverse": {
			"bytes": 600000000,
			"bits_per_second": 480000000.0,
			"retransmits": 5,
			"sender": true
		},
		"sum_received_bidir_reverse": {
			"bytes": 590000000,
			"bits_per_second": 472000000.0,
			"sender": false
		},
		"streams": [
			{
				"sender": {"socket": 5, "bits_per_second": 200000000.0, "retransmits": 1, "sender": true},
				"receiver": {"socket": 5, "bits_per_second": 198000000.0}
			},
			{
				"sender": {"socket": 6, "bits_per_second": 200000000.0, "retransmits": 1, "sender": true},
				"receiver": {"socket": 6, "bits_per_second": 198000000.0}
			},
			{
				"sender": {"socket": 7, "bits_per_second": 240000000.0, "retransmits": 3, "sender": false},
				"receiver": {"socket": 7, "bits_per_second": 236000000.0}
			},
			{
				"sender": {"socket": 8, "bits_per_second": 240000000.0, "retransmits": 2, "sender": false},
				"receiver": {"socket": 8, "bits_per_second": 236000000.0}
			}
		]
	}
}`

func TestParseResultBidir(t *testing.T) {
	result, err := ParseResult([]byte(sampleBidirJSON))
	if err != nil {
		t.Fatalf("ParseResult() error: %v", err)
	}

	// Forward direction
	if math.Abs(result.SentBps-400000000.0) > 1 {
		t.Errorf("SentBps = %f, want 400000000", result.SentBps)
	}
	if math.Abs(result.ReceivedBps-396000000.0) > 1 {
		t.Errorf("ReceivedBps = %f, want 396000000", result.ReceivedBps)
	}
	if result.Retransmits != 2 {
		t.Errorf("Retransmits = %d, want 2", result.Retransmits)
	}
	if result.BytesSent != 500000000 {
		t.Errorf("BytesSent = %d, want 500000000", result.BytesSent)
	}

	// Reverse direction
	if math.Abs(result.ReverseSentBps-480000000.0) > 1 {
		t.Errorf("ReverseSentBps = %f, want 480000000", result.ReverseSentBps)
	}
	if math.Abs(result.ReverseReceivedBps-472000000.0) > 1 {
		t.Errorf("ReverseReceivedBps = %f, want 472000000", result.ReverseReceivedBps)
	}
	if result.ReverseRetransmits != 5 {
		t.Errorf("ReverseRetransmits = %d, want 5", result.ReverseRetransmits)
	}
	if result.ReverseBytesSent != 600000000 {
		t.Errorf("ReverseBytesSent = %d, want 600000000", result.ReverseBytesSent)
	}
	if result.ReverseBytesReceived != 590000000 {
		t.Errorf("ReverseBytesReceived = %d, want 590000000", result.ReverseBytesReceived)
	}

	// 4 streams: 2 TX (sender=true) + 2 RX (sender=false)
	if len(result.Streams) != 4 {
		t.Fatalf("Streams count = %d, want 4", len(result.Streams))
	}
	if !result.Streams[0].Sender {
		t.Error("Streams[0].Sender should be true (TX)")
	}
	if !result.Streams[1].Sender {
		t.Error("Streams[1].Sender should be true (TX)")
	}
	if result.Streams[2].Sender {
		t.Error("Streams[2].Sender should be false (RX)")
	}
	if result.Streams[3].Sender {
		t.Error("Streams[3].Sender should be false (RX)")
	}

	// Verify helper methods
	if math.Abs(result.ReverseSentMbps()-480.0) > 0.01 {
		t.Errorf("ReverseSentMbps() = %f, want 480.0", result.ReverseSentMbps())
	}
	if math.Abs(result.ReverseReceivedMbps()-472.0) > 0.01 {
		t.Errorf("ReverseReceivedMbps() = %f, want 472.0", result.ReverseReceivedMbps())
	}

	// VerifyStreamTotals in bidir mode should only check forward streams
	result.Direction = "Bidirectional"
	sentOK, recvOK := result.VerifyStreamTotals()
	if !sentOK {
		t.Error("VerifyStreamTotals sentOK should be true for bidir forward streams")
	}
	if !recvOK {
		t.Error("VerifyStreamTotals recvOK should be true for bidir forward streams")
	}
}

const sampleBidirEndEvent = `{"event":"end","data":{"sum_sent":{"bytes":500000000,"bits_per_second":400000000,"retransmits":2,"sender":true},"sum_received":{"bytes":495000000,"bits_per_second":396000000,"sender":false},"sum_sent_bidir_reverse":{"bytes":600000000,"bits_per_second":480000000,"retransmits":5,"sender":true},"sum_received_bidir_reverse":{"bytes":590000000,"bits_per_second":472000000,"sender":false},"streams":[{"sender":{"socket":5,"bits_per_second":400000000,"retransmits":2,"sender":true},"receiver":{"socket":5,"bits_per_second":396000000}},{"sender":{"socket":7,"bits_per_second":480000000,"retransmits":5,"sender":false},"receiver":{"socket":7,"bits_per_second":472000000}}]}}`

func TestParseEndDataBidir(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleBidirEndEvent))
	result, err := ParseEndData(ev.Data)
	if err != nil {
		t.Fatalf("ParseEndData() error: %v", err)
	}
	if math.Abs(result.ReverseSentBps-480000000) > 1 {
		t.Errorf("ReverseSentBps = %f, want 480000000", result.ReverseSentBps)
	}
	if result.ReverseRetransmits != 5 {
		t.Errorf("ReverseRetransmits = %d, want 5", result.ReverseRetransmits)
	}
	if result.ReverseBytesSent != 600000000 {
		t.Errorf("ReverseBytesSent = %d, want 600000000", result.ReverseBytesSent)
	}
	if len(result.Streams) != 2 {
		t.Fatalf("Streams count = %d, want 2", len(result.Streams))
	}
	if !result.Streams[0].Sender {
		t.Error("Streams[0].Sender should be true")
	}
	if result.Streams[1].Sender {
		t.Error("Streams[1].Sender should be false")
	}
}

// Simulates --json-stream bidir mode: no sum_sent_bidir_reverse, and reverse
// streams have sender.bits_per_second=0 but receiver.bits_per_second has data.
const sampleBidirStreamEndEvent = `{"event":"end","data":{"sum_sent":{"bytes":500000000,"bits_per_second":400000000,"retransmits":2,"sender":true},"sum_received":{"bytes":495000000,"bits_per_second":0,"sender":false},"streams":[{"sender":{"socket":5,"bits_per_second":200000000,"retransmits":1,"sender":true},"receiver":{"socket":5,"bits_per_second":198000000}},{"sender":{"socket":6,"bits_per_second":200000000,"retransmits":1,"sender":true},"receiver":{"socket":6,"bits_per_second":198000000}},{"sender":{"socket":7,"bits_per_second":0,"retransmits":0,"sender":false},"receiver":{"socket":7,"bits_per_second":240000000}},{"sender":{"socket":8,"bits_per_second":0,"retransmits":0,"sender":false},"receiver":{"socket":8,"bits_per_second":232000000}}]}}`

func TestParseEndDataBidirStreamFallback(t *testing.T) {
	ev, _ := ParseStreamEvent([]byte(sampleBidirStreamEndEvent))
	result, err := ParseEndData(ev.Data)
	if err != nil {
		t.Fatalf("ParseEndData() error: %v", err)
	}
	// ReverseSentBps should be computed from per-stream ReceivedBps
	// since sender.bits_per_second=0 for reverse streams.
	// Expected: 240000000 + 232000000 = 472000000
	if math.Abs(result.ReverseSentBps-472000000) > 1 {
		t.Errorf("ReverseSentBps = %f, want 472000000 (from per-stream receiver)", result.ReverseSentBps)
	}
	if len(result.Streams) != 4 {
		t.Fatalf("Streams count = %d, want 4", len(result.Streams))
	}
	// RX streams should have ReceivedBps set
	if math.Abs(result.Streams[2].ReceivedBps-240000000) > 1 {
		t.Errorf("Streams[2].ReceivedBps = %f, want 240000000", result.Streams[2].ReceivedBps)
	}
}

func TestParseIntervalDataOmitted(t *testing.T) {
	data := `{"streams":[],"sum":{"start":0,"end":1,"seconds":1,"bytes":0,"bits_per_second":0,"retransmits":0,"omitted":true}}`
	interval, _, err := ParseIntervalData([]byte(data))
	if err != nil {
		t.Fatalf("ParseIntervalData() error: %v", err)
	}
	if !interval.Omitted {
		t.Error("Omitted should be true")
	}
}

func TestParseIntervalDataBidir(t *testing.T) {
	data := `{"streams":[{"socket":5,"start":0,"end":1,"bytes":100000000,"bits_per_second":800000000,"sender":true},{"socket":7,"start":0,"end":1,"bytes":50000000,"bits_per_second":400000000,"sender":false}],"sum":{"start":0,"end":1,"bytes":100000000,"bits_per_second":800000000,"omitted":false,"sender":true},"sum_bidir_reverse":{"start":0,"end":1,"bytes":50000000,"bits_per_second":400000000,"omitted":false,"sender":false}}`
	fwd, rev, err := ParseIntervalData([]byte(data))
	if err != nil {
		t.Fatalf("ParseIntervalData() error: %v", err)
	}
	if math.Abs(fwd.BandwidthBps-800000000) > 1 {
		t.Errorf("fwd BandwidthBps = %f, want 800000000", fwd.BandwidthBps)
	}
	if rev == nil {
		t.Fatal("rev should not be nil for bidir interval")
	}
	if math.Abs(rev.BandwidthBps-400000000) > 1 {
		t.Errorf("rev BandwidthBps = %f, want 400000000", rev.BandwidthBps)
	}
	if rev.Bytes != 50000000 {
		t.Errorf("rev Bytes = %d, want 50000000", rev.Bytes)
	}
}

func TestParseServerOutputText_SingleStreamTCPBidir(t *testing.T) {
	// Single-stream bidir TCP: server output has no [SUM] line, only a per-stream [N][RX-S] line.
	serverText := `Accepted connection from 192.168.1.100, port 43210
[  5] local 192.168.1.1 port 5201 connected to 192.168.1.100 port 43210
[ ID][Role] Interval           Transfer     Bitrate         Retr  Cwnd
[  5][TX-S]   0.00-10.04  sec   212 MBytes   177 Mbits/sec    0   2.06 MBytes
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID][Role] Interval           Transfer     Bitrate
[  5][RX-S]  0.00-10.04  sec  101 MBytes   84.7 Mbits/sec                  receiver
- - - - - - - - - - - - - - - - - - - - - - - - -
iperf Done.`

	r := &model.TestResult{Protocol: "TCP"}
	ParseServerOutputText(serverText, r, true)

	want := 84.7e6
	if math.Abs(r.FwdReceivedBps-want) > 1e3 {
		t.Errorf("FwdReceivedBps = %f, want %f (per-stream fallback)", r.FwdReceivedBps, want)
	}
}

func TestParseServerOutputText_MultiStreamTCPBidir(t *testing.T) {
	// Multi-stream bidir TCP: server output has a [SUM][RX-S] line; per-stream lines must be ignored.
	serverText := `[ ID][Role] Interval           Transfer     Bitrate
[  5][RX-S]  0.00-10.00  sec  50 MBytes   42.0 Mbits/sec                  receiver
[  7][RX-S]  0.00-10.00  sec  50 MBytes   42.0 Mbits/sec                  receiver
[SUM][RX-S]  0.00-10.00  sec  100 MBytes  85.0 Mbits/sec                  receiver`

	r := &model.TestResult{Protocol: "TCP"}
	ParseServerOutputText(serverText, r, true)

	want := 85.0e6
	if math.Abs(r.FwdReceivedBps-want) > 1e3 {
		t.Errorf("FwdReceivedBps = %f, want %f (SUM line)", r.FwdReceivedBps, want)
	}
}
