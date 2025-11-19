package llamacpp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/llamacpp"
)

var (
	modelChatCompletionsFile = "models/qwen2.5-0.5b-instruct-fp16.gguf"
	modelChatVisionFile      = "models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projChatVisionFile       = "models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	modelEmbedFile           = "models/embeddinggemma-300m-qat-Q8_0.gguf"
)

var (
	libPath     = "libraries"
	modelPath   = "models"
	imageFile   = "samples/giraffe.jpg"
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

func testChatCompletions(t *testing.T) {
	modelFile := modelChatCompletionsFile

	// -------------------------------------------------------------------------

	llm, err := llamacpp.New(concurrency, libPath, modelFile, llamacpp.Config{
		ContextWindow: 1024 * 32,
	})
	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	question := "Echo back the word: Gorilla"

	messages := []llamacpp.ChatMessage{
		{
			Role:    "user",
			Content: question,
		},
	}

	params := llamacpp.Params{
		TopK: 1.0,
		TopP: 0.9,
		Temp: 0.7,
	}

	f := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		ch, err := llm.ChatCompletions(ctx, messages, params)
		if err != nil {
			t.Fatalf("chat completions: %v", err)
		}

		var finalResponse strings.Builder
		for msg := range ch {
			if msg.Err != nil {
				t.Fatalf("error from model: %v", msg.Err)
			}
			finalResponse.WriteString(msg.Response)
		}

		find := "Gorilla"
		if !strings.Contains(finalResponse.String(), find) {
			t.Fatalf("expected %q, got %q", find, finalResponse.String())
		}
	}

	g := concurrency
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}

func TestChatVision(t *testing.T) {
	modelFile := modelChatVisionFile
	projFile := projChatVisionFile

	// -------------------------------------------------------------------------

	cfg := llamacpp.Config{
		LogSet:        llamacpp.LogSilent,
		ContextWindow: 1024 * 32,
	}

	llm, err := llamacpp.New(concurrency, libPath, modelFile, cfg, llamacpp.WithProjection(projFile))
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	question := "What is in this picture?"

	message := llamacpp.ChatMessage{
		Role:    "user",
		Content: question,
	}

	params := llamacpp.Params{
		TopK: 1.0,
		TopP: 0.9,
		Temp: 0.7,
	}

	f := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		ch, err := llm.ChatVision(ctx, message, imageFile, params)
		if err != nil {
			t.Fatalf("chat vision: %v", err)
		}

		var finalResponse strings.Builder
		for msg := range ch {
			if msg.Err != nil {
				t.Fatalf("error from model: %v", msg.Err)
			}
			finalResponse.WriteString(msg.Response)
		}

		find := "giraffes"
		if !strings.Contains(finalResponse.String(), find) {
			t.Fatalf("expected %q, got %q", find, finalResponse.String())
		}
	}

	g := concurrency
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}

func TestEmbedding(t *testing.T) {
	modelFile := modelEmbedFile

	// -------------------------------------------------------------------------

	cfg := llamacpp.Config{
		LogSet:        llamacpp.LogSilent,
		ContextWindow: 1024 * 32,
		Embeddings:    true,
	}

	llm, err := llamacpp.New(concurrency, libPath, modelFile, cfg)
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	text := "Embed this sentence"

	f := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		queryVector, err := llm.Embed(ctx, text)
		if err != nil {
			t.Fatalf("embed: %v", err)
		}

		first := float32(0.067838)
		last := float32(0.02118274)

		if queryVector[0] != first || queryVector[len(queryVector)-1] != last {
			t.Fatalf("expected first %v, last %v, got first %v, last %v", first, last, queryVector[0], queryVector[len(queryVector)-1])
		}
	}

	g := concurrency
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}
