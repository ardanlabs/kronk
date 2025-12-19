// Package start manages the server start sub-command.
package start

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/spf13/cobra"
)

func runLocal(cmd *cobra.Command) error {
	detach, _ := cmd.Flags().GetBool("detach")

	if detach {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("executable: %w", err)
		}

		logFile, _ := os.Create(logFilePath())

		proc := exec.Command(exePath, "server")
		proc.Stdout = logFile
		proc.Stderr = logFile
		proc.Stdin = nil
		proc.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

		if err := proc.Start(); err != nil {
			return fmt.Errorf("start: %w", err)
		}

		pidFile := pidFilePath()
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(proc.Process.Pid)), 0644); err != nil {
			return fmt.Errorf("failed to write pid file: %w", err)
		}

		fmt.Printf("Kronk server started in background (PID: %d)\n", proc.Process.Pid)

		return nil
	}

	if err := kronk.Run(false); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func logFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.log")
}

func pidFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.pid")
}
