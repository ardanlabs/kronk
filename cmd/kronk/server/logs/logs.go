// Package logs manages the server logs sub-command.
package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/spf13/cobra"
)

const tailLines = 10

func runLocal(cmd *cobra.Command) error {
	logFile := logFilePath(defaults.BaseDir(client.GetBasePath(cmd)))
	return followLog(cmd.Context(), logFile, cmd.OutOrStdout(), 250*time.Millisecond)
}

func followLog(ctx context.Context, logPath string, output io.Writer, interval time.Duration) error {
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file not found: %s (is the server running in detached mode?)", logPath)
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lines := make([]string, 0, tailLines)
	for scanner.Scan() {
		if len(lines) == tailLines {
			copy(lines, lines[1:])
			lines = lines[:tailLines-1]
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read log file: %w", err)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return fmt.Errorf("write log output: %w", err)
		}
	}

	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("seek log file: %w", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			info, err := file.Stat()
			if err != nil {
				return fmt.Errorf("stat log file: %w", err)
			}
			if info.Size() < offset {
				offset, err = file.Seek(0, io.SeekStart)
				if err != nil {
					return fmt.Errorf("seek truncated log file: %w", err)
				}
			}
			if info.Size() == offset {
				continue
			}

			written, err := io.Copy(output, file)
			offset += written
			if err != nil {
				return fmt.Errorf("follow log file: %w", err)
			}
		}
	}
}

func logFilePath(basePath string) string {
	return filepath.Join(basePath, "kronk.log")
}
