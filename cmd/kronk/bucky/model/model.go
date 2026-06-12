// Package model provides the "bucky model" sub-command tree for
// managing local whisper (GGML) model files.
package model

import (
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/model/catalog"
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/model/list"
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/model/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/model/remove"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model". It groups the
// whisper model management verbs (catalog, list, pull, remove).
var Cmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local whisper models (catalog, list, pull, remove)",
	Long: `Manage local whisper (GGML) models.

Whisper models are single .bin files stored flat under the bucky models
root (default: ~/.kronk/bucky-models/). The on-disk filename convention
mirrors the upstream HuggingFace mirror (ggml-<name>.bin); the short
name strips the "ggml-" prefix and ".bin" suffix.

COMMANDS

  catalog  List the bundled catalog of well-known whisper models
  list     List installed whisper models
  pull     Download a whisper model by short name or URL
  remove   Remove a whisper model from disk

EXAMPLES

  # List the bundled catalog.
  kronk bucky model catalog

  # Download the tiny English whisper model.
  kronk bucky model pull ggml-tiny.bin

  # List installed models.
  kronk bucky model list

  # Remove a model.
  kronk bucky model remove ggml-tiny.bin`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(catalog.Cmd)
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(pull.Cmd)
	Cmd.AddCommand(remove.Cmd)
}
