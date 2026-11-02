package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

func runServer(cmd *cobra.Command, run func(context.Context, func()) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return run(cmd.Context(), nil)
	}
	h := handler{run: func(ctx context.Context, ready func()) error {
		return serve(ctx, cmd, ready, run)
	}}
	if err := svc.Run(serviceName, &h); err != nil {
		return fmt.Errorf("running Windows service: %w", err)
	}
	return h.err
}

func serve(ctx context.Context, cmd *cobra.Command, ready func(), run func(context.Context, func()) error) (result error) {
	events, err := eventlog.Open(serviceName)
	if err != nil {
		return fmt.Errorf("opening Event Log: %w", err)
	}
	defer events.Close()
	defer func() {
		if result != nil {
			result = errors.Join(result, events.Error(2, result.Error()))
		} else {
			result = events.Info(3, "Kronk service stopped after cleanup")
		}
	}()
	basePath := client.GetBasePath(cmd)
	if !filepath.IsAbs(basePath) {
		return errors.New("service requires its installed absolute base path")
	}
	if err := validateServerFlags(cmd); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(basePath, "service.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening service log: %w", err)
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	defer func() {
		if result != nil {
			_, err := fmt.Fprintln(file, result)
			result = errors.Join(result, err)
		}
	}()
	os.Stdout, os.Stderr = file, file
	// Native backends write to the Windows standard handles, not Go's variables.
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(file.Fd())); err != nil {
		return fmt.Errorf("redirecting service stdout: %w", err)
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(file.Fd())); err != nil {
		return fmt.Errorf("redirecting service stderr: %w", err)
	}
	if err := events.Info(1, "Kronk service starting"); err != nil {
		return err
	}
	err = run(ctx, ready)
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
