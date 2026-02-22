package netutil

import "net"

// OutboundIP returns the machine's preferred outbound IP address by opening a
// UDP connection (no packet sent) to a public address. Returns "" on failure.
func OutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}
