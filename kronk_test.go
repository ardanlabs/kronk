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
	modelChatFile   = "models/qwen2.5-0.5b-instruct-q8_0.gguf"
	modelVisionFile = "models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projVisionFile  = "models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	modelEmbedFile  = "models/embeddinggemma-300m-qat-Q8_0.gguf"
)

var (
	gw          = os.Getenv("GITHUB_WORKSPACE")
	libPath     = filepath.Join(gw, "libraries")
	modelPath   = filepath.Join(gw, "models")
	imageFile   = filepath.Join(gw, "images/samples", "giraffe.jpg")
	concurrency = 1
)

func TestMain(m *testing.M) {
	fmt.Println("libpath        :", libPath)
	fmt.Println("modelPath      :", modelPath)
	fmt.Println("imageFile      :", imageFile)
	fmt.Println("LD_LIBRARY_PATH:", os.Getenv("LD_LIBRARY_PATH"))

	if os.Getenv("CONCURRENCY") != "" {
		var err error
		concurrency, err = strconv.Atoi(os.Getenv("CONCURRENCY"))
		if err != nil {
			concurrency = 1
		}
	}

	fmt.Println("CONCURRENCY    :", concurrency)

	if os.Getenv("INSTALL_LLAMA") == "1" {
		vi, err := kronk.RetrieveVersionInfo(libPath)
		if err != nil {
			fmt.Printf("Failed to retrieve version info: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Latest version : %s, Current version: %s\n", vi.Latest, vi.Current)

		if vi.Current != vi.Latest {
			fmt.Printf("LIBRARIES      : Installing at %s\n", libPath)
			_, err := kronk.InstallLlama(libPath, download.CPU, true)
			if err != nil {
				fmt.Printf("Failed to install llama: %s: error: %s\n", libPath, err)
				os.Exit(1)
			}

			if err := filepath.Walk(libPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				fmt.Println("lib:", path)
				return nil
			}); err != nil {
				fmt.Printf("error walking model path: %v\n", err)
			}
		}
	}

	fmt.Println("MODELS:")
	if err := filepath.Walk(modelPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Println("Model:", path)
		return nil
	}); err != nil {
		fmt.Printf("error walking model path: %v\n", err)
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		fmt.Printf("Failed to init the llamacpp library: %s: error: %s\n", libPath, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// =============================================================================

func initChatTest(t *testing.T) (*kronk.Kronk, []kronk.ChatMessage, kronk.Params) {
	modelFile := modelChatFile

	// -------------------------------------------------------------------------

	krn, err := kronk.New(concurrency, modelFile, "", kronk.ModelConfig{})
	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}

	// -------------------------------------------------------------------------

	question := "Echo back the word: Gorilla"

	messages := []kronk.ChatMessage{
		{
			Role:    "user",
			Content: question,
		},
	}

	return krn, messages, kronk.Params{}
}

func TestChat(t *testing.T) {
	krn, messages, params := initChatTest(t)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		response, err := krn.Chat(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		find := "Gorilla"
		if !strings.Contains(response, find) {
			return fmt.Errorf("expected %q, got %q", find, response)
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

func TestChatStreaming(t *testing.T) {
	krn, messages, params := initChatTest(t)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		ch, err := krn.ChatStreaming(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
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

// =============================================================================

func initVisionTest(t *testing.T) (*kronk.Kronk, kronk.ChatMessage, kronk.Params) {
	if runtime.GOOS == "darwin" && os.Getenv("RUN_MACOS") == "" {
		t.Skip("skipping test since it takes too long to run")
	}

	modelFile := modelVisionFile
	projFile := projVisionFile

	// -------------------------------------------------------------------------

	krn, err := kronk.New(concurrency, modelFile, projFile, kronk.ModelConfig{})
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}

	// -------------------------------------------------------------------------

	question := "What is in this picture?"

	message := kronk.ChatMessage{
		Role:    "user",
		Content: question,
	}

	return krn, message, kronk.Params{}
}

func TestVision(t *testing.T) {
	krn, message, params := initVisionTest(t)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		response, err := krn.Vision(ctx, message, imageFile, params)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
		}

		find := "giraffes"
		if !strings.Contains(response, find) {
			return fmt.Errorf("expected %q, got %q", find, response)
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

func TestVisionStreaming(t *testing.T) {
	krn, message, params := initVisionTest(t)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		ch, err := krn.VisionStreaming(ctx, message, imageFile, params)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
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

	cfg := kronk.ModelConfig{
		Embeddings: true,
	}

	krn, err := kronk.New(concurrency, modelFile, "", cfg)
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer krn.Unload()

	// -------------------------------------------------------------------------

	text := "Embed this sentence"

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		embed, err := krn.Embed(ctx, text)
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
