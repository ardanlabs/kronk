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
	Long: `KRONK - Local LLM inference server with hardware acceleration

Kronk provides a complete solution for running large language models locally
with support for GPU acceleration via Metal (macOS), CUDA (NVIDIA), ROCm (AMD),
and Vulkan across multiple platforms.

USAGE
  kronk [command]

COMMANDS
  server    Start/stop the model server
  catalog   Manage model catalogs (list, pull, show, update)
  model     Manage local models (list, pull, remove, show, ps)
  libs      Install/upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  run       Run a model directly for interactive chat (no server needed)

QUICK START
  # List available models
  $ kronk catalog list --local

  # Download a model (e.g., Qwen3-8B)
  $ kronk catalog pull Qwen3-8B-Q8_0 --local

  # Start the server (runs on http://localhost:8080)
  $ kronk server start

  # Open the Browser UI in your browser
  $ open http://localhost:8080

FEATURES
  - Text generation, Vision, Audio, Embeddings, and Reranking
  - Hardware acceleration: Metal, CUDA, ROCm, Vulkan, CPU
  - Batch processing with message caching (SPC/IMC)
  - YaRN context extension for extended context windows
  - Model pooling, catalog system, and Browser UI
  - MCP service integration, authentication, and observability

OPERATING MODES
  Web mode (default)    - CLI communicates with running server at localhost:8080
  Local mode (--local)  - Direct file operations without connecting to a server

ENVIRONMENT VARIABLES
  KRONK_BASE_PATH      Base path for kronk data (models, templates, catalogs)
  KRONK_PROCESSOR      Hardware target: cpu, cuda, metal, rocm, vulkan
  KRONK_LIB_VERSION    Pin llama.cpp library version
  KRONK_HF_TOKEN       HuggingFace auth token for gated models
  KRONK_WEB_API_HOST   Override web API host when server runs elsewhere
  KRONK_TOKEN          Pre-authenticated JWT token for API calls

FOR MORE INFORMATION
  $ kronk <command> --help    Get detailed help for any command
  $ kronk --help              View this complete help message
  See .manual/ directory or AGENTS.md for full documentation`,
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
