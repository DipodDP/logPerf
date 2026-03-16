package iperf

import (
	"context"
	"fmt"
	"net"
	"time"
)

// ProbeUDPReachability tests whether inbound UDP packets from the remote host
// can reach the local machine. This detects NAT that would block the iperf2
// Server Report ACK.
//
// Returns:
//   - true, nil: inbound UDP is open (direct mode safe)
//   - false, nil: timeout — NAT likely blocking inbound UDP (use SSH fallback)
//   - false, err: probe failed (SSH error, etc.)
func ProbeUDPReachability(ctx context.Context, sshCli SSHClient, localAddr string, probeTimeout time.Duration, isWindows bool) (bool, error) {
	if sshCli == nil {
		return false, fmt.Errorf("SSH client required for UDP probe")
	}
	if localAddr == "" {
		return false, fmt.Errorf("local address required for UDP probe")
	}
	if probeTimeout == 0 {
		probeTimeout = 2 * time.Second
	}

	// Bind a local UDP socket on an ephemeral port
	conn, err := net.ListenPacket("udp4", localAddr+":0")
	if err != nil {
		return false, fmt.Errorf("bind local UDP socket: %w", err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port

	// Tell remote to send a single UDP packet to localAddr:port
	var remoteCmd string
	if isWindows {
		remoteCmd = fmt.Sprintf(
			`PowerShell -Command "$u=New-Object System.Net.Sockets.UdpClient; $b=[Text.Encoding]::ASCII.GetBytes('PROBE'); $u.Send($b,$b.Length,'%s',%d); $u.Close()"`,
			localAddr, port)
	} else {
		remoteCmd = fmt.Sprintf("echo -n PROBE | nc -u -w1 %s %d", localAddr, port)
	}

	// Set read deadline before sending the remote command
	conn.SetReadDeadline(time.Now().Add(probeTimeout))

	// Send the probe command via SSH (fire and forget — don't block on result)
	errCh := make(chan error, 1)
	go func() {
		_, err := sshCli.RunCommand(remoteCmd)
		errCh <- err
	}()

	// Wait for the probe packet
	buf := make([]byte, 64)
	n, _, readErr := conn.ReadFrom(buf)

	// Check SSH result
	select {
	case sshErr := <-errCh:
		if sshErr != nil && readErr != nil {
			return false, fmt.Errorf("SSH probe command failed: %w", sshErr)
		}
	default:
		// SSH still running, that's fine
	}

	if readErr != nil {
		// Timeout means NAT is blocking
		if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
			return false, nil
		}
		return false, fmt.Errorf("read probe packet: %w", readErr)
	}

	if n > 0 {
		return true, nil
	}
	return false, nil
}
