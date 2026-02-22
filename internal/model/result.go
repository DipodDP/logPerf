package model

import (
	"math"
	"time"
)

// PingResult holds parsed ping latency statistics.
type PingResult struct {
	PacketsSent int
	PacketsRecv int
	PacketLoss  float64
	MinMs       float64
	AvgMs       float64
	MaxMs       float64
}

// IntervalResult holds a single interval measurement from an iperf3 test.
type IntervalResult struct {
	TimeStart    float64 // seconds from test start
	TimeEnd      float64
	Bytes        int64
	BandwidthBps float64
	Retransmits  int
	Packets      int
	LostPackets  int     // UDP only
	LostPercent  float64 // UDP only
	JitterMs     float64 // UDP only
	Omitted      bool
	StreamID     int // iperf3 stream/socket ID; 0 = aggregate/unknown
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
	Socket      int // iperf3 socket ID; used for server-side data matching
	SentBps     float64
	ReceivedBps float64
	Retransmits int
	JitterMs    float64
	LostPackets int
	LostPercent float64
	Packets     int
	Sender      bool // true = forward/TX stream, false = reverse/RX stream (bidir mode)
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
	Timestamp     time.Time
	ServerAddr    string
	Port          int
	Parallel      int
	Duration      int
	BlockSize     int // -l buffer/datagram size in bytes; 0 = iperf3 default
	Interval      int
	Protocol      string
	MeasurementID string // e.g. "20260218-163958-01"; empty = not set
	SSHRemoteHost string // remote SSH host if used; empty = local
	IperfVersion  string // e.g. "3.17"
	Mode          string // "CLI" or "GUI"
	LocalHostname string // os.Hostname() at test time
	LocalIP       string // primary outbound IP at test time; empty = unknown
	SentBps       float64
	ReceivedBps   float64
	Retransmits   int
	JitterMs      float64
	FwdJitterMs   float64 // fwd jitter measured by server (--get-server-output); 0 if unavailable
	LostPackets   int
	LostPercent   float64
	Packets       int
	BytesSent     int64  // total bytes sent
	BytesReceived int64  // total bytes received
	Direction     string // "Reverse", "Bidirectional", or "" (normal)
	Bandwidth            string // target bandwidth setting used
	Congestion           string // congestion algorithm used
	ReverseSentBps       float64 // bidir reverse: sent bps
	ReverseReceivedBps   float64 // bidir reverse: received bps
	ReverseRetransmits   int     // bidir reverse: retransmits
	ReverseBytesSent     int64   // bidir reverse: bytes sent
	ReverseBytesReceived int64   // bidir reverse: bytes received
	ReverseLostPackets   int     // bidir reverse: UDP lost packets
	ReverseLostPercent   float64 // bidir reverse: UDP lost percent
	ReversePackets       int     // bidir reverse: UDP total packets
	ReverseJitterMs      float64 // bidir reverse: jitter (client-measured)
	FwdReceivedBps       float64 // bidir fwd: bandwidth received by server (server-measured); 0 if unavailable
	FwdLostPackets       int     // bidir fwd: UDP lost packets (server-measured)
	FwdLostPercent       float64 // bidir fwd: UDP lost percent (server-measured)
	FwdPackets           int     // bidir fwd: UDP total packets (server-measured)
	ActualDuration       float64         // measured duration from last interval (seconds)
	Streams              []StreamResult
	Intervals            []IntervalResult // forward / single-direction intervals
	ReverseIntervals     []IntervalResult // bidir reverse-direction intervals (empty if not bidir)
	PingBaseline         *PingResult
	PingLoaded           *PingResult
	Error                string
	Interrupted          bool // true if test was stopped by user before natural completion
}

// SentMbps returns the sent throughput in Mbps.
func (r *TestResult) SentMbps() float64 {
	return r.SentBps / 1_000_000
}

// ReceivedMbps returns the received throughput in Mbps.
func (r *TestResult) ReceivedMbps() float64 {
	return r.ReceivedBps / 1_000_000
}

