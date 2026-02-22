package format

import (
	"fmt"
	"strings"

	"iperf-tool/internal/model"
)

// FormatIntervalHeader returns a header line for interval output.
func FormatIntervalHeader(isUDP bool) string {
	if isUDP {
		return fmt.Sprintf("%-14s %-12s %s", "Mbps", "MB", "Packets")
	}
	return fmt.Sprintf("%-14s %-12s %s", "Bandwidth", "Transfer", "Retransmits")
}

// FormatInterval produces a single formatted line for an interval measurement.
func FormatInterval(r *model.IntervalResult, isUDP bool) string {
	if isUDP {
		return fmt.Sprintf("%-14s %-12s %d pkts",
			fmt.Sprintf("%.2f Mbps", r.BandwidthMbps()),
			fmt.Sprintf("%.2f MB", r.TransferMB()),
			r.Packets)
	}
	return fmt.Sprintf("%-14s %-12s %d retransmits",
		fmt.Sprintf("%.2f Mbps", r.BandwidthMbps()),
		fmt.Sprintf("%.2f MB", r.TransferMB()),
		r.Retransmits)
}

// FormatBidirIntervalHeader returns a header line for bidirectional interval output.
func FormatBidirIntervalHeader(isUDP bool) string {
	if isUDP {
		return fmt.Sprintf("%-12s %-12s %-10s %-10s %-12s %-21s",
			"Fwd Mbps", "Rev Mbps", "Fwd MB", "Rev MB",
			"Rev Jitter", "Rev Lost")
	}
	return fmt.Sprintf("%-12s %-12s %-10s %-10s %-10s %s",
		"Fwd Mbps", "Rev Mbps", "Fwd MB", "Rev MB", "Fwd Retr", "Rev Retr")
}

