package main

import (
	"fmt"
	"os"
	"strings"

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

	// Pull the environment settings from the model server.
	if len(os.Args) >= 3 {
		if os.Args[1] == "server" && strings.Contains(os.Args[2], "help") {
			err := kronk.Run(true)
			serverCmd = &cobra.Command{
				Use:     "server",
				Aliases: []string{"start"},
				Short:   "Start kronk server",
				Long:    fmt.Sprintf("Start kronk server\n\n%s", err.Error()),
				Args:    cobra.NoArgs,
				Run:     runServer,
			}
		}
	}

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(libsLocalCmd)
	rootCmd.AddCommand(libsWebCmd)
	rootCmd.AddCommand(listLocalCmd)
	rootCmd.AddCommand(listWebCmd)
	rootCmd.AddCommand(pullLocalCmd)
	rootCmd.AddCommand(removeLocalCmd)
	rootCmd.AddCommand(showWebCmd)
	rootCmd.AddCommand(showLocalCmd)
	rootCmd.AddCommand(psCmd)
}

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"start"},
	Short:   "Start kronk server",
	Long:    `Start kronk server. Use --help to get environment settings`,
	Args:    cobra.NoArgs,
	Run:     runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	if err := kronk.Run(false); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var libsWebCmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade llama.cpp libraries",
	Long: `Install or upgrade llama.cpp libraries

Environment Variables:
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server.`,
	Args: cobra.NoArgs,
	Run:  runLibsWeb,
}

func runLibsWeb(cmd *cobra.Command, args []string) {
	if err := libs.RunWeb(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var libsLocalCmd = &cobra.Command{
	Use:   "libs-local",
	Short: "Install or upgrade llama.cpp libraries without running the model server",
	Long: `Install or upgrade llama.cpp libraries without running the model server

Environment Variables:
      KRONK_ARCH       (default: runtime.GOARCH)         The architecture to install.
      KRONK_LIB_PATH   (default: $HOME/kronk/libraries)  The path to the libraries directory,
      KRONK_OS         (default: runtime.GOOS)           The operating system to install.
      KRONK_PROCESSOR  (default: cpu)                    Options: cpu, cuda, metal, vulkan`,
	Args: cobra.NoArgs,
	Run:  runLibsLocal,
}

func runLibsLocal(cmd *cobra.Command, args []string) {
	if err := libs.RunLocal(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var listWebCmd = &cobra.Command{
	Use:   "list",
	Short: "List models",
	Long: `List models

Environment Variables:
	  KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.NoArgs,
	Run:  runListWeb,
}

func runListWeb(cmd *cobra.Command, args []string) {
	if err := list.RunWeb(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var listLocalCmd = &cobra.Command{
	Use:   "list-local",
	Short: "List models",
	Long: `List models

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.NoArgs,
	Run:  runListLocal,
}

func runListLocal(cmd *cobra.Command, args []string) {
	if err := list.RunLocal(args); err != nil {
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
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Run: runPs,
}

func runPs(cmd *cobra.Command, args []string) {
	fmt.Println("ps command not implemented")
}

// =============================================================================

var pullLocalCmd = &cobra.Command{
	Use:   "pull-local <MODEL_URL> <MMPROJ_URL>",
	Short: "Pull a model from the web without running the model server, the mmproj file is optional without running the model server",
	Long: `Pull a model from the web without running the model server, the mmproj file is optional

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.RangeArgs(1, 2),
	Run:  runPullLocal,
}

func runPullLocal(cmd *cobra.Command, args []string) {
	if err := pull.RunLocal(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var removeLocalCmd = &cobra.Command{
	Use:   "remove-local MODEL_NAME",
	Short: "Remove a model",
	Long: `Remove a model

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveLocal,
}

func runRemoveLocal(cmd *cobra.Command, args []string) {
	if err := remove.RunLocal(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var showWebCmd = &cobra.Command{
	Use:   "show <MODEL_NAME>",
	Short: "Show information for a model",
	Long: `Show information for a model

Environment Variables:
	  KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.ExactArgs(1),
	Run:  runShowWeb,
}

func runShowWeb(cmd *cobra.Command, args []string) {
	if err := show.RunWeb(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var showLocalCmd = &cobra.Command{
	Use:   "show-local <MODEL_NAME>",
	Short: "Show information for a model",
	Long: `Show information for a model

Environment Variables:
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  runShowLocal,
}

func runShowLocal(cmd *cobra.Command, args []string) {
	if err := show.RunLocal(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}
