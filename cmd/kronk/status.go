package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ardanlabs/kronk/cmd/kronk/status"
	"github.com/spf13/cobra"
)

func execute(ctx context.Context, root *cobra.Command) int {
	root.SilenceErrors = true
	root.SilenceUsage = true
	addStatus(root)
	_, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}

	if errors.Is(err, context.Canceled) {
		status.Report(ctx, "idle", "Canceled", -1)
	} else {
		msg := err.Error()
		if msg == "" {
			msg = "Command failed"
		}
		status.Report(ctx, "error", msg, -1)
	}
	if err.Error() != "" {
		fmt.Fprintln(root.ErrOrStderr(), err)
	}
	if exitErr, ok := errors.AsType[interface {
		error
		ExitCode() int
	}](err); ok {
		return exitErr.ExitCode()
	}
	return 1
}

// Only long-running operations announce work. All errors, including argument
// validation and authentication failures, are reported centrally by execute.
func addStatus(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		addStatus(child)
	}

	var msg string
	switch cmd.CommandPath() {
	case "kronk model pull", "kronk bucky model pull", "kronk malina model pull":
		msg = "Downloading model"
	case "kronk libs", "kronk bucky libs", "kronk malina libs":
		msg = "Installing libraries"
	case "kronk server start":
		msg = "Starting server"
	default:
		return
	}

	run := cmd.RunE
	if run == nil {
		return
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for _, flag := range []string{"list-combinations", "list-installs", "remove-install"} {
			if enabled, _ := cmd.Flags().GetBool(flag); enabled {
				return run(cmd, args)
			}
		}

		ctx := cmd.Context()
		detached, _ := cmd.Flags().GetBool("detach")
		foregroundServer := cmd.CommandPath() == "kronk server start" && !detached
		if !foregroundServer {
			var stop context.CancelFunc
			ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			cmd.SetContext(ctx)
		}
		status.Report(ctx, "working", msg, -1)
		if err := run(cmd, args); err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if cmd.CommandPath() == "kronk server start" {
			if foregroundServer {
				status.Report(ctx, "idle", "Server stopped", -1)
				return nil
			}
			status.Report(ctx, "done", "Server launch completed", -1)
			return nil
		}
		status.Report(ctx, "done", "Completed", -1)
		return nil
	}
}
