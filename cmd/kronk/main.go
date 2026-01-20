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
	Short: "Go for hardware accelerated local inference",
	Long: "Go for hardware accelerated local inference with llama.cpp directly integrated\n" +
		"into your applications via the yzma.\n" +
		"Kronk provides a high-level API that feels similar to using an OpenAI compatible API.",
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
