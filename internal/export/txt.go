package export

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"iperf-tool/internal/format"
	"iperf-tool/internal/model"
)

const (
	divider  = "==========================================================================================" // 90 chars
	sectionDash = "------------------------------------------------------------------------------------------" // 90 chars
)

// WriteTXT appends structured human-readable test result blocks to path.
// If the file does not exist it is created; if it exists the new block is
// appended (series logging).
func WriteTXT(path string, results []model.TestResult) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open txt file: %w", err)
	}
	defer f.Close()

	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(f)
		}
		writeBlock(f, &r)
	}
	return nil
}

type lineWriter interface {
	WriteString(s string) (int, error)
}

func writeln(w lineWriter, s string) {
	w.WriteString(s + "\n") //nolint:errcheck
}

func writeBlock(w lineWriter, r *model.TestResult) {
	writeln(w, divider)

	// Measurement ID (optional)
	if r.MeasurementID != "" {
		writeln(w, fmt.Sprintf("Measurement ID: %s", r.MeasurementID))
	}

	// --- Header: date, env ---
	local := r.Timestamp.Local()
	tzName, offset := local.Zone()
	offsetHours := offset / 3600
	offsetMins := (offset % 3600) / 60
	var utcStr string
	if offsetMins != 0 {
		utcStr = fmt.Sprintf("UTC%+03d:%02d", offsetHours, abs(offsetMins))
	} else {
		utcStr = fmt.Sprintf("UTC%+03d:00", offsetHours)
	}

	writeln(w, fmt.Sprintf("Date:            %s", local.Format("02.01.2006")))
	writeln(w, fmt.Sprintf("Time:            %s", local.Format("15:04:05")))
	writeln(w, fmt.Sprintf("Timezone:        %s (%s)", tzName, utcStr))
	writeln(w, fmt.Sprintf("RFC3339:         %s", r.Timestamp.Format("2006-01-02T15:04:05Z07:00")))
	writeln(w, "")

	if r.LocalHostname != "" {
		writeln(w, fmt.Sprintf("Hostname:        %s", r.LocalHostname))
	}
	osLabel := runtime.GOOS
	if osLabel == "darwin" {
		osLabel = "darwin (macOS)"
	}
	writeln(w, fmt.Sprintf("OS:              %s", osLabel))
	if r.LocalIP != "" {
		writeln(w, fmt.Sprintf("Local IP:        %s", r.LocalIP))
	}
	if r.IperfVersion != "" {
		writeln(w, fmt.Sprintf("iperf3 version:  %s", r.IperfVersion))
	}
	if r.Mode != "" {
		writeln(w, fmt.Sprintf("Mode:            %s", r.Mode))
	}
	jsonMode := "standard JSON"
	if len(r.Intervals) > 0 {
		jsonMode = "JSON-stream"
	}
	writeln(w, fmt.Sprintf("JSON mode:       %s", jsonMode))
	if r.SSHRemoteHost != "" {
		writeln(w, fmt.Sprintf("Remote host:     %s", r.SSHRemoteHost))
	}
	writeln(w, "")

	// --- Test Parameters ---
	writeln(w, "--- Test Parameters ---")
	writeln(w, fmt.Sprintf("Server:          %s:%d", r.ServerAddr, r.Port))
	writeln(w, fmt.Sprintf("Protocol:        %s", r.Protocol))

	dir := r.Direction
	switch dir {
	case "Bidirectional":
		dir = "Bidirectional (--bidir)"
	case "Reverse":
		dir = "Reverse (-R)"
	case "":
		dir = "Normal"
	}
	writeln(w, fmt.Sprintf("Direction:       %s", dir))
	writeln(w, fmt.Sprintf("Parallel:        %d streams", r.Parallel))
	writeln(w, fmt.Sprintf("Requested time:  %d seconds", r.Duration))
	if r.Bandwidth != "" {
		writeln(w, fmt.Sprintf("Bandwidth limit: %s Mbps/stream", r.Bandwidth))
	}
	if r.Congestion != "" {
		writeln(w, fmt.Sprintf("Congestion:      %s", r.Congestion))
	}
	writeln(w, "")

	// Error case — short circuit
	if r.Error != "" {
		writeln(w, sectionDash)
		writeln(w, "Summary")
		writeln(w, sectionDash)
		writeln(w, "")
		writeln(w, fmt.Sprintf("Error: %s", r.Error))
		writeln(w, "")
		writeln(w, divider)
		writeln(w, "END OF MEASUREMENT")
		writeln(w, divider)
		writeln(w, "")
		return
	}

	// --- Results table ---
	writeResultsTable(w, r)

	// --- Summary section ---
	writeSummarySection(w, r)

	// --- Per-Stream Average Bandwidth ---
	writeStreamSection(w, r)

	// --- Latency + END OF MEASUREMENT ---
	writeLatencySection(w, r)
}

