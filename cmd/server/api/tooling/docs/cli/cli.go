// Package cli generates the BUI reference from the registered CLI command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/commands"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type command struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Usage       string `json:"usage"`
	Short       string `json:"short"`
	Description string `json:"description"`
	Example     string `json:"example"`
	Flags       []flag `json:"flags"`
}

type flag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
	Inherited bool   `json:"inherited"`
}

// Run writes the CLI reference without executing commands or authentication hooks.
func Run() error {
	data, err := Generate(commands.Cmd)
	if err != nil {
		return err
	}
	path := "cmd/server/api/frontends/bui/src/components/cli-reference.json"
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing CLI reference: %w", err)
	}
	fmt.Printf("Generated %s\n", path)
	return nil
}

// Generate returns a deterministic JSON reference for the visible command tree.
// It includes declared defaults, not parsed flag values, and never executes commands.
func Generate(root *cobra.Command) ([]byte, error) {
	root.InitDefaultCompletionCmd()
	root.InitDefaultHelpCmd()
	var entries []command
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		entry := command{
			Name:        cmd.Name(),
			Path:        cmd.CommandPath(),
			Usage:       cmd.UseLine(),
			Short:       cmd.Short,
			Description: strings.TrimSpace(cmd.Long),
			Example:     strings.TrimSpace(cmd.Example),
			Flags:       []flag{},
		}
		appendFlags := func(flags *pflag.FlagSet, inherited bool) {
			flags.VisitAll(func(f *pflag.Flag) {
				if f.Hidden {
					return
				}
				entry.Flags = append(entry.Flags, flag{
					Name: f.Name, Shorthand: f.Shorthand, Type: f.Value.Type(),
					Default: f.DefValue, Usage: f.Usage, Inherited: inherited,
				})
			})
		}
		appendFlags(cmd.LocalFlags(), false)
		appendFlags(cmd.InheritedFlags(), true)
		entries = append(entries, entry)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding CLI reference: %w", err)
	}
	return append(data, '\n'), nil
}
