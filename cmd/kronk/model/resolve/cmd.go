// Package resolve provides the model resolve sub-command.
package resolve

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "resolve <MODEL_ID>",
	Short: "Resolve a model id to a provider, repo, files and download URLs",
	Long: `Resolve a model id to a provider, repo, files and download URLs.

Resolution order:
  1. Local on-disk index (re-uses any provider already downloaded)
  2. Resolver file (~/.kronk/catalog.yaml)
  3. HuggingFace API across the configured provider list

The id may be bare (Qwen3.6-35B-A3B-UD-Q4_K_M) or include an explicit
provider (unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M). On a successful HuggingFace
lookup the resolution is cached in the resolver file.

Environment Variables:
      KRONK_BASE_PATH  Base path for kronk data (defaults to $HOME/.kronk)
      KRONK_HF_TOKEN   HuggingFace token; recommended to avoid rate limiting`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("refresh", false, "Bypass the resolver-file cache and force an HF lookup")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	refresh, _ := cmd.Flags().GetBool("refresh")

	basePath := client.GetBasePath(cmd)

	mdls, err := models.NewWithPaths(basePath)
	if err != nil {
		return fmt.Errorf("models system: %w", err)
	}

	rfile, err := defaults.CatalogFile("", basePath)
	if err != nil {
		return fmt.Errorf("resolver file: %w", err)
	}

	r := models.NewResolver(mdls, rfile)

	if refresh {
		if err := dropCacheEntry(r, args[0]); err != nil {
			return fmt.Errorf("refresh: %w", err)
		}
	}

	res, err := r.Resolve(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	printResolution(rfile, res)

	return nil
}