// FormatBidirInterval produces a single formatted line for a bidirectional interval.
// rev may be nil if the reverse interval is not yet available.
func FormatBidirInterval(fwd, rev *model.IntervalResult, isUDP bool) string {
	var revMbps, revMB float64
	revRetr := 0
	if rev != nil {
		revMbps = rev.BandwidthMbps()
		revMB = rev.TransferMB()
		revRetr = rev.Retransmits
	}
	if isUDP {
		var revJitter float64
		var revLost, revPkts int
		var revLostPct float64
		if rev != nil {
			revJitter = rev.JitterMs
			revLost = rev.LostPackets
			revPkts = rev.Packets
			revLostPct = rev.LostPercent
		}
		revLostStr := fmt.Sprintf("%d/%d (%.1f%%)", revLost, revPkts, revLostPct)
		return fmt.Sprintf("%-12.2f %-12.2f %-10.2f %-10.2f %-12.3f %-21s",
			fwd.BandwidthMbps(), revMbps, fwd.TransferMB(), revMB,
			revJitter, revLostStr)
	}
	return fmt.Sprintf("%-12.2f %-12.2f %-10.2f %-10.2f %-10d %d",
		fwd.BandwidthMbps(), revMbps,
		fwd.TransferMB(), revMB,
		fwd.Retransmits, revRetr)
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
		b.WriteString(fmt.Sprintf("Bandwidth Target: %s Mbps/stream\n", r.Bandwidth))
	}

	if r.Parallel > 1 {
		b.WriteString(fmt.Sprintf("Parallel:        %d streams\n", r.Parallel))
	}

	b.WriteString(fmt.Sprintf("Duration:        %d seconds\n", r.Duration))

	if r.Error != "" {
		b.WriteString(fmt.Sprintf("\nError: %s\n", r.Error))
		b.WriteString("=========================================================================================")
		return b.String()
	}

	isUDP := r.Protocol == "UDP"
	isBidir := r.Direction == "Bidirectional"

	hasReceiver := r.ReceivedBps > 0

	if len(r.Streams) > 1 {
		b.WriteString("\n--- Per-Stream Results ---\n")
		for _, s := range r.Streams {
			if isUDP && isBidir {
				if s.Sender {
					jitter := fmt.Sprintf("%.3f ms", s.JitterMs)
					if r.Interrupted && s.JitterMs == 0 {
						jitter = "N/A"
					}
					if s.Packets > 0 {
						b.WriteString(fmt.Sprintf("Stream %d [Fwd]:  %.2f Mbps  Jitter: %s  Lost: %d/%d (%.2f%%)\n",
							s.ID, s.SentMbps(), jitter, s.LostPackets, s.Packets, s.LostPercent))
					} else {
						b.WriteString(fmt.Sprintf("Stream %d [Fwd]:  %.2f Mbps  Jitter: %s\n",
							s.ID, s.SentMbps(), jitter))
					}
				} else {
					mbps := fmt.Sprintf("%.2f Mbps", s.SentMbps())
					if r.Interrupted && s.SentBps == 0 {
						mbps = "N/A"
					}
					b.WriteString(fmt.Sprintf("Stream %d [Rev]:  %s  Jitter: %.3f ms  Lost: %d/%d (%.2f%%)\n",
						s.ID, mbps, s.JitterMs, s.LostPackets, s.Packets, s.LostPercent))
				}
			} else if isUDP {
				b.WriteString(fmt.Sprintf("Stream %d:  %.2f Mbps  Jitter: %.3f ms  Lost: %d/%d (%.2f%%)\n",
					s.ID, s.SentMbps(), s.JitterMs, s.LostPackets, s.Packets, s.LostPercent))
			} else if isBidir {
				dir := "Rev"
				bps := s.ReceivedMbps()
				if s.Sender {
					dir = "Fwd"
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
	if isBidir {
		revMbps := r.ReverseActualMbps()
		revRetrans := r.ReverseRetransmits
		if revMbps == 0 && r.ReceivedBps > 0 {
			revMbps = r.ReceivedMbps()
		}
		if isUDP {
			b.WriteString(fmt.Sprintf("Client Send:     %.2f Mbps\n", r.SentMbps()))
			if r.FwdReceivedBps > 0 {
				b.WriteString(fmt.Sprintf("Server Recv:     %.2f Mbps\n", r.FwdActualMbps()))
			} else {
				b.WriteString("Server Recv:     N/A\n")
			}
			if r.Interrupted && r.ReverseSentBps == 0 {
				b.WriteString("Server Send:     N/A\n")
			} else {
				b.WriteString(fmt.Sprintf("Server Send:     %.2f Mbps\n", r.ReverseSentMbps()))
			}
			if revRecv := r.ReverseActualMbps(); revRecv > 0 {
				b.WriteString(fmt.Sprintf("Client Recv:     %.2f Mbps\n", revRecv))
			}
			if r.ActualJitterMs() > 0 {
				b.WriteString(fmt.Sprintf("C→S Jitter:      %.3f ms\n", r.ActualJitterMs()))
			}
			if r.FwdPackets > 0 {
				b.WriteString(fmt.Sprintf("C→S Lost:        %d/%d (%.2f%%)\n", r.FwdLostPackets, r.FwdPackets, r.FwdLostPercent))
			}
			if r.ReverseJitterMs > 0 {
				b.WriteString(fmt.Sprintf("S→C Jitter:      %.3f ms\n", r.ReverseJitterMs))
			}
			if r.ReversePackets > 0 {
				b.WriteString(fmt.Sprintf("S→C Lost:        %d/%d (%.2f%%)\n", r.ReverseLostPackets, r.ReversePackets, r.ReverseLostPercent))
			}
		} else {
			b.WriteString(fmt.Sprintf("Send:            %.2f Mbps (retransmits: %d)\n", r.FwdActualMbps(), r.Retransmits))
			b.WriteString(fmt.Sprintf("Receive:         %.2f Mbps (retransmits: %d)\n", revMbps, revRetrans))
		}
		b.WriteString(formatBidirTransferred(r))
	} else if isUDP {
		b.WriteString(fmt.Sprintf("Sent:            %.2f Mbps\n", r.SentMbps()))
		if hasReceiver {
			b.WriteString(fmt.Sprintf("Received:        %.2f Mbps\n", r.ReceivedMbps()))
		}
		b.WriteString(fmt.Sprintf("Jitter:          %.3f ms\n", r.JitterMs))
		if r.FwdPackets > 0 {
			b.WriteString(fmt.Sprintf("Packet Loss:     %d/%d (%.2f%%)\n", r.FwdLostPackets, r.FwdPackets, r.FwdLostPercent))
		} else {
			b.WriteString(fmt.Sprintf("Packet Loss:     %d/%d (%.2f%%)\n", r.LostPackets, r.Packets, r.LostPercent))
		}
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

	errStr := "none"
	if r.Error != "" {
		errStr = r.Error
	} else if r.Interrupted {
		errStr = "Interrupted"
	}
	b.WriteString(fmt.Sprintf("Errors:      %s\n", errStr))

	b.WriteString(strings.Repeat("=", 90))
	return b.String()
}

// formatBidirTransferred returns two lines showing per-direction byte counts for
// bidirectional tests. Each line shows sent/received for that direction; a side
// is omitted when its byte count is zero (e.g. server-output unavailable).
func formatBidirTransferred(r *model.TestResult) string {
	var b strings.Builder
	// C→S: client sent, server received
	csSent := float64(r.BytesSent) / 1e6
	csRecv := float64(r.BytesReceived) / 1e6
	if r.BytesReceived > 0 {
		b.WriteString(fmt.Sprintf("C→S transferred: %.2f MB sent / %.2f MB received\n", csSent, csRecv))
	} else {
		b.WriteString(fmt.Sprintf("C→S transferred: %.2f MB sent\n", csSent))
	}
	// S→C: server sent, client received
	scSent := float64(r.ReverseBytesSent) / 1e6
	scRecv := r.TotalRevMB() // prefers ReverseBytesReceived, falls back to ReverseBytesSent
	if r.ReverseBytesSent > 0 {
		b.WriteString(fmt.Sprintf("S→C transferred: %.2f MB sent / %.2f MB received\n", scSent, scRecv))
	} else {
		b.WriteString(fmt.Sprintf("S→C transferred: %.2f MB received\n", scRecv))
	}
	return b.String()
}
