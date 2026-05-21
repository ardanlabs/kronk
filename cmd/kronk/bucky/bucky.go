// Package bucky provides the parent "bucky" sub-command tree, which
// hosts the whisper.cpp backend verbs (libs and model management).
//
// The verb shape mirrors the top-level llama (kronk) tree so users can
// predict it:
//
//	kronk bucky libs    — install/upgrade whisper.cpp libraries
//	kronk bucky model   — manage local whisper models (catalog, list,
//	                      pull, remove)
//
// Whisper has no chat / generation surface, so there is no "bucky run"
// verb. Every bucky verb takes a --local flag mirroring the llama
// tree; the default web mode talks to the model server's
// /v1/bucky/libs/... and /v1/bucky/models/... endpoints.
package bucky

import (
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/bucky/model"
	"github.com/spf13/cobra"
)

// Cmd is the parent "bucky" cobra command. It hosts every whisper.cpp
// backend verb. It is mounted at the top level by cmd/kronk/main.go.
var Cmd = &cobra.Command{
	Use:   "bucky",
	Short: "Whisper (whisper.cpp) backend: libs and model management",
	Long: `Whisper backend verbs (whisper.cpp via github.com/ardanlabs/bucky).

This sub-command tree mirrors the top-level llama verbs but targets the
whisper.cpp runtime instead of llama.cpp. Use it to install the whisper
shared libraries and download whisper GGML models.

COMMANDS

  libs    Install or upgrade whisper.cpp libraries
  model   Manage local whisper models (catalog, list, pull, remove)

EXAMPLES

  # Install the default whisper.cpp libraries for the current host.
  kronk bucky libs

  # Install a Linux/CUDA bundle alongside the active install.
  kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda

  # Download the tiny English whisper model.
  kronk bucky model pull tiny.en

  # List downloaded whisper models.
  kronk bucky model list

NOTES

  Whisper has no chat / generation surface, so there is no "bucky run"
  verb. The bucky verbs accept a --local flag mirroring the llama
  tree; the default web mode talks to the model server's whisper
  endpoints.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(libs.Cmd)
	Cmd.AddCommand(model.Cmd)
}