// writeResultsTable writes the Results table with sectionDash dividers.
func writeResultsTable(w lineWriter, r *model.TestResult) {
	if len(r.Intervals) == 0 {
		return
	}

	writeln(w, sectionDash)
	writeln(w, "Client-Side Results")
	writeln(w, sectionDash)
	writeln(w, "")

	isBidir := r.Direction == "Bidirectional"
	isUDP := r.Protocol == "UDP"

	if isBidir {
		writeln(w, "Timestamp                  "+format.FormatBidirIntervalHeader(isUDP))
		for i, iv := range r.Intervals {
			wallTime := r.Timestamp.Local().Add(time.Duration(iv.TimeStart * float64(time.Second)))
			ts := wallTime.Format("02.01.2006 15:04:05")
			var rev *model.IntervalResult
			if i < len(r.ReverseIntervals) {
				rv := r.ReverseIntervals[i]
				rev = &rv
			}
			writeln(w, fmt.Sprintf("%-26s %s", ts, format.FormatBidirInterval(&iv, rev, isUDP)))
		}
	} else if isUDP {
		writeln(w, "Timestamp                  Mbps       MB         Packets   Lost   Loss%    Jitter")
		for _, iv := range r.Intervals {
			wallTime := r.Timestamp.Local().Add(time.Duration(iv.TimeStart * float64(time.Second)))
			ts := wallTime.Format("02.01.2006 15:04:05")
			writeln(w, fmt.Sprintf("%-26s %-10.2f %-10.2f %-9d %-6d %-8.2f %.3f ms",
				ts, iv.BandwidthMbps(), iv.TransferMB(),
				iv.Packets, iv.LostPackets, iv.LostPercent, iv.JitterMs))
		}
	} else {
		// Normal / Reverse TCP
		writeln(w, "Timestamp                  Mbps       MB         Retr")
		for _, iv := range r.Intervals {
			wallTime := r.Timestamp.Local().Add(time.Duration(iv.TimeStart * float64(time.Second)))
			ts := wallTime.Format("02.01.2006 15:04:05")
			writeln(w, fmt.Sprintf("%-26s %-10.2f %-10.2f %d",
				ts, iv.BandwidthMbps(), iv.TransferMB(), iv.Retransmits))
		}
	}

	writeln(w, "")
}

