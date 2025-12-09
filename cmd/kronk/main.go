package main

import (
	"fmt"
	"os"
	"strings"

	k "github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/list"
	"github.com/ardanlabs/kronk/cmd/kronk/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/show"
	"github.com/ardanlabs/kronk/cmd/kronk/website/api/services/kronk"
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
	Short: "Go for hardware accelerated local inference",
	Long:  "Go for hardware accelerated local inference with llama.cpp directly integrated into your applications via the yzma. Kronk provides a high-level API that feels similar to using an OpenAI compatible API.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Version = version

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
	Short:   "Start Kronk model server",
	Long:    `Start Kronk model server. Use --help to get environment settings`,
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

var libsCmd = &cobra.Command{
	Use:   "libs",
	Short: "Install or upgrade llama.cpp libraries",
	Long: `Install or upgrade llama.cpp libraries

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_ARCH       (default: runtime.GOARCH)         The architecture to install.
      KRONK_LIB_PATH   (default: $HOME/kronk/libraries)  The path to the libraries directory,
      KRONK_OS         (default: runtime.GOOS)           The operating system to install.
      KRONK_PROCESSOR  (default: cpu)                    Options: cpu, cuda, metal, vulkan`,
	Args: cobra.NoArgs,
	Run:  runLibs,
}

func init() {

}

func runLibs(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = libs.RunLocal(args)
	default:
		err = libs.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List models",
	Long: `List models

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server

Environment Variables (--local mode):
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.NoArgs,
	Run:  runList,
}

func init() {
	listCmd.Flags().Bool("local", false, "Run without the model server")
}

func runList(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = list.RunLocal(args)
	default:
		err = list.RunWeb(args)
	}

	if err != nil {
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

var pullCmd = &cobra.Command{
	Use:   "pull <MODEL_URL> [MMPROJ_URL]",
	Short: "Pull a model from the web",
	Long: `Pull a model from the web, the mmproj file is optional

Environment Variables (--local mode):
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.RangeArgs(1, 2),
	Run:  runPull,
}

func init() {
	pullCmd.Flags().Bool("local", false, "Run without the model server")
}

func runPull(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = pull.RunLocal(args)
	default:
		err = pull.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var removeCmd = &cobra.Command{
	Use:   "remove MODEL_NAME",
	Short: "Remove a model",
	Long: `Remove a model

Environment Variables (--local mode):
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  runRemove,
}

func init() {
	removeCmd.Flags().Bool("local", false, "Run without the model server")
}

func runRemove(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = remove.RunLocal(args)
	default:
		err = remove.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================

var showCmd = &cobra.Command{
	Use:   "show <MODEL_NAME>",
	Short: "Show information for a model",
	Long: `Show information for a model

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server

Environment Variables (--local mode):
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
	Args: cobra.ExactArgs(1),
	Run:  runShow,
}

func init() {
	showCmd.Flags().Bool("local", false, "Run without the model server")
}

func runShow(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = show.RunLocal(args)
	default:
		err = show.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}
