package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iperf-tool/internal/model"
)

func TestWriteTXT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")

	results := []model.TestResult{
		{
			Timestamp:   time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC),
			ServerAddr:  "192.168.1.1",
			Port:        5201,
			Protocol:    "TCP",
			Parallel:    1,
			Duration:    10,
			SentBps:     100_000_000,
			ReceivedBps: 95_000_000,
			Retransmits: 3,
		},
		{
			Timestamp:   time.Date(2026, 2, 13, 12, 1, 0, 0, time.UTC),
			ServerAddr:  "192.168.1.1",
			Port:        5201,
			Protocol:    "UDP",
			Parallel:    2,
			Duration:    10,
			SentBps:     50_000_000,
			ReceivedBps: 48_000_000,
			Retransmits: 0,
			Streams: []model.StreamResult{
				{ID: 1, SentBps: 25_000_000, ReceivedBps: 24_000_000},
				{ID: 2, SentBps: 25_000_000, ReceivedBps: 24_000_000},
			},
		},
	}

	if err := WriteTXT(path, results); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, "=== Test Results ===") {
		t.Error("missing header")
	}
	if !strings.Contains(content, "192.168.1.1:5201") {
		t.Error("missing server address")
	}
	if !strings.Contains(content, "100.00 Mbps") {
		t.Error("missing first result sent Mbps")
	}
	if !strings.Contains(content, "Stream 1:") {
		t.Error("missing per-stream data for second result")
	}
	// Should contain two result blocks
	if strings.Count(content, "=== Test Results ===") != 2 {
		t.Errorf("expected 2 result blocks, got %d", strings.Count(content, "=== Test Results ==="))
	}
}

func TestWriteTXTEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	if err := WriteTXT(path, nil); err != nil {
		t.Fatalf("WriteTXT() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	if len(data) != 1 || data[0] != '\n' {
		t.Errorf("expected single newline for empty results, got %d bytes", len(data))
	}
}
