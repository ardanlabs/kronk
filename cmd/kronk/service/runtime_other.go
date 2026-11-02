//go:build !windows

package service

import (
	"context"

	"github.com/spf13/cobra"
)

// systemd and launchd supervise the ordinary foreground server and send signals
// handled by its existing shutdown path. Neither needs an in-process dispatcher.
func runServer(cmd *cobra.Command, run func(context.Context, func()) error) error {
	return run(cmd.Context(), nil)
}
