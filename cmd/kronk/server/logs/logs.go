// Package logs manages the server logs sub-command.
package logs

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
)

func runLocal() error {
	logFile := logFilePath()

	tail := exec.Command("tail", "-f", logFile)
	tail.Stdout = os.Stdout
	tail.Stderr = os.Stderr

	if err := tail.Run(); err != nil {
		return err
	}

	return nil
}

func logFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.log")
}
