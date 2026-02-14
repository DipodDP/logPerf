//go:build !windows

package ping

import (
	"os"
	"syscall"
)

func sigInterrupt() os.Signal {
	return syscall.SIGINT
}
