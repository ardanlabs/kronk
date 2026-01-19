package vram

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "vram <MODEL_NAME>",
	Short: "Calculate estimated VRAM usage for a model",
	Long: `Calculate estimated VRAM usage for a model

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_BASE_PATH  Base path for kronk data (models, templates, catalog)
      KRONK_MODELS     (default: $HOME/.kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", true, "Run without the model server")
	Cmd.Flags().Uint64("context-length", 8192, "Context length for VRAM calculation")
	Cmd.Flags().String("kv-cache-type", "fp8", "KV cache type for VRAM calculation (fp32, fp16, bf16, fp8, q8_0, q6_k, q5_1, q5_0, q5_k, q4_1, q4_0, q4_k, q3_k, q2_k, iq4_nl, iq4_xs, iq3_xxs, iq3_s, iq2_xxs, iq2_xs, iq2_s, iq1_s, iq1_m)")
	Cmd.Flags().Bool("--show-common", false, "Show VRAM usage for common KV cache sizes")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("at least model ID is required")
	}

	modelID := ""
	if len(args) >= 1 {
		modelID = args[0]
	}

	contextLength, err := cmd.Flags().GetUint64("context-length")
	if err != nil {
		return err
	}

	kvCacheType, err := cmd.Flags().GetString("kv-cache-type")
	if err != nil {
		return err
	}

	models, err := models.NewWithPaths(client.GetBasePath(cmd))
	if err != nil {
		return fmt.Errorf("unable to create models system: %w", err)
	}

	switch local {
	case true:
		err = runLocal(models, modelID, contextLength, kvCacheType)
	default:
		err = runWeb(args)
	}

	if err != nil {
		return err
	}

	return nil
}
