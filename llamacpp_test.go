package llamacpp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/llamacpp"
)

var (
	modelChatCompletionsURL = "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-fp16.gguf?download=true"
	modelChatVisionURL      = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf?download=true"
	projChatVisionURL       = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf?download=true"
	modelEmbedURL           = "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf?download=true"
)

var (
	libPath   = "libraries/"
	modelPath = "models/"
	imageFile = "samples/giraffe.jpg"
)

func TestMain(m *testing.M) {
	fmt.Println("libpath:", libPath)
	fmt.Println("modelPath:", modelPath)
	fmt.Println("imageFile:", imageFile)

	if err := filepath.Walk(modelPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Println("Model:", path)
		return nil
	}); err != nil {
		fmt.Printf("error walking model path: %v\n", err)
	}

	// if err := llamacpp.InstallLlama(libPath, download.CPU, true); err != nil {
	// 	fmt.Printf("unable to install llamacpp: %v", err)
	// 	os.Exit(1)
	// }

	os.Exit(m.Run())
}

func TestChatCompletions(t *testing.T) {
	modelFile, err := llamacpp.InstallModel(modelChatCompletionsURL, modelPath)
	if err != nil {
		t.Fatalf("unable to install model: %v", err)
	}

	// -------------------------------------------------------------------------

	const concurrency = 3

	llm, err := llamacpp.New(concurrency, libPath, modelFile, llamacpp.Config{
		ContextWindow: 8196,
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	g := concurrency * 5
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}

func TestChatVision(t *testing.T) {
	modelFile, err := llamacpp.InstallModel(modelChatVisionURL, modelPath)
	if err != nil {
		t.Fatalf("unable to install model: %v", err)
	}

	projFile, err := llamacpp.InstallModel(projChatVisionURL, modelPath)
	if err != nil {
		t.Fatalf("unable to install model: %v", err)
	}

	// -------------------------------------------------------------------------

	const concurrency = 3

	cfg := llamacpp.Config{
		ContextWindow: 4096,
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	g := concurrency * 2
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}

func TestEmbedding(t *testing.T) {
	modelFile, err := llamacpp.InstallModel(modelEmbedURL, modelPath)
	if err != nil {
		t.Fatalf("unable to install embedding model: %v", err)
	}

	// -------------------------------------------------------------------------

	const concurrency = 3

	cfg := llamacpp.Config{
		ContextWindow: 4096,
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	g := concurrency * 2
	var wg sync.WaitGroup
	for range g {
		wg.Go(f)
	}

	wg.Wait()
}
