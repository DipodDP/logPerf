package format

import (
	"fmt"
	"strings"

	"iperf-tool/internal/model"
)

// FormatIntervalHeader returns a header line for interval output.
func FormatIntervalHeader() string {
	return fmt.Sprintf("%-20s %12s %12s %12s", "Interval", "Bandwidth", "Transfer", "Retransmits")
}

// FormatInterval produces a single formatted line for an interval measurement.
func FormatInterval(r *model.IntervalResult) string {
	return fmt.Sprintf("[%5.1f-%5.1f sec]  %8.2f Mbps %8.2f MB   %d retransmits",
		r.TimeStart, r.TimeEnd, r.BandwidthMbps(), r.TransferMB(), r.Retransmits)
}

// FormatResult produces a human-readable formatted output of a test result.
func FormatResult(r *model.TestResult) string {
	var b strings.Builder

	b.WriteString("=== Test Results ===\n")
	b.WriteString(fmt.Sprintf("Timestamp:       %s\n", r.Timestamp.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Server:          %s:%d\n", r.ServerAddr, r.Port))
	b.WriteString(fmt.Sprintf("Protocol:        %s\n", r.Protocol))

	if r.Direction != "" {
		b.WriteString(fmt.Sprintf("Direction:       %s\n", r.Direction))
	}
	if r.Congestion != "" {
		b.WriteString(fmt.Sprintf("Congestion:      %s\n", r.Congestion))
	}
	if r.Bandwidth != "" {
		b.WriteString(fmt.Sprintf("Bandwidth Target: %s\n", r.Bandwidth))
	}

	if r.Parallel > 1 {
		b.WriteString(fmt.Sprintf("Parallel:        %d streams\n", r.Parallel))
	}

	b.WriteString(fmt.Sprintf("Duration:        %d seconds\n", r.Duration))

	if r.Error != "" {
		b.WriteString(fmt.Sprintf("\nError: %s\n", r.Error))
		b.WriteString("====================")
		return b.String()
	}

	isUDP := r.Protocol == "UDP"
	isBidir := r.Direction == "Bidirectional"

	hasReceiver := r.ReceivedBps > 0

	if len(r.Streams) > 1 {
		b.WriteString("\n--- Per-Stream Results ---\n")
		for _, s := range r.Streams {
			if isUDP {
				b.WriteString(fmt.Sprintf("Stream %d:  %.2f Mbps  Jitter: %.3f ms  Lost: %d/%d (%.2f%%)\n",
					s.ID, s.SentMbps(), s.JitterMs, s.LostPackets, s.Packets, s.LostPercent))
			} else if isBidir {
				dir := "RX"
				bps := s.ReceivedMbps()
				if s.Sender {
					dir = "TX"
					bps = s.SentMbps()
				}
				b.WriteString(fmt.Sprintf("Stream %d [%s]:  %.2f Mbps\n",
					s.ID, dir, bps))
			} else if hasReceiver {
				b.WriteString(fmt.Sprintf("Stream %d:  Sent: %.2f Mbps  Received: %.2f Mbps\n",
					s.ID, s.SentMbps(), s.ReceivedMbps()))
			} else {
				b.WriteString(fmt.Sprintf("Stream %d:  %.2f Mbps\n",
					s.ID, s.SentMbps()))
			}
		}
	}

	b.WriteString("\n--- Summary ---\n")
	if isUDP {
		b.WriteString(fmt.Sprintf("Sent:            %.2f Mbps\n", r.SentMbps()))
		b.WriteString(fmt.Sprintf("Jitter:          %.3f ms\n", r.JitterMs))
		b.WriteString(fmt.Sprintf("Packet Loss:     %d/%d (%.2f%%)\n", r.LostPackets, r.Packets, r.LostPercent))
	} else if isBidir {
		b.WriteString(fmt.Sprintf("Send:            %.2f Mbps (retransmits: %d)\n", r.SentMbps(), r.Retransmits))
		// Use explicit reverse summary if available, otherwise fall back to
		// ReceivedBps which in --json-stream bidir mode represents reverse throughput.
		revMbps := r.ReverseSentMbps()
		revRetrans := r.ReverseRetransmits
		if revMbps == 0 && r.ReceivedBps > 0 {
			revMbps = r.ReceivedMbps()
		}
		b.WriteString(fmt.Sprintf("Receive:         %.2f Mbps (retransmits: %d)\n", revMbps, revRetrans))
		totalSentMB := float64(r.BytesSent) / 1_000_000
		totalRecvMB := float64(r.ReverseBytesReceived) / 1_000_000
		if totalRecvMB == 0 {
			totalRecvMB = float64(r.BytesReceived) / 1_000_000
		}
		b.WriteString(fmt.Sprintf("Transferred:     %.2f MB sent / %.2f MB received\n", totalSentMB, totalRecvMB))
	} else if hasReceiver {
		b.WriteString(fmt.Sprintf("Sent:            %.2f Mbps\n", r.SentMbps()))
		b.WriteString(fmt.Sprintf("Received:        %.2f Mbps\n", r.ReceivedMbps()))
		b.WriteString(fmt.Sprintf("Retransmits:     %d\n", r.Retransmits))
	} else {
		b.WriteString(fmt.Sprintf("Bandwidth:       %.2f Mbps\n", r.SentMbps()))
		b.WriteString(fmt.Sprintf("Retransmits:     %d\n", r.Retransmits))
	}

	if !isBidir && (r.BytesSent > 0 || r.BytesReceived > 0) {
		b.WriteString(fmt.Sprintf("Transferred:     %.2f MB sent / %.2f MB received\n", r.SentMB(), r.ReceivedMB()))
	}

	sentOK, recvOK := r.VerifyStreamTotals()
	if !sentOK || !recvOK {
		b.WriteString("WARNING: Per-stream totals do not match summary values\n")
	}

	if r.PingBaseline != nil || r.PingLoaded != nil {
		b.WriteString("\n--- Latency ---\n")
		if r.PingBaseline != nil {
			b.WriteString(fmt.Sprintf("Baseline:    min/avg/max = %.2f / %.2f / %.2f ms\n",
				r.PingBaseline.MinMs, r.PingBaseline.AvgMs, r.PingBaseline.MaxMs))
		}
		if r.PingLoaded != nil {
			b.WriteString(fmt.Sprintf("Under load:  min/avg/max = %.2f / %.2f / %.2f ms\n",
				r.PingLoaded.MinMs, r.PingLoaded.AvgMs, r.PingLoaded.MaxMs))
		}
	}

	b.WriteString("====================")
	return b.String()
}
