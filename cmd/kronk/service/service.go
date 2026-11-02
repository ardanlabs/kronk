// Package service owns service installation, control, and runtime integration
// for Linux systemd, macOS launchd, and Windows SCM.
//
// systemd.go, launchd.go, and windows.go define the platform commands. The Linux
// and macOS files also implement their service operations and embed their
// templates; scm_windows.go implements Windows SCM operations. The command
// constructors preserve the existing systemd, launchd, and service CLI groups
// and are registered directly by the root command.
//
// All service managers launch server start, which enters Run. systemd and launchd
// supervise that foreground process and stop it with signals. Windows additionally
// requires an in-process SCM dispatcher to report state and receive stop controls.
// These runtime paths live in runtime_other.go and runtime_windows.go respectively.
package service

import (
	"context"

	"github.com/spf13/cobra"
)

// Run starts the server through the platform lifecycle. On Windows, an SCM
// invocation receives service controls and reports readiness through ready.
// Other invocations run normally with the command's context and no ready hook.
func Run(cmd *cobra.Command, run func(ctx context.Context, ready func()) error) error {
	return runServer(cmd, run)
}
