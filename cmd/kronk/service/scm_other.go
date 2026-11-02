//go:build !windows

package service

import (
	"errors"

	"github.com/spf13/cobra"
)

// Only the Windows management commands use these unsupported-platform fallbacks.
func unsupportedWindows() error {
	return errors.New("native services are supported only on Windows; use kronk systemd on Linux or kronk launchd on macOS")
}

func installService(*cobra.Command, []string, *cobra.Command) error { return unsupportedWindows() }
func manageService(*cobra.Command, string) error                    { return unsupportedWindows() }
