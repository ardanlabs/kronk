package main

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/install"
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
	Short: "Large language model runner",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Flags().BoolP("version", "v", false, "Show version information")
	rootCmd.SetVersionTemplate(version)

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(rmCmd)
}

var serveCmd = &cobra.Command{
	Use:     "serve",
	Aliases: []string{"start"},
	Short:   "Start kronk",
	Long: `Start kronk

Environment Variables:
      KRONK_HOST                  (default: 127.0.0.1:11434) IP Address for the kronk server 
      KRONK_MODELS                (default: ./models)        The path to the models directory
      KRONK_LLM_LIBRARY           (default: ./libraries)     Set LLM library to bypass autodetection
      KRONK_DEVICE                (default: autodetection)   Device to use for inference 
      KRONK_MODEL_INSTANCES       (default: 1)               Maximum number of parallel requests
      KRONK_MODEL_CONTEXT_WINDOW  (default: 4096)            Context window to use for inference 
      KRONK_MODEL_NBatch          (default: 2048)            Logical batch size or the maximum number of tokens that can be in a single forward pass through the model at any given time
      KRONK_MODEL_NUBatch         (default: 512)             Physical batch size or the maximum number of tokens processed together during the initial prompt processing phase (also called "prompt ingestion") to populate the KV cache
      KRONK_MODEL_NThreads        (default: llama.cpp)       Number of threads to use for generation
      KRONK_MODEL_NThreadsBatch   (default: llama.cpp)       Number of threads to use for batch processing`,
	Run: runServe,
}

var pullCmd = &cobra.Command{
	Use:   "pull MODEL_URL",
	Short: "Pull a model from a registry",
	Long: `Pull a model from a registry

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List models",
	Long: `List models

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Run: runList,
}

var showCmd = &cobra.Command{
	Use:   "show MODEL_NAME",
	Short: "Show information for a model",
	Long: `Show information for a model

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Args: cobra.ExactArgs(1),
	Run:  runShow,
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List running models",
	Long: `List running models

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Run: runPs,
}

var rmCmd = &cobra.Command{
	Use:   "rm MODEL_NAME",
	Short: "Remove a model",
	Long: `Remove a model

Environment Variables:
      KRONK_HOST  IP Address for the kronk server (default 127.0.0.1:11434)`,
	Args: cobra.ExactArgs(1),
	Run:  runRm,
}

func runServe(cmd *cobra.Command, args []string) {
	fmt.Println("serve command not implemented")
}

func runPull(cmd *cobra.Command, args []string) {
	f := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		fmt.Printf("\r\x1b[KDownloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec)
		if complete {
			fmt.Println()
		}
	}

	url := args[0]
	_, downloaded, err := install.ModelWithProgress(url, "./models", f)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}

	if !downloaded {
		fmt.Println("Already downloaded")
		return
	}

	fmt.Println("Download Completed")
}

func runList(cmd *cobra.Command, args []string) {
	fmt.Println("list command not implemented")
}

func runShow(cmd *cobra.Command, args []string) {
	fmt.Println("show command not implemented")
}

func runPs(cmd *cobra.Command, args []string) {
	fmt.Println("ps command not implemented")
}

func runRm(cmd *cobra.Command, args []string) {
	fmt.Println("rm command not implemented")
}
