package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"iperf-tool/internal/model"
)

var csvHeaders = []string{
	"Timestamp",
	"Server",
	"Port",
	"Parallel",
	"Duration",
	"Protocol",
	"Sent_Mbps",
	"Received_Mbps",
	"Retransmits",
	"Error",
}

// WriteCSV writes test results to a CSV file, creating it with headers if it
// doesn't exist, or appending rows if it does.
func WriteCSV(path string, results []model.TestResult) error {
	exists := fileExists(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open csv file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if !exists {
		if err := w.Write(csvHeaders); err != nil {
			return fmt.Errorf("write csv headers: %w", err)
		}
	}

	for _, r := range results {
		row := []string{
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.ServerAddr,
			strconv.Itoa(r.Port),
			strconv.Itoa(r.Parallel),
			strconv.Itoa(r.Duration),
			r.Protocol,
			fmt.Sprintf("%.2f", r.SentMbps()),
			fmt.Sprintf("%.2f", r.ReceivedMbps()),
			strconv.Itoa(r.Retransmits),
			r.Error,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
