package pull

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "pull <MODEL_URL|SHORTHAND> [MMPROJ_URL]",
	Short: "Pull a model from the web",
	Long: `Pull a model from the web, the mmproj file is optional.

The model can be specified as:
  - A full HuggingFace URL: https://huggingface.co/org/repo/resolve/main/model.gguf
  - A short form: org/repo/model.gguf
  - A shorthand: owner/repo:Q4_K_M (auto-resolves files via HuggingFace API)
  - With hf.co prefix: hf.co/owner/repo:Q4_K_M
  - With revision: owner/repo:Q4_K_M@revision

Shorthand references automatically resolve multi-file (split) models and
projection files for vision/audio models.

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_BASE_PATH  Base path for kronk data (models, templates, catalog)
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

	models, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("unable to create models system: %w", err)
	}

	switch local {
	case true:
		err = runLocal(models, args)
	default:
		err = runWeb(args)
	}

	if err != nil {
		return err
	}

	return nil
}
