// Package run provides the run command for interactive chat with models.
package run

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "run <MODEL_NAME>",
	Short: "Run an interactive chat session with a model",
	Long: `Run an interactive chat session with a local model (REPL mode).

This command provides a simple interactive interface for chatting with a model.
Type your messages and press Enter to get responses. Type 'quit' to exit.

Example:
  kronk run Qwen3-8B-Q8_0

Environment Variables:
      KRONK_BASE_PATH  Base path for kronk data (models, templates, catalog)
      KRONK_MODELS     (default: $HOME/.kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Int("max-tokens", 0, "Maximum tokens for response")
	Cmd.Flags().Float64("temperature", 0.0, "Temperature for sampling")
	Cmd.Flags().Float64("top-p", 0.0, "Top-p for sampling")
	Cmd.Flags().Int("top-k", 0, "Top-k for sampling")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	modelName := args[0]

	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	temperature, _ := cmd.Flags().GetFloat64("temperature")
	topP, _ := cmd.Flags().GetFloat64("top-p")
	topK, _ := cmd.Flags().GetInt("top-k")

	cfg := Config{
		ModelName:   modelName,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		TopP:        topP,
		TopK:        topK,
		BasePath:    client.GetBasePath(cmd),
	}

	return runChat(cfg)
}
