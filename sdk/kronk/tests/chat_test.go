package kronk_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_ThinkChat(t *testing.T) {
	testChat(t, krnThinkToolChat, dChatNoTool, false)
}

func Test_ThinkStreamingChat(t *testing.T) {
	testChatStreaming(t, krnThinkToolChat, dChatNoTool, false)
}

func Test_ToolChat(t *testing.T) {
	testChat(t, krnThinkToolChat, dChatTool, true)
}

func Test_ToolStreamingChat(t *testing.T) {
	testChatStreaming(t, krnThinkToolChat, dChatTool, true)
}

func Test_GPTChat(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, krnGPTChat, dChatNoTool, false)
}

func Test_GPTStreamingChat(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, krnGPTChat, dChatNoTool, false)
}

func Test_ToolGPTChat(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChat(t, krnGPTChat, dChatToolGPT, true)
}

func Test_ToolGPTStreamingChat(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions")
	}

	testChatStreaming(t, krnGPTChat, dChatToolGPT, true)
}

// =============================================================================

func testChat(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, d)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		if tooling {
			if err := testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatText, "London", "get_weather", "location", false); err != nil {
				t.Logf("%#v", resp)
				return err
			}
			return nil
		}

		if err := testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatText, "Gorilla", "", "", false); err != nil {
			t.Logf("%#v", resp)
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

func testChatStreaming(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatText, true, true); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		if tooling {
			if err := testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatText, "London", "get_weather", "location", true); err != nil {
				t.Logf("%#v", lastResp)
				return err
			}
			return nil
		}

		if err := testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatText, "Gorilla", "", "", true); err != nil {
			t.Logf("%#v", lastResp)
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
