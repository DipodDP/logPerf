//go:build windows

package ping

import "os"

func sigInterrupt() os.Signal {
	return os.Interrupt
}