// writeSummarySection writes the Summary block with sectionDash dividers.
// The content mirrors FormatResult's --- Summary --- section for consistency.
func writeSummarySection(w lineWriter, r *model.TestResult) {
	writeln(w, sectionDash)
	writeln(w, "Summary")
	writeln(w, sectionDash)
	writeln(w, "")

	isBidir := r.Direction == "Bidirectional"
	isUDP := r.Protocol == "UDP"
	hasReceiver := r.ReceivedBps > 0

	actualDur := r.ActualDuration
	if actualDur == 0 && len(r.Intervals) > 0 {
		for i := len(r.Intervals) - 1; i >= 0; i-- {
			if !r.Intervals[i].Omitted {
				actualDur = r.Intervals[i].TimeEnd
				break
			}
		}
	}

	if isBidir {
		revMbps := r.ReverseActualMbps()
		revRetrans := r.ReverseRetransmits
		if revMbps == 0 && r.ReceivedBps > 0 {
			revMbps = r.ReceivedMbps()
		}
		if isUDP {
			writeln(w, fmt.Sprintf("Client Send:     %.2f Mbps", r.SentMbps()))
			if r.FwdReceivedBps > 0 {
				writeln(w, fmt.Sprintf("Server Recv:     %.2f Mbps", r.FwdActualMbps()))
			} else {
				writeln(w, "Server Recv:     N/A")
			}
			if r.Interrupted && r.ReverseSentBps == 0 {
				writeln(w, "Server Send:     N/A")
			} else {
				writeln(w, fmt.Sprintf("Server Send:     %.2f Mbps", r.ReverseSentMbps()))
			}
			if revRecv := r.ReverseActualMbps(); revRecv > 0 {
				writeln(w, fmt.Sprintf("Client Recv:     %.2f Mbps", revRecv))
			}
			if r.ActualJitterMs() > 0 {
				writeln(w, fmt.Sprintf("C→S Jitter:      %.3f ms", r.ActualJitterMs()))
			}
			if r.FwdPackets > 0 {
				writeln(w, fmt.Sprintf("C→S Lost:        %d/%d (%.2f%%)", r.FwdLostPackets, r.FwdPackets, r.FwdLostPercent))
			}
			if r.ReverseJitterMs > 0 {
				writeln(w, fmt.Sprintf("S→C Jitter:      %.3f ms", r.ReverseJitterMs))
			}
			if r.ReversePackets > 0 {
				writeln(w, fmt.Sprintf("S→C Lost:        %d/%d (%.2f%%)", r.ReverseLostPackets, r.ReversePackets, r.ReverseLostPercent))
			}
		} else {
			writeln(w, fmt.Sprintf("Send:            %.2f Mbps (retransmits: %d)", r.FwdActualMbps(), r.Retransmits))
			writeln(w, fmt.Sprintf("Receive:         %.2f Mbps (retransmits: %d)", revMbps, revRetrans))
		}
		// C→S: client sent, server received
		csSent := float64(r.BytesSent) / 1e6
		csRecv := float64(r.BytesReceived) / 1e6
		if r.BytesReceived > 0 {
			writeln(w, fmt.Sprintf("C→S transferred: %.2f MB sent / %.2f MB received", csSent, csRecv))
		} else {
			writeln(w, fmt.Sprintf("C→S transferred: %.2f MB sent", csSent))
		}
		// S→C: server sent, client received
		scSent := float64(r.ReverseBytesSent) / 1e6
		scRecv := r.TotalRevMB()
		if r.ReverseBytesSent > 0 {
			writeln(w, fmt.Sprintf("S→C transferred: %.2f MB sent / %.2f MB received", scSent, scRecv))
		} else {
			writeln(w, fmt.Sprintf("S→C transferred: %.2f MB received", scRecv))
		}
	} else if isUDP {
		writeln(w, fmt.Sprintf("Sent:            %.2f Mbps", r.SentMbps()))
		if hasReceiver {
			writeln(w, fmt.Sprintf("Received:        %.2f Mbps", r.ReceivedMbps()))
		}
		writeln(w, fmt.Sprintf("Jitter:          %.3f ms", r.JitterMs))
		if r.FwdPackets > 0 {
			writeln(w, fmt.Sprintf("Packet Loss:     %d/%d (%.2f%%)", r.FwdLostPackets, r.FwdPackets, r.FwdLostPercent))
		} else {
			writeln(w, fmt.Sprintf("Packet Loss:     %d/%d (%.2f%%)", r.LostPackets, r.Packets, r.LostPercent))
		}
	} else if hasReceiver {
		writeln(w, fmt.Sprintf("Sent:            %.2f Mbps", r.SentMbps()))
		writeln(w, fmt.Sprintf("Received:        %.2f Mbps", r.ReceivedMbps()))
		writeln(w, fmt.Sprintf("Retransmits:     %d", r.Retransmits))
	} else {
		writeln(w, fmt.Sprintf("Bandwidth:       %.2f Mbps", r.SentMbps()))
		writeln(w, fmt.Sprintf("Retransmits:     %d", r.Retransmits))
	}

	if !isBidir && (r.BytesSent > 0 || r.BytesReceived > 0) {
		writeln(w, fmt.Sprintf("Transferred:     %.2f MB sent / %.2f MB received", r.SentMB(), r.ReceivedMB()))
	}

	sentOK, recvOK := r.VerifyStreamTotals()
	if !sentOK || !recvOK {
		writeln(w, "WARNING: Per-stream totals do not match summary values")
	}

	writeln(w, "")
	errStr := "none"
	if r.Error != "" {
		errStr = r.Error
	} else if r.Interrupted {
		errStr = "Interrupted"
	}
	writeln(w, fmt.Sprintf("Errors: %s", errStr))
	if actualDur > 0 {
		writeln(w, fmt.Sprintf("Actual duration: %.1f s", actualDur))
	}
	writeln(w, "")
}