// SentMB returns the total bytes sent in megabytes.
func (r *TestResult) SentMB() float64 {
	return float64(r.BytesSent) / 1_000_000
}

// ReceivedMB returns the total bytes received in megabytes.
func (r *TestResult) ReceivedMB() float64 {
	return float64(r.BytesReceived) / 1_000_000
}

// TotalFwdMB returns the forward transfer in megabytes as received by the server.
// Prefers BytesReceived (server-measured) and falls back to BytesSent when
// server output was unavailable.
func (r *TestResult) TotalFwdMB() float64 {
	if r.BytesReceived > 0 {
		return float64(r.BytesReceived) / 1_000_000
	}
	return float64(r.BytesSent) / 1_000_000
}

// TotalRevMB returns the best available reverse transfer in megabytes.
// Prefers ReverseBytesReceived (client-side received) and falls back to
// ReverseBytesSent when the receiver count is zero (e.g. graceful stop
// zeroes out one side).
func (r *TestResult) TotalRevMB() float64 {
	if r.ReverseBytesReceived > 0 {
		return float64(r.ReverseBytesReceived) / 1_000_000
	}
	return float64(r.ReverseBytesSent) / 1_000_000
}

// ReverseSentMbps returns the reverse-direction sent throughput in Mbps.
func (r *TestResult) ReverseSentMbps() float64 {
	return r.ReverseSentBps / 1_000_000
}

// ReverseReceivedMbps returns the reverse-direction received throughput in Mbps.
func (r *TestResult) ReverseReceivedMbps() float64 {
	return r.ReverseReceivedBps / 1_000_000
}

// FwdActualMbps returns the best available forward throughput in Mbps.
// Prefers FwdReceivedBps (server-measured, reflects actual delivery) over
// SentBps (client-side sender rate). Falls back to SentBps when server
// output was not available.
func (r *TestResult) FwdActualMbps() float64 {
	if r.FwdReceivedBps > 0 {
		return r.FwdReceivedBps / 1_000_000
	}
	return r.SentBps / 1_000_000
}

// ActualJitterMs returns the best available forward jitter in ms.
// In UDP bidir mode, prefers FwdJitterMs (server-measured via --get-server-output)
// over JitterMs (which is 0 for the sender side).
func (r *TestResult) ActualJitterMs() float64 {
	if r.FwdJitterMs > 0 {
		return r.FwdJitterMs
	}
	return r.JitterMs
}

// ReverseActualMbps returns the best available reverse throughput in Mbps.
// Prefers ReverseReceivedBps (receiver-side, reflects actual delivery)
// over ReverseSentBps (sender-side, doesn't account for UDP packet loss).
func (r *TestResult) ReverseActualMbps() float64 {
	if r.ReverseReceivedBps > 0 {
		return r.ReverseReceivedBps / 1_000_000
	}
	return r.ReverseSentBps / 1_000_000
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
	// In bidir mode only sum Fwd streams (Sender=true); Rev streams are the
	// reverse direction and should not be included in the forward total.
	if r.Protocol == "UDP" {
		isBidirUDP := r.Direction == "Bidirectional"
		var sentSum float64
		for _, s := range r.Streams {
			if isBidirUDP && !s.Sender {
				continue
			}
			sentSum += s.SentBps
		}
		sentOK = r.SentBps == 0 || math.Abs(sentSum-r.SentBps)/r.SentBps <= tolerance
		return sentOK, true
	}

	// In bidir mode, only sum forward streams (Sender=true) for verification.
	isBidir := r.Direction == "Bidirectional"

	var sentSum, recvSum float64
	for _, s := range r.Streams {
		if isBidir && !s.Sender {
			continue // skip reverse streams for forward summary verification
		}
		sentSum += s.SentBps
		recvSum += s.ReceivedBps
	}
	sentOK = r.SentBps == 0 || math.Abs(sentSum-r.SentBps)/r.SentBps <= tolerance
	recvOK = r.ReceivedBps == 0 || math.Abs(recvSum-r.ReceivedBps)/r.ReceivedBps <= tolerance
	return sentOK, recvOK
}
