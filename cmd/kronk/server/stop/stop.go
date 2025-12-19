// Package stop manages the server stop sub-command.
package stop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
)

func runLocal() error {
	pidFile := pidFilePath()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("read-file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("atoi: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find-process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal: %w", err)
	}

	os.Remove(pidFile)
	fmt.Printf("Sent SIGTERM to Kronk server (PID: %d)\n", pid)

	return nil
}

func pidFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.pid")
}
