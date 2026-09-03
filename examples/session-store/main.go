// This example shows how SDK users can implement and inject a custom IMC
// session-store factory. The included disk store is intentionally temporary:
// it creates anonymous files and removes them on Close. It demonstrates the
// extension contract, not durable session persistence.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-session-store

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelSource = "unsloth/Qwen3-0.6B-Q8_0"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	storeDir, err := os.MkdirTemp("", "kronk-session-store-")
	if err != nil {
		return fmt.Errorf("create session-store directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(storeDir); err != nil {
			fmt.Printf("remove session-store directory: %v\n", err)
		}
	}()

	factory, err := NewFactory(context.Background(), storeDir)
	if err != nil {
		return fmt.Errorf("construct session-store factory: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
		model.WithIncrementalCache(true),
		model.WithSessionStoreFactory(factory),

		// Need this to trigger session usage for this
		// small example.
		model.WithCacheMinTokens(5),
	)
	if err != nil {
		return fmt.Errorf("create kronk: %w", err)
	}
	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("unload model: %v\n", err)
		}
	}()

	fmt.Println("temporary session files:", storeDir)
	return askQuestion(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libraryManager, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libraryManager.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("install llama.cpp: %w", err)
	}
	if err := kronk.Init(kronk.WithLibPath(libraryManager.LibsPath())); err != nil {
		return models.Path{}, fmt.Errorf("initialize kronk: %w", err)
	}

	modelManager, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("initialize models: %w", err)
	}

	mp, err := modelManager.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("install model: %w", err)
	}

	return mp, nil
}

func askQuestion(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	doc := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, "Explain custom session storage in one sentence."),
		),
		"max_tokens": 128,
	}

	ch, err := krn.ChatStreaming(ctx, doc)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	for resp := range ch {
		if resp.Choices[0].FinishReason() == model.FinishReasonError {
			return fmt.Errorf("model: %s", resp.Choices[0].Delta.Content)
		}
		fmt.Print(resp.Choices[0].Delta.Content)
	}
	fmt.Println()

	return nil
}
