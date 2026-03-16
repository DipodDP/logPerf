package iperf

import (
	"context"
	"testing"
	"time"
)

type mockSSHClient struct {
	calls     []string
	responses map[string]string
	errors    map[string]error
}

func newMockSSH() *mockSSHClient {
	return &mockSSHClient{
		responses: map[string]string{},
		errors:    map[string]error{},
	}
}

func (m *mockSSHClient) RunCommand(cmd string) (string, error) {
	m.calls = append(m.calls, cmd)
	for k, err := range m.errors {
		if cmd == k {
			return "", err
		}
	}
	for k, v := range m.responses {
		if cmd == k {
			return v, nil
		}
	}
	return "", nil
}

func TestProbeUDPReachability_NilSSH(t *testing.T) {
	_, err := ProbeUDPReachability(context.Background(), nil, "127.0.0.1", 2*time.Second, false)
	if err == nil {
		t.Error("expected error for nil SSH client")
	}
}

func TestProbeUDPReachability_EmptyAddr(t *testing.T) {
	mock := newMockSSH()
	_, err := ProbeUDPReachability(context.Background(), mock, "", 2*time.Second, false)
	if err == nil {
		t.Error("expected error for empty local address")
	}
}
