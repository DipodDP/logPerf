package ping

import (
	"math"
	"testing"
)

const macOSOutput = `PING 192.168.1.1 (192.168.1.1): 56 data bytes
64 bytes from 192.168.1.1: icmp_seq=0 ttl=64 time=1.234 ms
64 bytes from 192.168.1.1: icmp_seq=1 ttl=64 time=1.456 ms
64 bytes from 192.168.1.1: icmp_seq=2 ttl=64 time=1.789 ms
64 bytes from 192.168.1.1: icmp_seq=3 ttl=64 time=2.012 ms

--- 192.168.1.1 ping statistics ---
4 packets transmitted, 4 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 1.234/1.623/2.012/0.295 ms
`

const linuxOutput = `PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
64 bytes from 10.0.0.1: icmp_seq=1 ttl=64 time=0.543 ms
64 bytes from 10.0.0.1: icmp_seq=2 ttl=64 time=0.621 ms
64 bytes from 10.0.0.1: icmp_seq=3 ttl=64 time=0.598 ms
64 bytes from 10.0.0.1: icmp_seq=4 ttl=64 time=0.612 ms

--- 10.0.0.1 ping statistics ---
4 packets transmitted, 4 received, 0% packet loss, time 3004ms
rtt min/avg/max/mdev = 0.543/0.594/0.621/0.029 ms
`

const partialLossOutput = `PING 192.168.1.1 (192.168.1.1): 56 data bytes
64 bytes from 192.168.1.1: icmp_seq=0 ttl=64 time=1.234 ms
64 bytes from 192.168.1.1: icmp_seq=2 ttl=64 time=3.456 ms

--- 192.168.1.1 ping statistics ---
4 packets transmitted, 2 packets received, 50.0% packet loss
round-trip min/avg/max/stddev = 1.234/2.345/3.456/1.111 ms
`

const totalLossOutput = `PING 192.168.1.1 (192.168.1.1): 56 data bytes

--- 192.168.1.1 ping statistics ---
4 packets transmitted, 0 packets received, 100.0% packet loss
`

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestParseOutput_MacOS(t *testing.T) {
	r, err := ParseOutput(macOSOutput)
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
	if !almostEqual(r.MinMs, 1.234) {
		t.Errorf("MinMs = %f, want 1.234", r.MinMs)
	}
	if !almostEqual(r.AvgMs, 1.623) {
		t.Errorf("AvgMs = %f, want 1.623", r.AvgMs)
	}
	if !almostEqual(r.MaxMs, 2.012) {
		t.Errorf("MaxMs = %f, want 2.012", r.MaxMs)
	}
}

func TestParseOutput_Linux(t *testing.T) {
	r, err := ParseOutput(linuxOutput)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if r.PacketsSent != 4 {
		t.Errorf("PacketsSent = %d, want 4", r.PacketsSent)
	}
	if r.PacketsRecv != 4 {
		t.Errorf("PacketsRecv = %d, want 4", r.PacketsRecv)
	}
	if !almostEqual(r.MinMs, 0.543) {
		t.Errorf("MinMs = %f, want 0.543", r.MinMs)
	}
	if !almostEqual(r.AvgMs, 0.594) {
		t.Errorf("AvgMs = %f, want 0.594", r.AvgMs)
	}
	if !almostEqual(r.MaxMs, 0.621) {
		t.Errorf("MaxMs = %f, want 0.621", r.MaxMs)
	}
}

func TestParseOutput_PartialLoss(t *testing.T) {
	r, err := ParseOutput(partialLossOutput)
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

func TestParseOutput_TotalLoss(t *testing.T) {
	r, err := ParseOutput(totalLossOutput)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}

	if r.PacketsRecv != 0 {
		t.Errorf("PacketsRecv = %d, want 0", r.PacketsRecv)
	}
	if !almostEqual(r.PacketLoss, 100.0) {
		t.Errorf("PacketLoss = %f, want 100.0", r.PacketLoss)
	}
	// No RTT stats on 100% loss
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
