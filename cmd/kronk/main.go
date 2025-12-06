package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/list"
	"github.com/ardanlabs/kronk/cmd/kronk/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/show"
	"github.com/ardanlabs/kronk/cmd/kronk/website/api/services/kronk"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "kronk",
	Short: "Go for hardware accelerated local inference",
	Long:  "Go for hardware accelerated local inference with llama.cpp directly integrated into your applications via the yzma. Kronk provides a high-level API that feels similar to using an OpenAI compatible API.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")
	rootCmd.SetVersionTemplate(version)

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(libsCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(psCmd)
}

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"start"},
	Short:   "Start kronk server",
	Long: `Start kronk server

Environment Variables:
      KRONK_WEB_API_HOST          (default: 0.0.0.0:3000)        IP Address for the app endpoints 
	  KRONK_WEB_DEBUG_HOST        (default: 0.0.0.0:3010)        IP Address for the debug endpoints
      KRONK_MODELS                (default: $HOME/kronk/models)  The path to the models directory
	  KRONK_PROCESSOR             (default: cpu)                 Options: cpu, cuda, metal, vulkan
      KRONK_DEVICE                (default: autodetection)       Device to use for inference 
      KRONK_MODEL_INSTANCES       (default: 1)                   Maximum number of parallel requests
      KRONK_MODEL_CONTEXT_WINDOW  (default: 4096)                Context window to use for inference 
      KRONK_MODEL_NBatch          (default: 2048)                Logical batch size or the maximum number of tokens that can be in a single forward pass through the model at any given time
      KRONK_MODEL_NUBatch         (default: 512)                 Physical batch size or the maximum number of tokens processed together during the initial prompt processing phase (also called "prompt ingestion") to populate the KV cache
      KRONK_MODEL_NThreads        (default: llama.cpp)           Number of threads to use for generation
      KRONK_MODEL_NThreadsBatch   (default: llama.cpp)           Number of threads to use for batch processing`,
	Args: cobra.NoArgs,
	Run:  runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	if err := kronk.Run(); err != nil {
		if errors.Is(err, libs.ErrInvalidArguments) {
			cmd.Help()
			os.Exit(1)
		}

		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var libsCmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade llama.cpp libraries",
	Long: `Install or upgrade llama.cpp libraries

Environment Variables:
      KRONK_PROCESSOR  (default: cpu)  Options: cpu, cuda, metal, vulkan`,
	Args: cobra.NoArgs,
	Run:  runLibs,
}

func runLibs(cmd *cobra.Command, args []string) {
	if err := libs.Run(args); err != nil {
		if errors.Is(err, libs.ErrInvalidArguments) {
			cmd.Help()
			os.Exit(1)
		}

		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List models",
	Long: `List models

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.NoArgs,
	Run:  runList,
}

func runList(cmd *cobra.Command, args []string) {
	if err := list.Run(args); err != nil {
		if errors.Is(err, list.ErrInvalidArguments) {
			cmd.Help()
			os.Exit(1)
		}

		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running models",
	Long: `List running models

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Run: runPs,
}

func runPs(cmd *cobra.Command, args []string) {
	fmt.Println("ps command not implemented")
}

// =============================================================================

var pullCmd = &cobra.Command{
	Use:   "pull <MODEL_URL> <MMPROJ_URL>",
	Short: "Pull a model from the web, the mmproj file is optional",
	Long: `Pull a model from the web, the mmproj file is optional

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.RangeArgs(1, 2),
	Run:  runPull,
}

func runPull(cmd *cobra.Command, args []string) {
	if err := pull.Run(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var removeCmd = &cobra.Command{
	Use:   "remove MODEL_NAME",
	Short: "Remove a model",
	Long: `Remove a model

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Args: cobra.ExactArgs(1),
	Run:  runRemove,
}

func runRemove(cmd *cobra.Command, args []string) {
	if err := remove.Run(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var showCmd = &cobra.Command{
	Use:   "show <MODEL_NAME>",
	Short: "Show information for a model",
	Long: `Show information for a model

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  runShow,
}

func runShow(cmd *cobra.Command, args []string) {
	if err := show.Run(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}