// writeStreamSection writes the Per-Stream Results block.
// Omitted for single-stream tests. Mirrors FormatResult's per-stream output.
func writeStreamSection(w lineWriter, r *model.TestResult) {
	if len(r.Streams) <= 1 {
		return
	}

	writeln(w, sectionDash)
	writeln(w, "Per-Stream Results")
	writeln(w, sectionDash)
	writeln(w, "")

	isUDP := r.Protocol == "UDP"
	isBidir := r.Direction == "Bidirectional"
	hasReceiver := r.ReceivedBps > 0

	for _, s := range r.Streams {
		if isUDP && isBidir {
			if s.Sender {
				jitter := fmt.Sprintf("%.3f ms", s.JitterMs)
				if r.Interrupted && s.JitterMs == 0 {
					jitter = "N/A"
				}
				if s.Packets > 0 {
					writeln(w, fmt.Sprintf("Stream %d [Fwd]:  %.2f Mbps  Jitter: %s  Lost: %d/%d (%.2f%%)",
						s.ID, s.SentMbps(), jitter, s.LostPackets, s.Packets, s.LostPercent))
				} else {
					writeln(w, fmt.Sprintf("Stream %d [Fwd]:  %.2f Mbps  Jitter: %s",
						s.ID, s.SentMbps(), jitter))
				}
			} else {
				mbps := fmt.Sprintf("%.2f Mbps", s.SentMbps())
				if r.Interrupted && s.SentBps == 0 {
					mbps = "N/A"
				}
				writeln(w, fmt.Sprintf("Stream %d [Rev]:  %s  Jitter: %.3f ms  Lost: %d/%d (%.2f%%)",
					s.ID, mbps, s.JitterMs, s.LostPackets, s.Packets, s.LostPercent))
			}
		} else if isUDP {
			writeln(w, fmt.Sprintf("Stream %d:  %.2f Mbps  Jitter: %.3f ms  Lost: %d/%d (%.2f%%)",
				s.ID, s.SentMbps(), s.JitterMs, s.LostPackets, s.Packets, s.LostPercent))
		} else if isBidir {
			dir := "Rev"
			bps := s.ReceivedMbps()
			if s.Sender {
				dir = "Fwd"
				bps = s.SentMbps()
			}
			writeln(w, fmt.Sprintf("Stream %d [%s]:  %.2f Mbps", s.ID, dir, bps))
		} else if hasReceiver {
			writeln(w, fmt.Sprintf("Stream %d:  Sent: %.2f Mbps  Received: %.2f Mbps",
				s.ID, s.SentMbps(), s.ReceivedMbps()))
		} else {
			writeln(w, fmt.Sprintf("Stream %d:  %.2f Mbps", s.ID, s.SentMbps()))
		}
	}

	writeln(w, "")
}

// writeLatencySection writes the freestanding LATENCY ANALYSIS block and END OF MEASUREMENT.
func writeLatencySection(w lineWriter, r *model.TestResult) {
	if r.PingBaseline != nil || r.PingLoaded != nil {
		writeln(w, divider)
		writeln(w, "LATENCY ANALYSIS")
		writeln(w, divider)
		writeln(w, "")

		writeln(w, "Method:           ICMP ping")

		samples := 0
		if r.PingLoaded != nil {
			samples = r.PingLoaded.PacketsSent
		} else if r.PingBaseline != nil {
			samples = r.PingBaseline.PacketsSent
		}
		writeln(w, fmt.Sprintf("Samples:          %d", samples))
		writeln(w, fmt.Sprintf("Target:           %s", r.ServerAddr))
		writeln(w, "")

		if r.PingBaseline != nil {
			writeln(w, fmt.Sprintf("Baseline:         min/avg/max = %.2f / %.2f / %.2f ms",
				r.PingBaseline.MinMs, r.PingBaseline.AvgMs, r.PingBaseline.MaxMs))
		}
		if r.PingLoaded != nil {
			writeln(w, fmt.Sprintf("Under load:       min/avg/max = %.2f / %.2f / %.2f ms",
				r.PingLoaded.MinMs, r.PingLoaded.AvgMs, r.PingLoaded.MaxMs))
		}
		if r.PingBaseline != nil && r.PingLoaded != nil && r.PingBaseline.AvgMs > 0 {
			increase := r.PingLoaded.AvgMs - r.PingBaseline.AvgMs
			pct := increase / r.PingBaseline.AvgMs * 100
			writeln(w, fmt.Sprintf("Increase:         +%.2f ms (+%.1f%%)", increase, pct))
		}
		writeln(w, "")
	}

	writeln(w, divider)
	writeln(w, "END OF MEASUREMENT")
	writeln(w, divider)
	writeln(w, "")
}


func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// compile-time check that *strings.Builder satisfies lineWriter
var _ lineWriter = (*strings.Builder)(nil)
