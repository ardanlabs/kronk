package main

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/catalog"
	"github.com/ardanlabs/kronk/cmd/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/model"
	"github.com/ardanlabs/kronk/cmd/kronk/run"
	"github.com/ardanlabs/kronk/cmd/kronk/security"
	"github.com/ardanlabs/kronk/cmd/kronk/server"
	k "github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/spf13/cobra"
)

var version = k.Version

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "kronk",
	Short: "Local LLM inference with hardware acceleration",
	Long: `Kronk is a Go SDK and Model Server for running local inference with open-source GGUF models. 
Built on llama.cpp via the yzma Go bindings (a non-CGO FFI layer), Kronk provides hardware-accelerated 
inference for text generation, vision, audio, embeddings, and reranking.

The SDK is the foundation—Kronk's Model Server is built entirely on top of it. You can embed local 
LLM inference directly into your Go applications, or run the Model Server for OpenAI-compatible REST APIs,
OpenWebUI integration, and agent tool support.

FEATURES

  • Model Types: Text generation, vision (image analysis), audio (speech-to-text), 
    embeddings (vector search), and reranking (document relevance)

  • Hardware Acceleration: Metal (macOS), CUDA (NVIDIA), ROCm (AMD GPU), Vulkan 
    (cross-platform), or CPU fallback

  • Performance: Batch processing for concurrent requests, system prompt and 
    incremental message caching, YaRN context extension (2-4x native window)

  • Model Pooling: Keep models loaded in memory with configurable TTL for faster responses

  • Catalog System: Curated collection of verified GGUF models with one-command downloads

  • Browser UI (BUI): Web interface for model management, downloads, configuration, 
    and interactive testing

  • MCP Service: Built-in Model Context Protocol support for AI agent tool integration
    (e.g., web_search via Brave Search API)

  • Security: JWT-based authentication, key management, endpoint authorization, and rate limiting

  • Observability: Distributed tracing (Tempo/OpenTelemetry), Prometheus metrics, pprof profiling

USAGE

  Run as a library in your Go code:
    import "github.com/ardanlabs/kronk/sdk/kronk"
    
    krn, _ := kronk.New(cfg)
    defer krn.Unload(ctx)

  Or run as a server with OpenAI-compatible API:
    kronk server start
    curl http://localhost:8080/v1/chat/completions -d '{"model":"Qwen3-8B-Q8_0","messages":[...]}'

QUICK START

  # Install llama.cpp libraries
  kronk libs --local

  # List available models from the catalog
  kronk catalog list --local

  # Download a model (e.g., Qwen3-8B)
  kronk catalog pull Qwen3-8B-Q8_0 --local

  # Start the model server (runs in foreground)
  kronk server start

  # Or run in background
  kronk server start -d

  # Open the Browser UI
  open http://localhost:8080

COMMANDS

  server    Start, stop, and manage the Kronk model server
  catalog   Manage model catalogs (list, pull, show, update)
  model     Manage local models (index, list, pull, remove, show, ps)
  libs      Install or upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  run       Run a model directly for interactive chat without a server

MODES OF OPERATION

  Web Mode (default): Most commands communicate with a running server at localhost:8080.
    This mode requires an active server and enables progress streaming in the BUI.

  Local Mode (--local flag): Run commands directly without a server for:
    - Initial setup when no server is running
    - Same-machine operations with direct file access
    - Offline model and library management

ENVIRONMENT VARIABLES

  KRONK_BASE_PATH        - Base directory for all Kronk data (default: ~/.kronk)
  KRONK_PROCESSOR        - Hardware backend: cpu, metal, cuda, rocm, vulkan
  KRONK_LIB_VERSION      - Pin llama.cpp library version
  KRONK_HF_TOKEN         - Hugging Face API token for gated models
  KRONK_WEB_API_HOST     - Model server API address (default: localhost:8080)
  KRONK_TOKEN            - Admin token for security commands

See "kronk <command> --help" for more information on each command.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.Version = version

	rootCmd.PersistentFlags().String("base-path", "", "Base path for kronk data (models, templates, catalog)")

	rootCmd.AddCommand(server.Cmd)
	rootCmd.AddCommand(libs.Cmd)
	rootCmd.AddCommand(model.Cmd)
	rootCmd.AddCommand(catalog.Cmd)
	rootCmd.AddCommand(security.Cmd)
	rootCmd.AddCommand(run.Cmd)
}
