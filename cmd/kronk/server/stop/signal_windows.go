//go:build windows

package stop

import (
	"os"
)

// terminateProcess kills the process on Windows.
// Windows doesn't support SIGTERM, so we use Kill().
func terminateProcess(process *os.Process) error {
	return process.Kill()
}
