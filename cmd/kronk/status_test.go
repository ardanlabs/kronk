package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/status"
	"github.com/spf13/cobra"
)

type commandExitError struct{}

func (commandExitError) Error() string { return "child failed" }
func (commandExitError) ExitCode() int { return 7 }

func TestCommandStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   []string
		args   []string
		err    error
		preErr error
		want   []string
		code   int
	}{
		{name: "short-success", path: []string{"devices"}},
		{name: "short-error", path: []string{"devices"}, err: errors.New("failed"), want: []string{"error"}, code: 1},
		{name: "child-exit-code", path: []string{"launch"}, err: commandExitError{}, want: []string{"error"}, code: 7},
		{name: "pull", path: []string{"model", "pull"}, want: []string{"working", "done"}},
		{name: "pull-error", path: []string{"model", "pull"}, err: errors.New("failed"), want: []string{"working", "error"}, code: 1},
		{name: "cancellation", path: []string{"model", "pull"}, err: context.Canceled, want: []string{"working", "idle"}, code: 1},
		{name: "libs", path: []string{"libs"}, want: []string{"working", "done"}},
		{name: "libs-list", path: []string{"libs"}, args: []string{"--list-installs"}},
		{name: "bucky-libs", path: []string{"bucky", "libs"}, want: []string{"working", "done"}},
		{name: "malina-libs", path: []string{"malina", "libs"}, want: []string{"working", "done"}},
		{name: "server", path: []string{"server", "start"}, want: []string{"working", "idle"}},
		{name: "detached-server", path: []string{"server", "start"}, args: []string{"--detach"}, want: []string{"working", "done"}},
		{name: "argument-error", path: []string{"libs"}, args: []string{"--unknown"}, want: []string{"error"}, code: 1},
		{name: "auth-error", path: []string{"security", "key"}, preErr: errors.New("unauthorized"), want: []string{"error"}, code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "kronk"}
			leaf := root
			for _, name := range tc.path {
				child := &cobra.Command{Use: name}
				leaf.AddCommand(child)
				leaf = child
			}
			leaf.Flags().Bool("detach", false, "")
			leaf.Flags().Bool("list-installs", false, "")
			leaf.RunE = func(cmd *cobra.Command, args []string) error { return tc.err }
			root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { return tc.preErr }
			root.SetArgs(append(append([]string{}, tc.path...), tc.args...))
			var reports, output bytes.Buffer
			root.SetErr(&output)
			root.SetOut(&output)
			code := execute(status.WithWriter(context.Background(), &reports), root)
			if code != tc.code {
				t.Fatalf("exit code: got %d, want %d", code, tc.code)
			}
			var states []string
			for _, report := range strings.Split(reports.String(), "\x1b]7501;state=")[1:] {
				states = append(states, strings.SplitN(report, ":", 2)[0])
			}
			if strings.Join(states, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("states: got %v, want %v", states, tc.want)
			}
			if tc.err != nil && strings.Count(output.String(), tc.err.Error()) != 1 {
				t.Fatalf("error output: got %q, want one copy of %q", output.String(), tc.err)
			}
		})
	}
}
