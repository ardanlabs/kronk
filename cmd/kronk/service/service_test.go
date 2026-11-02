package service_test

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/server/start"
	"github.com/ardanlabs/kronk/cmd/kronk/service"
	"github.com/spf13/cobra"
)

func TestCommandArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"detach", []string{"install", "--", "--detach"}, "--detach is not allowed"},
		{"detach false", []string{"install", "--", "--detach=false"}, "--detach is not allowed"},
		{"duplicate base", []string{"install", "--", "--base-path", "other"}, "--base-path is not allowed"},
		{"secret", []string{"install", "--", "--hf-token=secret"}, "--hf-token is not allowed"},
		{"unknown flag", []string{"install", "--", "--not-a-server-flag"}, "unknown flag"},
		{"positional", []string{"install", "--", "server"}, "not positional"},
		{"missing separator", []string{"install", "server"}, "after --"},
		{"help in server args", []string{"install", "--", "--help"}, "--help is not allowed"},
		{"unexpected action args", []string{"status", "extra"}, "unknown command"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := service.NewWindowsCommand(start.NewCommand())
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want %q", err, test.want)
			}
		})
	}
}

func TestHelpAndUnsupportedPlatform(t *testing.T) {
	root := &cobra.Command{Use: "kronk"}
	root.PersistentFlags().String("base-path", "", "")
	root.AddCommand(service.NewWindowsCommand(start.NewCommand()))
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"service", "install", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--account") || !strings.Contains(output.String(), "--base-path") {
		t.Fatalf("install help is missing account or inherited base-path flag: %s", output.String())
	}
	if runtime.GOOS == "windows" {
		return
	}
	cmd := service.NewWindowsCommand(start.NewCommand())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"install", "--", "--api-host=127.0.0.1:11435", "--media-backends-enabled=false"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "only on Windows") {
		t.Fatalf("install: got %v, want Windows-only error", err)
	}
}

func TestRunConsoleLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := start.NewCommand()
	cmd.SetContext(ctx)
	called := false
	err := service.Run(cmd, func(runCtx context.Context, ready func()) error {
		called = true
		if runCtx != ctx {
			t.Error("console server did not receive the caller's context")
		}
		if ready != nil {
			t.Error("console server received an SCM readiness hook")
		}
		cancel()
		return runCtx.Err()
	})
	if !called || !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: called=%v, error=%v; want caller cancellation from the console server", called, err)
	}
}
