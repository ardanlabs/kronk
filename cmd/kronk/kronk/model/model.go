// Package model provide support for the model sub-command.
package model

import (
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/index"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/list"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/ps"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/resolve"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/sessions"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model/show"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local models and inspect running models",
	Long: `Manage local models and inspect running models.

This command manages GGUF model files stored locally on your system. It provides
operations for building indexes, listing available models, downloading from the
catalog, removing unused models, and querying which models are currently loaded
in memory.

MODES

  Web Mode (default): Communicates with running server at localhost:11435.
  Local Mode (--local): Direct file access without requiring a server.`,
	Example: `
  # List all downloaded models
  kronk model list

  # Download a model via catalog
  kronk model pull unsloth/Qwen3-1.7B-UD-Q8_K_XL

  # Show model details
  kronk model show unsloth/Qwen3-1.7B-UD-Q8_K_XL

  # Check which models are loaded in memory
  kronk model ps`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(index.Cmd)
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(pull.Cmd)
	Cmd.AddCommand(remove.Cmd)
	Cmd.AddCommand(resolve.Cmd)
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(ps.Cmd)
	Cmd.AddCommand(sessions.Cmd)
}
