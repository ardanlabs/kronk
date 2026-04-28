package pull

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "pull <MODEL_ID|MODEL_URL|SHORTHAND> [MMPROJ_URL]",
	Short: "Pull a model from the web",
	Long: `Pull a model from the web, the mmproj file is optional.

The model can be specified as:
  - A bare model id: Qwen3-0.6B-Q8_0 (resolved via the provider list)
  - A canonical id: unsloth/Qwen3-0.6B-Q8_0 (skips provider walk)
  - A full HuggingFace URL: https://huggingface.co/org/repo/resolve/main/model.gguf
  - A short form: org/repo/model.gguf
  - A shorthand: owner/repo:Q4_K_M (auto-resolves files via HuggingFace API)
  - With hf.co prefix: hf.co/owner/repo:Q4_K_M
  - With revision: owner/repo:Q4_K_M@revision

Bare or canonical ids consult ~/.kronk/catalog.yaml first, then walk
the configured provider list (unsloth, ggml-org, bartowski, ...) and persist
the resolution. Shorthand and URL forms auto-resolve multi-file (split)
models and projection files for vision/audio models.

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:11435)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_BASE_PATH  Base path for kronk data (models, libraries, catalog, model_config)
      KRONK_MODELS     (default: $HOME/.kronk/models)  The path to the models directory`,
	Args: cobra.RangeArgs(1, 2),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")

	basePath := client.GetBasePath(cmd)

	models, err := models.NewWithPaths(basePath)
	if err != nil {
		return fmt.Errorf("unable to create models system: %w", err)
	}

	switch local {
	case true:
		err = runLocal(models, basePath, args)
	default:
		err = runWeb(args)
	}

	if err != nil {
		return err
	}

	return nil
}
