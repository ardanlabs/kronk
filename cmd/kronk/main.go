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
	Long: "Kronk is a Go SDK and Model Server for running local inference with open-source\n" +
		"GGUF models. Built on llama.cpp via the yzma Go bindings (a non-CGO FFI layer),\n" +
		"Kronk provides hardware-accelerated inference for text, vision, audio, embeddings,\n" +
		"and reranking.\n\n" +
		"Key Features:\n" +
		"  • Model Types: Text, vision, audio, embeddings, and reranking\n" +
		"  • Hardware: Metal (macOS), CUDA (NVIDIA), ROCm (AMD), Vulkan (cross-platform)\n" +
		"  • Batch Processing: Process multiple requests concurrently\n" +
		"  • Message Caching: System prompt and incremental message caching\n" +
		"  • Extended Context: YaRN support for 2-4x context window extension\n" +
		"  • Model Pooling: Keep models loaded in memory with configurable TTL\n" +
		"  • Browser UI: Web interface for model management and testing\n\n" +
		"Use Kronk two ways:\n" +
		"  • As a library: Embed local LLM inference directly into your Go app\n" +
		"  • As a server: OpenAI-compatible REST API with authentication\n\n" +
		"Quick Start:\n" +
		"  • kronk server start        - Start the model server\n" +
		"  • kronk catalog pull <model> - Download a model from the catalog\n" +
		"  • kronk catalog list        - List available models\n" +
		"  • kronk libs                - Install llama.cpp libraries",
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
