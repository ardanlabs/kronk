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

This command provides a simple interactive interface for chatting directly with
a GGUF model without starting the full Model Server. It loads the model, applies
the chat template, and enters a REPL loop for conversation.

FEATURES

  • Interactive chat with streaming responses
  • Customizable Jinja chat templates
  • Fine-grained control over inference parameters
  • No server required - runs directly on your machine

COMMAND LINE OPTIONS

Model Configuration:
  --jinja-file          Path to custom Jinja template file
  --context-window      Context window size in tokens
  --flash-attention     Flash attention mode (on, off, auto)
  --ngpu-layers         GPU layers to offload (-1 = CPU only)
  --cache-type-k        KV cache type for keys (f16, q8_0, etc.)
  --cache-type-v        KV cache type for values (f16, q8_0, etc.)
  --nbatch              Logical batch size for processing
  --nubatch             Physical micro-batch size

Sampling Parameters:
  --max-tokens          Maximum tokens for response
  --temperature         Temperature for sampling (0.0-2.0)
  --top-p               Top-p nucleus sampling parameter
  --top-k               Top-k sampling parameter
  --min-p               Minimum probability threshold
  --repeat-penalty      Repetition penalty
  --frequency-penalty   Frequency penalty
  --presence-penalty    Presence penalty

Model-Specific:
  --enable-thinking     Enable thinking/reasoning mode (true, false)
  --reasoning-effort    Reasoning effort level (low, medium, high)

EXAMPLES

  # Start chat with a model
  kronk run Qwen3-8B-Q8_0

  # Use custom template and context window
  kronk run Llama-3.3-70B-Instruct-Q8_0 --jinja-file=/tmp/template.j2 --context-window=32764

  # Run with GPU offloading
  kronk run Qwen3-8B-Q8_0 --ngpu-layers=35 --temperature=0.7

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH    Base directory for kronk data (models, templates, catalog)
  KRONK_MODELS       Path to the models directory (default: $HOME/.kronk/models)`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {

	// Model configuration flags.
	Cmd.Flags().String("jinja-file", "", "Path to a custom Jinja template file")
	Cmd.Flags().Int("context-window", 0, "Context window size in tokens")
	Cmd.Flags().String("flash-attention", "", "Flash attention mode (on, off, auto)")
	Cmd.Flags().Int("ngpu-layers", 0, "Number of layers to offload to GPU (-1 = CPU only)")
	Cmd.Flags().String("cache-type-k", "", "KV cache type for keys (f16, q8_0, q4_0, etc.)")
	Cmd.Flags().String("cache-type-v", "", "KV cache type for values (f16, q8_0, q4_0, etc.)")
	Cmd.Flags().Int("nbatch", 0, "Logical batch size for processing")
	Cmd.Flags().Int("nubatch", 0, "Physical micro-batch size for prompt ingestion")

	// Sampling parameter flags.
	Cmd.Flags().Int("max-tokens", 0, "Maximum tokens for response")
	Cmd.Flags().Float64("temperature", 0.0, "Temperature for sampling")
	Cmd.Flags().Float64("top-p", 0.0, "Top-p for sampling")
	Cmd.Flags().Int("top-k", 0, "Top-k for sampling")
	Cmd.Flags().Float64("min-p", 0.0, "Min-p for sampling")
	Cmd.Flags().Float64("repeat-penalty", 0.0, "Repetition penalty")
	Cmd.Flags().Float64("frequency-penalty", 0.0, "Frequency penalty")
	Cmd.Flags().Float64("presence-penalty", 0.0, "Presence penalty")
	Cmd.Flags().String("enable-thinking", "", "Enable thinking/reasoning (true, false)")
	Cmd.Flags().String("reasoning-effort", "", "Reasoning effort level (low, medium, high)")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	modelName := args[0]

	// Model configuration flags.
	jinjaFile, _ := cmd.Flags().GetString("jinja-file")
	contextWindow, _ := cmd.Flags().GetInt("context-window")
	flashAttention, _ := cmd.Flags().GetString("flash-attention")
	ngpuLayers, _ := cmd.Flags().GetInt("ngpu-layers")
	cacheTypeK, _ := cmd.Flags().GetString("cache-type-k")
	cacheTypeV, _ := cmd.Flags().GetString("cache-type-v")
	nbatch, _ := cmd.Flags().GetInt("nbatch")
	nubatch, _ := cmd.Flags().GetInt("nubatch")

	// Sampling parameter flags.
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	temperature, _ := cmd.Flags().GetFloat64("temperature")
	topP, _ := cmd.Flags().GetFloat64("top-p")
	topK, _ := cmd.Flags().GetInt("top-k")
	minP, _ := cmd.Flags().GetFloat64("min-p")
	repeatPenalty, _ := cmd.Flags().GetFloat64("repeat-penalty")
	frequencyPenalty, _ := cmd.Flags().GetFloat64("frequency-penalty")
	presencePenalty, _ := cmd.Flags().GetFloat64("presence-penalty")
	enableThinking, _ := cmd.Flags().GetString("enable-thinking")
	reasoningEffort, _ := cmd.Flags().GetString("reasoning-effort")

	cfg := Config{
		ModelName: modelName,
		BasePath:  client.GetBasePath(cmd),

		// Model configuration.
		JinjaFile:      jinjaFile,
		ContextWindow:  contextWindow,
		FlashAttention: flashAttention,
		NGpuLayers:     ngpuLayers,
		CacheTypeK:     cacheTypeK,
		CacheTypeV:     cacheTypeV,
		NBatch:         nbatch,
		NUBatch:        nubatch,

		// Sampling parameters.
		MaxTokens:        maxTokens,
		Temperature:      temperature,
		TopP:             topP,
		TopK:             topK,
		MinP:             minP,
		RepeatPenalty:    repeatPenalty,
		FrequencyPenalty: frequencyPenalty,
		PresencePenalty:  presencePenalty,
		EnableThinking:   enableThinking,
		ReasoningEffort:  reasoningEffort,
	}

	return runChat(cfg)
}
