//go:build windows

package iperf

import "os"

func stopProcess(p *os.Process) error {
	return p.Kill()
}
