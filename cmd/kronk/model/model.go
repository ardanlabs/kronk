// Package model provide support for the model sub-command.
package model

import (
	"github.com/ardanlabs/kronk/cmd/kronk/model/index"
	"github.com/ardanlabs/kronk/cmd/kronk/model/list"
	"github.com/ardanlabs/kronk/cmd/kronk/model/ps"
	"github.com/ardanlabs/kronk/cmd/kronk/model/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/model/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/model/show"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local models (index, list, pull, remove, show, ps)",
	Long: `Manage local models - index, list, pull, remove, show, and check running models.

This command manages GGUF model files stored locally on your system. It provides
operations for building indexes, listing available models, downloading from catalogs,
removing unused models, and querying which models are currently loaded in memory.

COMMANDS

  index   Build or rebuild the local model index
  list    List all downloaded models
  pull    Download a model from the catalog
  remove  Remove a model from the local system
  show    Display detailed information about a model
  ps      Show models currently loaded in server memory

MODES

  Web Mode (default): Communicates with running server at localhost:8080.
  Local Mode (--local): Direct file access without requiring a server.

EXAMPLES

  # List all downloaded models
  kronk model list

  # Download a model via catalog
  kronk model pull Qwen3-8B-Q8_0

  # Show model details
  kronk model show Qwen3-8B-Q8_0

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
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(ps.Cmd)
}
