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
	modelSimpleChatFile   = "models/qwen2.5-0.5b-instruct-q8_0.gguf"
	modelThinkChatFile    = "models/Qwen3-8B-Q8_0.gguf"
	modelGPTChatFile      = "models/gpt-oss-20b-Q8_0.gguf"
	modelSimpleVisionFile = "models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projSimpleVisionFile  = "models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	modelEmbedFile        = "models/embeddinggemma-300m-qat-Q8_0.gguf"
)

var (
	gw          = os.Getenv("GITHUB_WORKSPACE")
	libPath     = filepath.Join(gw, "libraries")
	modelPath   = filepath.Join(gw, "models")
	imageFile   = filepath.Join(gw, "images/samples", "giraffe.jpg")
	concurrency = 1
)

func TestMain(m *testing.M) {
	install()

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		fmt.Printf("Failed to init the llamacpp library: %s: error: %s\n", libPath, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// =============================================================================

func TestSimpleChat(t *testing.T) {
	// Run on all platforms.
	testChat(t, modelSimpleChatFile, false)
}

func TestThinkChat(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelThinkChatFile, true)
}

func TestGPTChat(t *testing.T) {
	// Don't run at all on GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelGPTChatFile, true)
}

// =============================================================================

func TestSimpleChatStreaming(t *testing.T) {
	// Run on all platforms.
	testChatStreaming(t, modelSimpleChatFile, false)
}

func TestThinkChatStreaming(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, modelThinkChatFile, true)
}

func TestGPTChatStreaming(t *testing.T) {
	// Don't run at all on GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, modelGPTChatFile, true)
}

// =============================================================================

func TestSimpleVision(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testVision(t, modelSimpleVisionFile, projSimpleVisionFile)
}

func TestSimpleVisionStreaming(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testVisionStreaming(t, modelSimpleVisionFile, projSimpleVisionFile)
}

// =============================================================================

func TestEmbedding(t *testing.T) {
	// Run on all platforms.
	testEmbedding(t, modelEmbedFile)
}

// =============================================================================

func install() {
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
}

func testResponse(msg kronk.ChatResponse, modelFile string, object string, reasoning bool) error {
	if msg.ID == "" {
		return fmt.Errorf("expected id")
	}

	if msg.Object != object {
		return fmt.Errorf("expected object type to be %s, got %s", object, msg.Object)
	}

	if msg.Created == 0 {
		return fmt.Errorf("expected created time")
	}

	modelName := filepath.Base(modelFile)
	modelName = strings.TrimSuffix(modelName, filepath.Ext(modelName))

	if msg.Model != modelName {
		return fmt.Errorf("expected model to be %s, got %s", modelName, msg.Model)
	}

	if len(msg.Choice) == 0 {
		return fmt.Errorf("expected choice, got %d", len(msg.Choice))
	}

	if msg.Choice[0].FinishReason == "" && msg.Choice[0].Delta.Content == "" && msg.Choice[0].Delta.Reasoning == "" {
		return fmt.Errorf("expected delta content and reasoning empty")
	}

	if msg.Choice[0].FinishReason == "" && msg.Choice[0].Delta.Role != "assistant" {
		return fmt.Errorf("expected delta role to be assistant, got %s", msg.Choice[0].Delta.Role)
	}

	if msg.Choice[0].FinishReason == "stop" && msg.Choice[0].Delta.Content == "" {
		return fmt.Errorf("expected final content")
	}

	if reasoning {
		if msg.Choice[0].FinishReason == "stop" && msg.Choice[0].Delta.Reasoning == "" {
			return fmt.Errorf("expected final reasoning")
		}
	}

	return nil
}

// =============================================================================

func initChatTest(t *testing.T, modelFile string) (*kronk.Kronk, []kronk.ChatMessage, kronk.Params) {
	krn, err := kronk.New(concurrency, modelFile, "", kronk.ModelConfig{})
	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}

	question := "Echo back the word: Gorilla"

	messages := []kronk.ChatMessage{
		{
			Role:    "user",
			Content: question,
		},
	}

	return krn, messages, kronk.Params{}
}

func testChat(t *testing.T, modelFile string, reasoning bool) {
	krn, messages, params := initChatTest(t, modelFile)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		resp, err := krn.Chat(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		if err := testResponse(resp, modelFile, "chat", reasoning); err != nil {
			return err
		}

		find := "Gorilla"
		if !strings.Contains(resp.Choice[0].Delta.Content, find) {
			return fmt.Errorf("expected %q, got %q", find, resp.Choice[0].Delta.Content)
		}

		if reasoning {
			if !strings.Contains(resp.Choice[0].Delta.Reasoning, find) {
				return fmt.Errorf("expected %q, got %q", find, resp.Choice[0].Delta.Content)
			}
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

func testChatStreaming(t *testing.T, modelFile string, reasoning bool) {
	krn, messages, params := initChatTest(t, modelFile)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		ch, err := krn.ChatStreaming(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp kronk.ChatResponse
		for resp := range ch {
			if err := testResponse(resp, modelFile, "chat", reasoning); err != nil {
				return err
			}

			lastResp = resp
		}

		if err := testResponse(lastResp, modelFile, "chat", reasoning); err != nil {
			return err
		}

		find := "Gorilla"
		if !strings.Contains(lastResp.Choice[0].Delta.Content, find) {
			return fmt.Errorf("expected %q, got %q", find, lastResp.Choice[0].Delta.Content)
		}

		if reasoning {
			if !strings.Contains(lastResp.Choice[0].Delta.Reasoning, find) {
				return fmt.Errorf("expected %q, got %q", find, lastResp.Choice[0].Delta.Content)
			}
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

func initVisionTest(t *testing.T, modelFile, projFile string) (*kronk.Kronk, kronk.ChatMessage, kronk.Params) {
	krn, err := kronk.New(concurrency, modelFile, projFile, kronk.ModelConfig{})
	if err != nil {
		t.Fatalf("unable to create inference model: %v", err)
	}

	question := "What is in this picture?"

	message := kronk.ChatMessage{
		Role:    "user",
		Content: question,
	}

	return krn, message, kronk.Params{}
}

func testVision(t *testing.T, modelFile string, profFile string) {
	krn, message, params := initVisionTest(t, modelFile, profFile)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		resp, err := krn.Vision(ctx, message, imageFile, params)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
		}

		if err := testResponse(resp, modelFile, "vision", false); err != nil {
			return err
		}

		find := "giraffes"
		if !strings.Contains(resp.Choice[0].Delta.Content, find) {
			return fmt.Errorf("expected %q, got %q", find, resp.Choice[0].Delta.Content)
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

func testVisionStreaming(t *testing.T, modelFile string, profFile string) {
	krn, message, params := initVisionTest(t, modelFile, profFile)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		ch, err := krn.VisionStreaming(ctx, message, imageFile, params)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
		}

		var lastResp kronk.ChatResponse
		for resp := range ch {
			if err := testResponse(resp, modelFile, "vision", false); err != nil {
				return err
			}

			lastResp = resp
		}

		if err := testResponse(lastResp, modelFile, "vision", false); err != nil {
			return err
		}

		find := "giraffes"
		if !strings.Contains(lastResp.Choice[0].Delta.Content, find) {
			return fmt.Errorf("expected %q, got %q", find, lastResp.Choice[0].Delta.Content)
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

func testEmbedding(t *testing.T, modelFile string) {
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
