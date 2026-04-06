//go:build windows

package ping

import (
	"math"
	"testing"
)

const windowsOutput = `
Pinging 192.168.1.1 with 32 bytes of data:
Reply from 192.168.1.1: bytes=32 time=1ms TTL=64
Reply from 192.168.1.1: bytes=32 time=2ms TTL=64
Reply from 192.168.1.1: bytes=32 time=1ms TTL=64
Reply from 192.168.1.1: bytes=32 time=3ms TTL=64

Ping statistics for 192.168.1.1:
    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 1ms, Maximum = 3ms, Average = 1ms
`

const windowsPartialLossOutput = `
Pinging 192.168.1.1 with 32 bytes of data:
Reply from 192.168.1.1: bytes=32 time=1ms TTL=64
Request timed out.
Reply from 192.168.1.1: bytes=32 time=3ms TTL=64
Request timed out.

Ping statistics for 192.168.1.1:
    Packets: Sent = 4, Received = 2, Lost = 2 (50% loss),
Approximate round trip times in milli-seconds:
    Minimum = 1ms, Maximum = 3ms, Average = 2ms
`

const windowsTotalLossOutput = `
Pinging 192.168.1.1 with 32 bytes of data:
Request timed out.
Request timed out.
Request timed out.
Request timed out.

Ping statistics for 192.168.1.1:
    Packets: Sent = 4, Received = 0, Lost = 4 (100% loss),
`

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestParseOutput_Windows(t *testing.T) {
	r, err := ParseOutput(windowsOutput)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if r.PacketsSent != 4 {
		t.Errorf("PacketsSent = %d, want 4", r.PacketsSent)
	}
	if r.PacketsRecv != 4 {
		t.Errorf("PacketsRecv = %d, want 4", r.PacketsRecv)
	}
	if !almostEqual(r.PacketLoss, 0.0) {
		t.Errorf("PacketLoss = %f, want 0.0", r.PacketLoss)
	}
	if !almostEqual(r.MinMs, 1.0) {
		t.Errorf("MinMs = %f, want 1.0", r.MinMs)
	}
	if !almostEqual(r.MaxMs, 3.0) {
		t.Errorf("MaxMs = %f, want 3.0", r.MaxMs)
	}
	if !almostEqual(r.AvgMs, 1.0) {
		t.Errorf("AvgMs = %f, want 1.0", r.AvgMs)
	}
}

func TestParseOutput_WindowsPartialLoss(t *testing.T) {
	r, err := ParseOutput(windowsPartialLossOutput)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if r.PacketsSent != 4 {
		t.Errorf("PacketsSent = %d, want 4", r.PacketsSent)
	}
	if r.PacketsRecv != 2 {
		t.Errorf("PacketsRecv = %d, want 2", r.PacketsRecv)
	}
	if !almostEqual(r.PacketLoss, 50.0) {
		t.Errorf("PacketLoss = %f, want 50.0", r.PacketLoss)
	}
}

func TestParseOutput_WindowsTotalLoss(t *testing.T) {
	r, err := ParseOutput(windowsTotalLossOutput)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if r.PacketsRecv != 0 {
		t.Errorf("PacketsRecv = %d, want 0", r.PacketsRecv)
	}
	if !almostEqual(r.PacketLoss, 100.0) {
		t.Errorf("PacketLoss = %f, want 100.0", r.PacketLoss)
	}
	if r.MinMs != 0 || r.AvgMs != 0 || r.MaxMs != 0 {
		t.Errorf("expected zero RTT on 100%% loss, got min=%f avg=%f max=%f", r.MinMs, r.AvgMs, r.MaxMs)
	}
}

func TestParseOutput_InvalidInput(t *testing.T) {
	_, err := ParseOutput("garbage data")
	if err == nil {
		t.Error("expected error for invalid ping output")
	}
}
