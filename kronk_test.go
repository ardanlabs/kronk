package kronk_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/hybridgroup/yzma/pkg/download"
	"golang.org/x/sync/errgroup"
)

var (
	modelChatCompletionsFile = "models/qwen2.5-0.5b-instruct-q8_0.gguf"
	modelChatVisionFile      = "models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projChatVisionFile       = "models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	modelEmbedFile           = "models/embeddinggemma-300m-qat-Q8_0.gguf"
)

var (
	gw          = os.Getenv("GITHUB_WORKSPACE")
	libPath     = filepath.Join(gw, "libraries")
	modelPath   = filepath.Join(gw, "models")
	imageFile   = filepath.Join(gw, "samples", "giraffe.jpg")
	concurrency = 1
)

func TestMain(m *testing.M) {
	fmt.Println("libpath        :", libPath)
	fmt.Println("modelPath      :", modelPath)
	fmt.Println("imageFile      :", imageFile)
	fmt.Println("LD_LIBRARY_PATH:", os.Getenv("LD_LIBRARY_PATH"))
	fmt.Println("YZMA_LIB       :", os.Getenv("YZMA_LIB"))

	if os.Getenv("CONCURRENCY") != "" {
		var err error
		concurrency, err = strconv.Atoi(os.Getenv("CONCURRENCY"))
		if err != nil {
			concurrency = 1
		}
	}

	fmt.Println("CONCURRENCY    :", concurrency)

	fmt.Println("LIBRARIES      :")
	kronk.InstallLlama(libPath, download.CPU, false)
	if err := filepath.Walk(libPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Println("lib:", path)
		return nil
	}); err != nil {
		fmt.Printf("error walking model path: %v\n", err)
	}

	fmt.Println("MODELS         :")
	if err := filepath.Walk(modelPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Println("Model:", path)
		return nil
	}); err != nil {
		fmt.Printf("error walking model path: %v\n", err)
	}

	os.Exit(m.Run())
}

func TestChatCompletions(t *testing.T) {
	modelFile := modelChatCompletionsFile

	// -------------------------------------------------------------------------

	llm, err := kronk.New(concurrency, libPath, modelFile, kronk.Config{
		LogSet:        kronk.LogSilent,
		ContextWindow: 1024 * 4,
	})
	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	question := "Echo back the word: Gorilla"

	messages := []kronk.ChatMessage{
		{
			Role:    "user",
			Content: question,
		},
	}

	params := kronk.Params{
		TopK: 1.0,
		TopP: 0.9,
		Temp: 0.7,
	}

	// -------------------------------------------------------------------------

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		ch, err := llm.ChatCompletions(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat completions: %w", err)
		}

		var finalResponse strings.Builder
		for msg := range ch {
			if msg.Err != nil {
				return fmt.Errorf("error from model: %w", msg.Err)
			}
			finalResponse.WriteString(msg.Response)
		}

		find := "Gorilla"
		if !strings.Contains(finalResponse.String(), find) {
			return fmt.Errorf("expected %q, got %q", find, finalResponse.String())
		}

		return nil
	}

	var g errgroup.Group
	for range concurrency {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestChatVision(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping test since it takes too long to run")
	}

	modelFile := modelChatVisionFile
	projFile := projChatVisionFile

	// -------------------------------------------------------------------------

	cfg := kronk.Config{
		ContextWindow: 1024 * 4,
	}

	llm, err := kronk.New(concurrency, libPath, modelFile, cfg, kronk.WithProjection(projFile))
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	question := "What is in this picture?"

	message := kronk.ChatMessage{
		Role:    "user",
		Content: question,
	}

	params := kronk.Params{
		TopK: 1.0,
		TopP: 0.9,
		Temp: 0.7,
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		ch, err := llm.ChatVision(ctx, message, imageFile, params)
		if err != nil {
			return fmt.Errorf("chat vision: %w", err)
		}

		var finalResponse strings.Builder
		for msg := range ch {
			if msg.Err != nil {
				return fmt.Errorf("error from model: %w", msg.Err)
			}
			finalResponse.WriteString(msg.Response)
		}

		find := "giraffes"
		if !strings.Contains(finalResponse.String(), find) {
			return fmt.Errorf("expected %q, got %q", find, finalResponse.String())
		}

		return nil
	}

	var g errgroup.Group
	for range concurrency {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestEmbedding(t *testing.T) {
	modelFile := modelEmbedFile

	// -------------------------------------------------------------------------

	cfg := kronk.Config{
		ContextWindow: 1024 * 4,
		Embeddings:    true,
	}

	llm, err := kronk.New(concurrency, libPath, modelFile, cfg)
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	text := "Embed this sentence"

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		embed, err := llm.Embed(ctx, text)
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}

		if embed[0] == 0 || embed[len(embed)-1] == 0 {
			return fmt.Errorf("expected to have values in the embedding")
		}

		return nil
	}

	var g errgroup.Group
	for range concurrency {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
