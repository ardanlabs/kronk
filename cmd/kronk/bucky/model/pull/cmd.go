// Package pull downloads a whisper model by short name or URL.
package pull

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model pull".
var Cmd = &cobra.Command{
	Use:   "pull <MODEL>",
	Short: "Download a whisper model by short name or URL",
	Long: `Download a whisper model from the bundled catalog or from a URL.

The argument may be:

  - A short catalog name        ("tiny", "base.en", "large-v3")
  - A full ggml filename        ("ggml-tiny.bin")
  - A fully qualified URL       (any go-getter-compatible URL)

Use "kronk bucky model catalog --local" to list the bundled short names.

MODES

  Web Mode (default): Reserved for a future server-wiring step.
  Local Mode (--local): Downloads directly to the bucky models root.

EXAMPLES

  # Download the tiny English model.
  kronk bucky model pull --local tiny.en

  # Download by full filename.
  kronk bucky model pull --local ggml-base.bin

  # Download from a custom URL.
  kronk bucky model pull --local https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

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

func run(cmd *cobra.Command, source string) error {
	local, _ := cmd.Flags().GetBool("local")
	if !local {
		return fmt.Errorf("bucky model pull: web mode not yet implemented; pass --local to run against local files")
	}

	mdls, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("bucky model pull: new: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	path, err := mdls.Download(ctx, bucky.FmtLogger, source)
	if err != nil {
		return fmt.Errorf("bucky model pull: %w", err)
	}

	fmt.Println()
	for _, f := range path.ModelFiles {
		fmt.Println("installed:", f)
	}

	return nil
}
