package kronk_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_ThinkChat(t *testing.T) {
	testChat(t, modelThinkToolChatFile, false)
}

func Test_ThinkStreamingChat(t *testing.T) {
	testChatStreaming(t, modelThinkToolChatFile, false)
}

func Test_ToolChat(t *testing.T) {
	testChat(t, modelThinkToolChatFile, true)
}

func Test_ToolStreamingChat(t *testing.T) {
	testChatStreaming(t, modelThinkToolChatFile, true)
}

func Test_GPTChat(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, modelGPTChatFile, false)
}

func Test_GPTStreamingChat(t *testing.T) {
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
		t.Fatalf("unable to load model: %s: %v", modelFile, err)
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

	krn, cr := initChatTest(t, modelFile, tooling)
	defer krn.Unload()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().Name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, cr)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		if tooling {
			if err := testChatResponse(resp, krn.ModelInfo().Name, model.ObjectChat, "London", "get_weather", "location"); err != nil {
				return err
			}
			return nil
		}

		if err := testChatResponse(resp, krn.ModelInfo().Name, model.ObjectChat, "Gorilla", "", ""); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().Name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, cr)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().Name, model.ObjectChat, true); err != nil {
				return err
			}
		}

		if tooling {
			if err := testChatResponse(lastResp, krn.ModelInfo().Name, model.ObjectChat, "London", "get_weather", "location"); err != nil {
				return err
			}
			return nil
		}

		if err := testChatResponse(lastResp, krn.ModelInfo().Name, model.ObjectChat, "Gorilla", "", ""); err != nil {
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
