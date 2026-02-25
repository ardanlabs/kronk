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
		"  • Model Types: Text generation, vision, audio, embeddings, and reranking\n" +
		"  • Hardware: Metal (macOS), CUDA, ROCm, Vulkan across Linux, macOS, and Windows\n" +
		"  • Batch Processing: Process multiple requests concurrently\n" +
		"  • Message Caching: System prompt and incremental message caching\n" +
		"  • Extended Context: YaRN support for 2-4x context window extension\n" +
		"  • Model Pooling: Keep models loaded in memory with configurable TTL\n\n" +
		"The Kronk Model Server is built on top of the SDK — everything it can do is\n" +
		"available to you programmatically. Use the CLI for model management, or embed\n" +
		"Kronk directly into your Go applications.\n\n" +
		"Quick Start:\n" +
		"  • kronk server start      - Start the model server\n" +
		"  • kronk catalog pull      - Download models from the catalog\n" +
		"  • kronk libs              - Install llama.cpp libraries",
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
