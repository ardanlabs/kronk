package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	k "github.com/ardanlabs/kronk"
	cataloglist "github.com/ardanlabs/kronk/cmd/kronk/catalog/list"
	catalogpull "github.com/ardanlabs/kronk/cmd/kronk/catalog/pull"
	catalogshow "github.com/ardanlabs/kronk/cmd/kronk/catalog/show"
	catalogupdate "github.com/ardanlabs/kronk/cmd/kronk/catalog/update"
	"github.com/ardanlabs/kronk/cmd/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/model/list"
	"github.com/ardanlabs/kronk/cmd/kronk/model/ps"
	"github.com/ardanlabs/kronk/cmd/kronk/model/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/model/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/model/show"
	"github.com/ardanlabs/kronk/cmd/kronk/website/api/services/kronk"
	"github.com/ardanlabs/kronk/defaults"
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

	serverCmd.Flags().BoolP("detach", "d", false, "Run server in the background")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(libsCmd)
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(catalogCmd)

	// Model subcommands
	modelCmd.AddCommand(listCmd)
	modelCmd.AddCommand(pullCmd)
	modelCmd.AddCommand(removeCmd)
	modelCmd.AddCommand(showCmd)
	modelCmd.AddCommand(psCmd)

	// Catalog subcommands
	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogPullCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	catalogCmd.AddCommand(catalogUpdateCmd)
}

// =============================================================================
// Model

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models",
	Long:  `Manage models - list, pull, remove, show, and check running models`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// =============================================================================
// Catalog

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage model catalog",
	Long:  `Manage model catalog - list and update available models`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var catalogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog models",
	Long: `List catalog models

Flags (--local mode):
      --filter-category  Filter catalogs by category name (substring match)

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.ArbitraryArgs,
	Run:  runCatalogList,
}

func init() {
	catalogListCmd.Flags().Bool("local", false, "Run without the model server")
	catalogListCmd.Flags().String("filter-category", "", "Filter catalogs by category name (substring match)")
}

func runCatalogList(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")
	filterCategory, _ := cmd.Flags().GetString("filter-category")

	if filterCategory != "" {
		args = append(args, "--filter-category", filterCategory)
	}

	var err error

	switch local {
	case true:
		err = cataloglist.RunLocal(args)
	default:
		err = cataloglist.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var catalogPullCmd = &cobra.Command{
	Use:   "pull <MODEL_ID>",
	Short: "Pull a model from the catalog",
	Long: `Pull a model from the catalog

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.ExactArgs(1),
	Run:  runCatalogPull,
}

func init() {
	catalogPullCmd.Flags().Bool("local", false, "Run without the model server")
}

func runCatalogPull(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = catalogpull.RunLocal(args)
	default:
		err = catalogpull.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var catalogShowCmd = &cobra.Command{
	Use:   "show <MODEL_ID>",
	Short: "Show catalog model information",
	Long: `Show catalog model information

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.ExactArgs(1),
	Run:  runCatalogShow,
}

func init() {
	catalogShowCmd.Flags().Bool("local", false, "Run without the model server")
}

func runCatalogShow(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = catalogshow.RunLocal(args)
	default:
		err = catalogshow.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

var catalogUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the model catalog",
	Long: `Update the model catalog

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Args: cobra.NoArgs,
	Run:  runCatalogUpdate,
}

func init() {
	catalogUpdateCmd.Flags().Bool("local", false, "Run without the model server")
}

func runCatalogUpdate(cmd *cobra.Command, args []string) {
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = catalogupdate.RunLocal(args)
	default:
		err = catalogupdate.RunWeb(args)
	}

	if err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================
// Server

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"start"},
	Short:   "Start Kronk model server",
	Long:    `Start Kronk model server. Use --help to get environment settings`,
	Args:    cobra.NoArgs,
	Run:     runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	detach, _ := cmd.Flags().GetBool("detach")

	if detach {
		exePath, err := os.Executable()
		if err != nil {
			fmt.Println("\nERROR:", err)
			os.Exit(1)
		}

		logFile, _ := os.Create(logFilePath())

		proc := exec.Command(exePath, "server")
		proc.Stdout = logFile
		proc.Stderr = logFile
		proc.Stdin = nil
		proc.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

		if err := proc.Start(); err != nil {
			fmt.Println("\nERROR:", err)
			os.Exit(1)
		}

		pidFile := pidFilePath()
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(proc.Process.Pid)), 0644); err != nil {
			fmt.Println("\nERROR: failed to write pid file:", err)
			os.Exit(1)
		}

		fmt.Printf("Kronk server started in background (PID: %d)\n", proc.Process.Pid)
		return
	}

	if err := kronk.Run(false); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

func logFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.log")
}

// =============================================================================
// STOP

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running Kronk model server",
	Long:  `Stop the running Kronk model server by sending SIGTERM`,
	Args:  cobra.NoArgs,
	Run:   runStop,
}

func runStop(cmd *cobra.Command, args []string) {
	pidFile := pidFilePath()

	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("ERROR: no running server found (pid file not found)")
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("ERROR: invalid pid file")
		os.Exit(1)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("ERROR: could not find process:", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Println("ERROR: failed to send signal:", err)
		os.Exit(1)
	}

	os.Remove(pidFile)
	fmt.Printf("Sent SIGTERM to Kronk server (PID: %d)\n", pid)
}

func pidFilePath() string {
	return filepath.Join(defaults.BaseDir(""), "kronk.pid")
}

// =============================================================================
// LOGS

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream server logs",
	Long:  `Stream the Kronk model server logs (tail -f)`,
	Args:  cobra.NoArgs,
	Run:   runLogs,
}

func runLogs(cmd *cobra.Command, args []string) {
	logFile := logFilePath()

	tail := exec.Command("tail", "-f", logFile)
	tail.Stdout = os.Stdout
	tail.Stderr = os.Stderr

	if err := tail.Run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================
// LIBS

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
	libsCmd.Flags().Bool("local", false, "Run without the model server")
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
// LIST

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
// PS

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running models",
	Long: `List running models

Environment Variables:
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server`,
	Run: runPs,
}

func runPs(cmd *cobra.Command, args []string) {
	if err := ps.RunWeb(args); err != nil {
		fmt.Println("\nERROR:", err)
		os.Exit(1)
	}
}

// =============================================================================
// PULL

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
// REMOVE

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
// SHOW

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
