package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ardanlabs/kronk/cmd/kronk/bucky"
	"github.com/ardanlabs/kronk/cmd/kronk/devices"
	"github.com/ardanlabs/kronk/cmd/kronk/diagnose"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/catalog"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model"
	"github.com/ardanlabs/kronk/cmd/kronk/launch"
	"github.com/ardanlabs/kronk/cmd/kronk/malina"
	"github.com/ardanlabs/kronk/cmd/kronk/security"
	"github.com/ardanlabs/kronk/cmd/kronk/server"
	"github.com/ardanlabs/kronk/cmd/kronk/status"
	"github.com/ardanlabs/kronk/cmd/kronk/systemd"
	k "github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var version = k.Version

func main() {
	ctx := context.Background()
	if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb" {
		ctx = status.WithWriter(ctx, os.Stderr)
	}
	os.Exit(execute(ctx, rootCmd))
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
  systemd   Install/manage the Linux systemd user service
  model     Manage local models (list, pull, remove, show, ps)
  catalog   Browse and manage the model catalog (list, show, remove)
  libs      Install/upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  launch    Run OpenCode with a local Kronk model
  bucky     Whisper (whisper.cpp) backend: libs and model management
  malina    Stable Diffusion backend: libs and model management

QUICK START
  # Download a model
  $ kronk model pull unsloth/Qwen3-1.7B-UD-Q8_K_XL --local

  # Start the server with the Browser UI
  $ kronk server start --web-admin-enabled

  # Open the Browser UI in your browser
  $ open http://localhost:11435/admin/

FEATURES
  - Text generation, Vision, Audio, Embeddings, and Reranking
  - Hardware acceleration: Metal, CUDA, ROCm, Vulkan, CPU
  - Batch processing with message caching (SPC/IMC)
  - YaRN context extension for extended context windows
  - Model pooling, catalog system, and Browser UI
  - MCP service integration, authentication, and observability

OPERATING MODES
  Web mode (default)    - CLI communicates with running server at localhost:11435
  Local mode (--local)  - Direct file operations without connecting to a server

ENVIRONMENT VARIABLES
  KRONK_BASE_PATH      Base path for kronk data (models, libraries, catalog, model_config)
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

	rootCmd.PersistentFlags().String("base-path", "", "Base path for kronk data (models, libraries, catalog, model_config)")

	// Llama (kronk) backend verbs mount at the top level so the
	// existing CLI surface is unchanged.
	rootCmd.AddCommand(libs.Cmd)
	rootCmd.AddCommand(model.Cmd)
	rootCmd.AddCommand(catalog.Cmd)

	// Whisper (bucky) backend verbs mount under "bucky".
	rootCmd.AddCommand(bucky.Cmd)

	// Stable Diffusion (malina) management verbs operate locally.
	rootCmd.AddCommand(malina.Cmd)

	// Cross-backend verbs.
	rootCmd.AddCommand(server.Cmd)
	rootCmd.AddCommand(systemd.Cmd)
	rootCmd.AddCommand(security.Cmd)
	rootCmd.AddCommand(devices.Cmd)
	rootCmd.AddCommand(diagnose.Cmd)
	rootCmd.AddCommand(launch.Cmd)
}

// =============================================================================

func execute(ctx context.Context, root *cobra.Command) int {
	root.SilenceErrors = true
	root.SilenceUsage = true
	addStatus(root)
	_, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}

	if errors.Is(err, context.Canceled) {
		status.Report(ctx, "idle", "Canceled", -1)
	} else {
		msg := err.Error()
		if msg == "" {
			msg = "Command failed"
		}
		status.Report(ctx, "error", msg, -1)
	}
	if err.Error() != "" {
		fmt.Fprintln(root.ErrOrStderr(), err)
	}
	if exitErr, ok := errors.AsType[interface {
		error
		ExitCode() int
	}](err); ok {
		return exitErr.ExitCode()
	}
	return 1
}

// Only long-running operations announce work. All errors, including argument
// validation and authentication failures, are reported centrally by execute.
func addStatus(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		addStatus(child)
	}

	var msg string
	switch cmd.CommandPath() {
	case "kronk model pull", "kronk bucky model pull", "kronk malina model pull":
		msg = "Downloading model"
	case "kronk libs", "kronk bucky libs", "kronk malina libs":
		msg = "Installing libraries"
	case "kronk server start":
		msg = "Starting server"
	default:
		return
	}

	run := cmd.RunE
	if run == nil {
		return
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for _, flag := range []string{"list-combinations", "list-installs", "remove-install"} {
			if enabled, _ := cmd.Flags().GetBool(flag); enabled {
				return run(cmd, args)
			}
		}

		ctx := cmd.Context()
		detached, _ := cmd.Flags().GetBool("detach")
		foregroundServer := cmd.CommandPath() == "kronk server start" && !detached
		if !foregroundServer {
			var stop context.CancelFunc
			ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			cmd.SetContext(ctx)
		}
		status.Report(ctx, "working", msg, -1)
		if err := run(cmd, args); err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if cmd.CommandPath() == "kronk server start" {
			if foregroundServer {
				status.Report(ctx, "idle", "Server stopped", -1)
				return nil
			}
			status.Report(ctx, "done", "Server launch completed", -1)
			return nil
		}
		status.Report(ctx, "done", "Completed", -1)
		return nil
	}
}
