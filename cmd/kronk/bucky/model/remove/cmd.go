// Package remove deletes an installed whisper model from disk.
package remove

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model remove".
var Cmd = &cobra.Command{
	Use:   "remove <MODEL>",
	Short: "Remove an installed whisper model from disk",
	Long: `Remove an installed whisper model from disk. The argument is the
short name ("tiny", "base.en"), the full ggml filename, or the bare
basename without extension.

MODES

  Web Mode (default): Reserved for a future server-wiring step.
  Local Mode (--local): Removes the model file from disk directly.

EXAMPLES

  # Remove the tiny English model.
  kronk bucky model remove --local tiny.en

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH  Base path for kronk data (models, libraries, catalog, model_config)`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server (currently required; web mode lands with the server-wiring step)")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, modelID string) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return fmt.Errorf("bucky model remove: web mode not yet implemented; pass --local to run against local files")
	}

	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model remove: new: %w", err)
	}

	if err := mdls.BuildIndex(bucky.DiscardLogger, false); err != nil {
		return fmt.Errorf("bucky model remove: build index: %w", err)
	}

	mp, err := mdls.FullPath(modelID)
	if err != nil {
		return fmt.Errorf("bucky model remove: %w", err)
	}

	if err := mdls.Remove(mp, bucky.FmtLogger); err != nil {
		return fmt.Errorf("bucky model remove: %w", err)
	}

	for _, f := range mp.ModelFiles {
		fmt.Println("removed:", f)
	}

	return nil
}
