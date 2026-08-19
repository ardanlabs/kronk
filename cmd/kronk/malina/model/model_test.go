package model

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunCatalog(t *testing.T) {
	var output bytes.Buffer
	cmd := cobra.Command{}
	cmd.SetOut(&output)

	if err := runCatalog(&cmd, nil); err != nil {
		t.Fatalf("runCatalog() error = %v", err)
	}
	if !strings.Contains(output.String(), "sd-1.5") {
		t.Errorf("catalog output = %q, want sd-1.5", output.String())
	}
}

func TestRunPullRejectsUnknownBundle(t *testing.T) {
	cmd := cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("base-path", t.TempDir(), "")
	if err := runPull(&cmd, "unknown"); err == nil {
		t.Fatal("runPull() error = nil, want unknown bundle error")
	}
}

func TestCommandRequiresLocal(t *testing.T) {
	cmd := newCmd()
	cmd.SetArgs([]string{"catalog"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "use --local") {
		t.Errorf("Execute() error = %v, want use --local", err)
	}
}
