// Package catalog provides support for the catalog sub-command.
package catalog

import (
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/list"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/show"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "catalog",
	Short: "Browse and manage the model catalog (list, show, remove)",
	Long: `Browse and manage the model catalog.

The catalog is the curated set of HuggingFace models the system knows how to
download. Entries are stored in catalog.yaml under ~/.kronk/catalog/ and seeded
from an embedded default on first run.

COMMANDS

  list     List all catalog entries
  show     Display detailed information about a catalog entry
  remove   Remove a catalog entry, its GGUF cache, and any downloaded files

MODES

  Web Mode (default): Communicates with running server at localhost:11435.
  Local Mode (--local): Direct file access without requiring a server.

EXAMPLES

  # List every catalog entry
  kronk catalog list

  # Show full details for a single entry
  kronk catalog show unsloth/Qwen3-8B-GGUF

  # Remove an entry plus its downloaded files
  kronk catalog remove unsloth/Qwen3-8B-GGUF`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(remove.Cmd)
}
