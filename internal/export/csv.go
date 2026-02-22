package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"iperf-tool/internal/model"
)

var csvHeaders = []string{
	"date",
	"time",
	"measurement_id",
	"hostname",
	"local_ip",
	"server",
	"port",
	"test_duration",
	"actual_duration",
	"streams",
	"protocol",
	"direction",
	"block_size",
	"stream_bandwidth",
	"congestion",
	"mode",
	"iperf_version",
	"fwd_mbps",
	"fwd_mb",
	"rev_mbps",
	"rev_mb",
	"fwd_retransmits",
	"rev_retransmits",
	"fwd_jitter_ms",
	"fwd_lost_packets",
	"fwd_lost_percent",
	"fwd_packets",
	"rev_jitter_ms",
	"rev_lost_packets",
	"rev_lost_percent",
	"rev_packets",
	"ping_baseline_min_ms",
	"ping_baseline_avg_ms",
	"ping_baseline_max_ms",
	"ping_loaded_min_ms",
	"ping_loaded_avg_ms",
	"ping_loaded_max_ms",
	"error",
}

// WriteCSV writes test results to a CSV file (semicolon-separated), creating
// it with headers if it doesn't exist, or appending rows if it does.
func WriteCSV(path string, results []model.TestResult) error {
	exists := fileExists(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open csv file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Comma = ';'
	defer w.Flush()

	if !exists {
		if err := w.Write(csvHeaders); err != nil {
			return fmt.Errorf("write csv headers: %w", err)
		}
	}

	for _, r := range results {
		// Ping fields
		var baselineMin, baselineAvg, baselineMax string
		var loadedMin, loadedAvg, loadedMax string
		if r.PingBaseline != nil {
			baselineMin = fmt.Sprintf("%.2f", r.PingBaseline.MinMs)
			baselineAvg = fmt.Sprintf("%.2f", r.PingBaseline.AvgMs)
			baselineMax = fmt.Sprintf("%.2f", r.PingBaseline.MaxMs)
		}
		if r.PingLoaded != nil {
			loadedMin = fmt.Sprintf("%.2f", r.PingLoaded.MinMs)
			loadedAvg = fmt.Sprintf("%.2f", r.PingLoaded.AvgMs)
			loadedMax = fmt.Sprintf("%.2f", r.PingLoaded.MaxMs)
		}

		// Actual duration: prefer r.ActualDuration; fall back to last non-omitted interval
		actualDur := r.ActualDuration
		if actualDur == 0 && len(r.Intervals) > 0 {
			for i := len(r.Intervals) - 1; i >= 0; i-- {
				if !r.Intervals[i].Omitted {
					actualDur = r.Intervals[i].TimeEnd
					break
				}
			}
		}
		actualDurStr := ""
		if actualDur > 0 {
			actualDurStr = fmt.Sprintf("%.1f", actualDur)
		}

		// Block size: empty when 0 (iperf3 default)
		blockSize := ""
		if r.BlockSize > 0 {
			blockSize = strconv.Itoa(r.BlockSize)
		}

		row := []string{
			r.Timestamp.Format("02.01.2006"),
			r.Timestamp.Format("15:04:05"),
			r.MeasurementID,
			r.LocalHostname,
			r.LocalIP,
			r.ServerAddr,
			strconv.Itoa(r.Port),
			strconv.Itoa(r.Duration),
			actualDurStr,
			strconv.Itoa(r.Parallel),
			r.Protocol,
			r.Direction,
			blockSize,
			r.Bandwidth,
			r.Congestion,
			r.Mode,
			r.IperfVersion,
			fwdMbpsCSV(r),
			fwdMbCSV(r),
			revMbpsCSV(r),
			fmt.Sprintf("%.2f", r.TotalRevMB()),
			strconv.Itoa(r.Retransmits),
			strconv.Itoa(r.ReverseRetransmits),
			fwdJitter(r),
			strconv.Itoa(r.LostPackets),
			fmt.Sprintf("%.2f", r.LostPercent),
			strconv.Itoa(r.Packets),
			fmt.Sprintf("%.3f", r.ReverseJitterMs),
			strconv.Itoa(r.ReverseLostPackets),
			fmt.Sprintf("%.2f", r.ReverseLostPercent),
			strconv.Itoa(r.ReversePackets),
			baselineMin,
			baselineAvg,
			baselineMax,
			loadedMin,
			loadedAvg,
			loadedMax,
			errorField(r),
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	return nil
}

// revMbpsCSV returns the rev_mbps CSV value.
// For non-bidir UDP, receiver rate lives in ReceivedBps rather than ReverseActualMbps.
// fwdMbpsCSV returns the fwd_mbps CSV value.
// For UDP when server output was unavailable, returns "N/A" instead of the client send rate.
func fwdMbpsCSV(r model.TestResult) string {
	if strings.EqualFold(r.Protocol, "UDP") && r.FwdReceivedBps == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", r.FwdActualMbps())
}

// fwdMbCSV returns the fwd_mb CSV value.
// For UDP when server output was unavailable, returns "N/A" instead of the client sent bytes.
func fwdMbCSV(r model.TestResult) string {
	if strings.EqualFold(r.Protocol, "UDP") && r.BytesReceived == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", r.TotalFwdMB())
}

func revMbpsCSV(r model.TestResult) string {
	if r.Direction != "Bidirectional" && strings.EqualFold(r.Protocol, "UDP") {
		return fmt.Sprintf("%.2f", r.ReceivedMbps())
	}
	return fmt.Sprintf("%.2f", r.ReverseActualMbps())
}

// fwdJitter returns the fwd_jitter_ms CSV value.
// For interrupted UDP bidir tests where server output was not received, returns "N/A".
func fwdJitter(r model.TestResult) string {
	if r.Interrupted && strings.EqualFold(r.Protocol, "UDP") && r.Direction == "Bidirectional" && r.FwdJitterMs == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.3f", r.ActualJitterMs())
}

// errorField returns the error CSV value.
// For interrupted tests with no other error, returns "Interrupted".
func errorField(r model.TestResult) string {
	if r.Error != "" {
		return r.Error
	}
	if r.Interrupted {
		return "Interrupted"
	}
	return ""
}

var intervalHeaders = []string{
	"measurement_id",
	"wall_time",
	"protocol",
	"streams",
	"test_direction",
	"block_size",
	"stream_bandwidth",
	"server",
	"port",
	"fwd_bandwidth_mbps",
	"fwd_transfer_mb",
	"fwd_retransmits",
	"fwd_packets",
	"fwd_omitted",
	"rev_bandwidth_mbps",
	"rev_transfer_mb",
	"rev_retransmits",
	"rev_packets",
	"rev_lost_packets",
	"rev_lost_percent",
	"rev_jitter_ms",
}

// WriteIntervalLog writes interval measurements to a CSV file (semicolon-separated).
// Each row is stamped with test parameters from result. The file is created (or
// truncated) on each call — it holds the intervals for a single test run.
// In bidirectional mode result.ReverseIntervals should be populated; pass an empty
// slice for normal/UDP mode.
func WriteIntervalLog(path string, result *model.TestResult) error {
	exists := fileExists(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open interval log: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Comma = ';'
	defer w.Flush()

	if !exists {
		if err := w.Write(intervalHeaders); err != nil {
			return fmt.Errorf("write interval headers: %w", err)
		}
	}

	blockSize := ""
	if result.BlockSize > 0 {
		blockSize = strconv.Itoa(result.BlockSize)
	}

	wallTime := result.Timestamp

	for i, iv := range result.Intervals {
		omitted := "0"
		if iv.Omitted {
			omitted = "1"
		}

		revBw, revMB, revRtr, revPkts, revLost, revLostPct, revJitter := "", "", "", "", "", "", ""
		if i < len(result.ReverseIntervals) {
			rev := result.ReverseIntervals[i]
			revBw = fmt.Sprintf("%.2f", rev.BandwidthMbps())
			revMB = fmt.Sprintf("%.2f", rev.TransferMB())
			revRtr = strconv.Itoa(rev.Retransmits)
			revPkts = strconv.Itoa(rev.Packets)
			revLost = strconv.Itoa(rev.LostPackets)
			revLostPct = fmt.Sprintf("%.2f", rev.LostPercent)
			revJitter = fmt.Sprintf("%.3f", rev.JitterMs)
		}

		row := []string{
			result.MeasurementID,
			wallTime.Add(time.Duration(iv.TimeStart * float64(time.Second))).Format("2006-01-02T15:04:05"),
			result.Protocol,
			strconv.Itoa(result.Parallel),
			result.Direction,
			blockSize,
			result.Bandwidth,
			result.ServerAddr,
			strconv.Itoa(result.Port),
			fmt.Sprintf("%.2f", iv.BandwidthMbps()),
			fmt.Sprintf("%.2f", iv.TransferMB()),
			strconv.Itoa(iv.Retransmits),
			strconv.Itoa(iv.Packets),
			omitted,
			revBw, revMB, revRtr, revPkts, revLost, revLostPct, revJitter,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write interval row: %w", err)
		}
	}

	return nil
}
