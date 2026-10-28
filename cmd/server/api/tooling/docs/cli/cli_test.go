package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/api/tooling/docs/cli"
	"github.com/spf13/cobra"
)

type referenceFlag struct {
	Name      string
	Shorthand string
	Type      string
	Default   string
	Usage     string
	Inherited bool
}

type referenceCommand struct {
	Path        string
	Usage       string
	Description string
	Example     string
	Flags       []referenceFlag
}

func TestReference(t *testing.T) {
	root := &cobra.Command{Use: "sample", Version: "1.2.3"}
	root.PersistentFlags().String("host", "localhost:1234", "Server address")
	root.PersistentFlags().String("secret", "", "Hidden credential")
	if err := root.PersistentFlags().MarkHidden("secret"); err != nil {
		t.Fatal(err)
	}
	parent := &cobra.Command{Use: "model", Short: "Manage models"}
	child := &cobra.Command{
		Use: "pull <source>", Short: "Download a model", Long: "Download without inference.",
		Example: "sample model pull owner/model", Run: func(*cobra.Command, []string) {
			t.Fatal("reference generation executed a command")
		},
	}
	child.Flags().BoolP("local", "l", false, "Use local files")
	// A parsed value must not replace the documented default.
	if err := child.Flags().Set("local", "true"); err != nil {
		t.Fatal(err)
	}
	shadow := &cobra.Command{Use: "show", Short: "Show a model"}
	shadow.Flags().String("host", "override:5678", "Command-specific address")
	parent.AddCommand(child, shadow, &cobra.Command{Use: "internal", Hidden: true})
	root.AddCommand(parent)

	data, err := cli.Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	var entries []referenceCommand
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]referenceCommand)
	for _, entry := range entries {
		byPath[entry.Path] = entry
		for _, f := range entry.Flags {
			if f.Name == "secret" {
				t.Error("hidden flag appeared in reference")
			}
		}
	}
	if _, ok := byPath["sample model internal"]; ok {
		t.Error("hidden command appeared in reference")
	}
	for _, path := range []string{"sample", "sample model", "sample model pull", "sample model show", "sample completion zsh"} {
		if _, ok := byPath[path]; !ok {
			t.Errorf("missing command %q", path)
		}
	}
	pull := byPath["sample model pull"]
	if pull.Usage != "sample model pull <source> [flags]" || pull.Description != "Download without inference." || pull.Example != "sample model pull owner/model" {
		t.Errorf("pull reference: got %+v", pull)
	}
	for _, tc := range []struct {
		path string
		want referenceFlag
	}{
		{"sample model pull", referenceFlag{Name: "host", Type: "string", Default: "localhost:1234", Usage: "Server address", Inherited: true}},
		{"sample model pull", referenceFlag{Name: "local", Shorthand: "l", Type: "bool", Default: "false", Usage: "Use local files"}},
		{"sample model show", referenceFlag{Name: "host", Type: "string", Default: "override:5678", Usage: "Command-specific address"}},
	} {
		var matches []referenceFlag
		for _, f := range byPath[tc.path].Flags {
			if f.Name == tc.want.Name {
				matches = append(matches, f)
			}
		}
		if len(matches) != 1 || matches[0] != tc.want {
			t.Errorf("%s --%s: got %+v, want %+v", tc.path, tc.want.Name, matches, tc.want)
		}
	}
	again, err := cli.Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Error("regeneration changed reference without command changes")
	}
}
