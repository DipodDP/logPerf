package iperf

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"

	"iperf-tool/internal/model"
)

// requireIperf3 skips the test if iperf3 is not installed.
func requireIperf3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("iperf3"); err != nil {
		t.Skip("iperf3 not found in PATH; skipping integration test")
	}
}

// startIperf3Server launches an iperf3 server on the given port and returns
// a cleanup function that kills it.
func startIperf3Server(t *testing.T, port int) func() {
	t.Helper()
	cmd := exec.Command("iperf3", "-s", "-p", fmt.Sprintf("%d", port), "-1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start iperf3 server on port %d: %v", port, err)
	}
	// Give the server a moment to bind the port.
	time.Sleep(200 * time.Millisecond)
	return func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func TestBidirIntegration_TCPIntervalsPaired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	requireIperf3(t)

	const fwdPort = 15301
	const revPort = 15302

	// Start two iperf3 servers (RunBidir uses port and port+1).
	cleanFwd := startIperf3Server(t, fwdPort)
	defer cleanFwd()
	cleanRev := startIperf3Server(t, revPort)
	defer cleanRev()

	cfg := DefaultConfig()
	cfg.ServerAddr = "127.0.0.1"
	cfg.Port = fwdPort
	cfg.Duration = 3
	cfg.Interval = 1
	cfg.Bidir = true

	runner := NewRunner()

	var mu sync.Mutex
	type intervalPair struct {
		fwd *model.IntervalResult
		rev *model.IntervalResult
	}
	var pairs []intervalPair

	result, err := runner.RunBidir(context.Background(), cfg, func(fwd, rev *model.IntervalResult) {
		mu.Lock()
		defer mu.Unlock()
		pairs = append(pairs, intervalPair{fwd: fwd, rev: rev})
	})
	if err != nil {
		t.Fatalf("RunBidir failed: %v", err)
	}

	// Verify final result has bidirectional data.
	if result.Direction != "Bidirectional" {
		t.Errorf("expected Direction=Bidirectional, got %q", result.Direction)
	}
	if result.SentBps <= 0 {
		t.Error("expected forward SentBps > 0")
	}
	if result.ReverseSentBps <= 0 {
		t.Error("expected reverse SentBps > 0")
	}

	// Verify we got interval callbacks.
	mu.Lock()
	n := len(pairs)
	mu.Unlock()
	if n == 0 {
		t.Fatal("expected at least one interval callback, got none")
	}

	// Every callback should have BOTH fwd and rev non-nil.
	// The first forward interval is held until reverse data arrives,
	// so even the first displayed row should be paired.
	var pairedCount int
	for i, p := range pairs {
		if p.fwd == nil {
			t.Errorf("pair[%d]: fwd should never be nil in callback", i)
		}
		if p.rev == nil {
			t.Errorf("pair[%d]: rev should never be nil in callback (first fwd should be held until rev arrives)", i)
		}
		if p.fwd != nil && p.rev != nil {
			pairedCount++
		}
	}
	if pairedCount == 0 {
		t.Error("expected at least one interval with both fwd and rev non-nil; got all rev=nil")
	}

	// Verify paired intervals have non-zero bandwidth.
	for i, p := range pairs {
		if p.fwd != nil && p.fwd.BandwidthBps <= 0 {
			t.Errorf("pair[%d]: fwd bandwidth should be > 0", i)
		}
		if p.rev != nil && p.rev.BandwidthBps <= 0 {
			t.Errorf("pair[%d]: rev bandwidth should be > 0", i)
		}
	}

	t.Logf("Got %d interval callbacks, %d with both directions paired", n, pairedCount)
}

func TestBidirIntegration_UDPIntervalsPaired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	requireIperf3(t)

	const fwdPort = 15303
	const revPort = 15304

	cleanFwd := startIperf3Server(t, fwdPort)
	defer cleanFwd()
	cleanRev := startIperf3Server(t, revPort)
	defer cleanRev()

	cfg := DefaultConfig()
	cfg.ServerAddr = "127.0.0.1"
	cfg.Port = fwdPort
	cfg.Duration = 3
	cfg.Interval = 1
	cfg.Protocol = "udp"
	cfg.Bidir = true

	runner := NewRunner()

	var mu sync.Mutex
	type intervalPair struct {
		fwd *model.IntervalResult
		rev *model.IntervalResult
	}
	var pairs []intervalPair

	result, err := runner.RunBidir(context.Background(), cfg, func(fwd, rev *model.IntervalResult) {
		mu.Lock()
		defer mu.Unlock()
		pairs = append(pairs, intervalPair{fwd: fwd, rev: rev})
	})
	if err != nil {
		t.Fatalf("RunBidir UDP failed: %v", err)
	}

	if result.Direction != "Bidirectional" {
		t.Errorf("expected Direction=Bidirectional, got %q", result.Direction)
	}

	mu.Lock()
	n := len(pairs)
	mu.Unlock()
	if n == 0 {
		t.Fatal("expected at least one interval callback, got none")
	}

	var pairedCount int
	for i, p := range pairs {
		if p.fwd == nil {
			t.Errorf("pair[%d]: fwd should never be nil in callback", i)
		}
		if p.rev == nil {
			t.Errorf("pair[%d]: rev should never be nil in callback", i)
		}
		if p.fwd != nil && p.rev != nil {
			pairedCount++
		}
	}
	if pairedCount == 0 {
		t.Error("expected at least one interval with both fwd and rev non-nil; got all rev=nil")
	}

	t.Logf("Got %d interval callbacks, %d with both directions paired", n, pairedCount)
}

func TestBidirIntegration_StopEarly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	requireIperf3(t)

	const fwdPort = 15305
	const revPort = 15306

	cleanFwd := startIperf3Server(t, fwdPort)
	defer cleanFwd()
	cleanRev := startIperf3Server(t, revPort)
	defer cleanRev()

	cfg := DefaultConfig()
	cfg.ServerAddr = "127.0.0.1"
	cfg.Port = fwdPort
	cfg.Duration = 10
	cfg.Interval = 1
	cfg.Bidir = true

	runner := NewRunner()

	var callbackCount int
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		// We don't check error here — Stop() causes an expected error.
		_, _ = runner.RunBidir(context.Background(), cfg, func(fwd, rev *model.IntervalResult) {
			mu.Lock()
			callbackCount++
			mu.Unlock()
		})
	}()

	// Let it run for 2 seconds then stop.
	time.Sleep(2 * time.Second)
	runner.Stop()

	select {
	case <-done:
		// good, RunBidir returned
	case <-time.After(5 * time.Second):
		t.Fatal("RunBidir did not return after Stop()")
	}

	mu.Lock()
	n := callbackCount
	mu.Unlock()

	t.Logf("Received %d interval callbacks before Stop()", n)
	if n == 0 {
		t.Error("expected at least one interval callback before stop")
	}
}
