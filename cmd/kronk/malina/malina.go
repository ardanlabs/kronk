// Package malina provides the parent "malina" sub-command tree for
// stable-diffusion.cpp library and model-bundle management.
package malina

import (
	"github.com/ardanlabs/kronk/cmd/kronk/malina/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/malina/model"
	"github.com/spf13/cobra"
)

// Cmd is the parent "malina" cobra command.
var Cmd = newCmd()

func newCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "malina",
		Short: "Stable Diffusion backend: libs and model management",
		Long: `Manage the local stable-diffusion.cpp runtime and curated image-model bundles.

The Malina CLI currently requires --local on management commands and operates
directly on local files without a running server.`,
		Example: `
  kronk malina libs --local
  kronk malina model catalog --local
  kronk malina model pull --local sd-1.5
  kronk malina model list --local`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(libs.Cmd)
	cmd.AddCommand(model.Cmd)

	return cmd
}
