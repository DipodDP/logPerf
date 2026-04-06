//go:build !windows

package iperf

import (
	"os"
	"syscall"
)

func stopProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
