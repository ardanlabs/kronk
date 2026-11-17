// This example provides a terminal chat with tools, images, and saved IMC conversations.
// Run the entire package: make example-chat (or go run ./chat from examples).
// See README.md for controls, vision setup, and replacing the storage backend.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/storage/disk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Match the agent example, including its tool-capable vision model.
const modelSource = "ornith-ai/Ornith-1.5-9B-Q4_K_M"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run() (retErr error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	modelPath := flag.String("model", "", "path to an installed GGUF model")
	source := flag.String("model-source", modelSource, "model to locate or download when -model is omitted")
	projector := flag.String("projector", "", "matching vision projector GGUF; enables image attachments")
	directory := flag.String("directory", filepath.Join(home, ".kronk", "examples", "chat"), "private snapshot directory")
	logPath := flag.String("log", filepath.Join(home, ".kronk", "examples", "chat.log"), "storage and model diagnostic log file")
	workspace := flag.String("workspace", ".", "directory accessible to file tools; writes require confirmation")
	libPath := flag.String("lib-path", "", "optional llama.cpp library directory")
	window := flag.Int("context", 65536, "model context window; keep unchanged when testing checkpoint reuse")
	flag.Parse()
	*logPath, err = filepath.Abs(*logPath)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(*workspace)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()

	if err := os.MkdirAll(filepath.Dir(*logPath), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	logger := log.New(f, "", log.LstdFlags|log.LUTC|log.Lmicroseconds)
	logFunc := func(_ context.Context, operation string, args ...any) {
		logger.Printf("%s %v", operation, args)
	}

	// Replace this constructor with your own imc.Store. The UI uses only that
	// public contract. Both log callbacks write to the file, never to the TUI.
	store, err := disk.New(*directory, disk.WithLog(logFunc))
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, store.Close()) }()
	// Clear only after acquiring the backend's directory lock, so a second
	// app using the same snapshots cannot erase the active app's log.
	if err := f.Truncate(0); err != nil {
		return err
	}
	logger.Printf("chat-start model_source=%q model_path=%q projector=%q context=%d workspace=%q storage=%q",
		*source, *modelPath, *projector, *window, *workspace, *directory)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if *libPath == "" {
		manager, err := libs.New(libs.WithDetect(ctx, kronk.FmtLogger), libs.WithValidation(true))
		if err != nil {
			return err
		}
		if _, err := manager.Download(ctx, kronk.FmtLogger); err != nil {
			return err
		}
		*libPath = manager.LibsPath()
	}
	if err := kronk.Init(kronk.WithLibPath(*libPath)); err != nil {
		return err
	}
	files := []string{*modelPath}
	if *modelPath == "" {
		manager, err := models.New()
		if err != nil {
			return err
		}
		paths, err := manager.Download(ctx, kronk.FmtLogger, *source)
		if err != nil {
			return err
		}
		files = paths.ModelFiles
		if *projector == "" {
			*projector = paths.ProjFile
		}
	}
	fmt.Println("Loading model. Diagnostic log:", *logPath)
	krn, err := kronk.New(
		model.WithLog(logFunc),
		model.WithModelFiles(files), model.WithProjFile(*projector), model.WithAutoTune(true),
		model.WithContextWindow(*window), model.WithNSeqMax(1),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithSpeculationMode(model.SpeculationDisabled),
		model.WithStorage(model.StorageConfig{Store: store}))
	if err != nil {
		return err
	}
	// Unload before closing storage. The TUI drains each stream even on cancel.
	defer func() { retErr = errors.Join(retErr, krn.Unload(context.Background())) }()

	app := newChat(krn, store, root, *projector != "")
	app.logPath = *logPath
	_, err = tea.NewProgram(app, tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
		// Signals must take the same cancellation/drain path as Ctrl+Q;
		// Bubble Tea otherwise handles these before Update sees them.
		switch msg.(type) {
		case tea.InterruptMsg:
			return tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
		case tea.QuitMsg:
			if m.(*chat).busy {
				return tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
			}
		}
		return msg
	})).Run()
	return err
}
