package kronk_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_ThinkChat(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelThinkToolChatFile, false)
}

func Test_ThinkStreamingChat(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, modelThinkToolChatFile, false)
}

func Test_ToolChat(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelThinkToolChatFile, true)
}

func Test_ToolStreamingChat(t *testing.T) {
	// Run on Linux only in GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, modelThinkToolChatFile, true)
}

func Test_GPTChat(t *testing.T) {
	// Don't run at all on GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelGPTChatFile, false)
}

func Test_GPTStreamingChat(t *testing.T) {
	// Don't run at all on GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, modelGPTChatFile, false)
}

// =============================================================================

func initChatTest(t *testing.T, modelFile string, tooling bool) (*kronk.Kronk, model.ChatRequest) {
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: modelFile,
	})

	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}

	var tools []model.Tool
	question := "Echo back the word: Gorilla"

	if tooling {
		question = "What is the weather like in London, England?"
		tools = []model.Tool{
			model.NewToolFunction(
				"get_weather",
				"Get the weather for a place",
				model.ToolParameter{
					Name:        "location",
					Type:        "string",
					Description: "The location to get the weather for, e.g. San Francisco, CA",
				},
			),
		}
	}

	cr := model.ChatRequest{
		Messages: []model.ChatMessage{
			{Role: "user", Content: question},
		},
		Tools: tools,
		Params: model.Params{
			MaxTokens: 4096,
		},
	}

	return krn, cr
}

func testChat(t *testing.T, modelFile string, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	krn, req := initChatTest(t, modelFile, tooling)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			name := strings.TrimSuffix(modelFile, path.Ext(modelFile))
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, req)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		if tooling {
			if err := testChatResponse(resp, modelFile, model.ObjectChat, "London", "get_weather", "location"); err != nil {
				return err
			}
			return nil
		}

		if err := testChatResponse(resp, modelFile, model.ObjectChat, "Gorilla", "", ""); err != nil {
			return err
		}

		return nil
	}

	var g errgroup.Group
	for range goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testChatStreaming(t *testing.T, modelFile string, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	krn, cr := initChatTest(t, modelFile, tooling)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*5*time.Second)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			name := strings.TrimSuffix(modelFile, path.Ext(modelFile))
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, cr)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, modelFile, model.ObjectChat, true); err != nil {
				return err
			}
		}

		if tooling {
			if err := testChatResponse(lastResp, modelFile, model.ObjectChat, "London", "get_weather", "location"); err != nil {
				return err
			}
			return nil
		}

		if err := testChatResponse(lastResp, modelFile, model.ObjectChat, "Gorilla", "", ""); err != nil {
			return err
		}

		return nil
	}

	var g errgroup.Group
	for range goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
