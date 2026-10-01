// Package stop manages the server stop sub-command.
package stop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/spf13/cobra"
)

func runLocal(cmd *cobra.Command) error {
	pidFile := pidFilePath(defaults.BaseDir(client.GetBasePath(cmd)))

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

	if err := terminateProcess(process); err != nil {
		return fmt.Errorf("terminate: %w", err)
	}

	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pid file: %w", err)
	}
	fmt.Printf("Stopped Kronk server (PID: %d)\n", pid)

	return nil
}

func pidFilePath(basePath string) string {
	return filepath.Join(basePath, "kronk.pid")
}
