//go:build !windows

package stop

import (
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM to gracefully stop the process.
func terminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
