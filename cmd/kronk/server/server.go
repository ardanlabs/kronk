// Package server provide support for the server sub-command.
package server

import (
	"github.com/ardanlabs/kronk/cmd/kronk/server/logs"
	"github.com/ardanlabs/kronk/cmd/kronk/server/start"
	"github.com/ardanlabs/kronk/cmd/kronk/server/stop"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "server",
	Short: "Start, stop, and manage the Kronk model server",
	Long: `Start, stop, and manage the Kronk model server.

The Model Server provides an OpenAI-compatible REST API for local LLM inference.
It supports text generation, vision, audio, embeddings, and reranking with hardware
acceleration via Metal (macOS), CUDA (NVIDIA), ROCm (AMD), Vulkan, or CPU.

The server includes:
  • OpenAI-compatible REST API endpoints
  • Browser UI (BUI) for interactive testing and management
  • Model caching with configurable TTL
  • Distributed tracing via Tempo/OpenTelemetry
  • Prometheus metrics for observability
  • JWT authentication and rate limiting
  • Built-in MCP service for AI agent tool integration

COMMANDS

  start    Start the Kronk model server
  stop     Stop the running Kronk model server
  logs     View server logs`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(start.Cmd)
	Cmd.AddCommand(stop.Cmd)
	Cmd.AddCommand(logs.Cmd)
}
