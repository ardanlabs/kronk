// Package catalog provide support for the catalog sub-command.
package catalog

import (
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/list"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/show"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/update"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage model catalogs (list, pull, show, update)",
	Long: `Manage model catalogs - list, pull, show, and update available models.

The catalog system provides a curated collection of verified GGUF models with 
preconfigured settings for optimal performance. Models are organized by type:
  • Text-Generation - For chat and text completion tasks
  • Image-Text-to-Text - Vision models for image analysis
  • Audio-Text-to-Text - Speech models for transcription
  • Embedding - For vector search and similarity

COMMANDS

  list    List available models from the catalog
  pull    Download a model from the catalog
  show    Display detailed information about a model
  update  Update the local catalog cache

MODES

  Web Mode (default): Communicates with running server at localhost:8080.
  Local Mode (--local): Direct file access without requiring a server.

EXAMPLES

  # List all available models
  kronk catalog list

  # Filter by category
  kronk catalog list --filter-category=Text-Generation

  # Download a model
  kronk catalog pull Qwen3-8B-Q8_0

  # Show model details
  kronk catalog show Qwen3-8B-Q8_0`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(pull.Cmd)
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(update.Cmd)
}
