package kronk_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func testMedia(t *testing.T, krn *kronk.Kronk) {
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

		resp, err := krn.Chat(ctx, dMedia)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		result := testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", "", false)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", resp)
			return result.Err
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

func testMediaStreaming(t *testing.T, krn *kronk.Kronk) {
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

		ch, err := krn.ChatStreaming(ctx, dMedia)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatMedia, false, true); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		result := testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", "", true)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", lastResp)
			return result.Err
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

func testMediaResponse(t *testing.T, krn *kronk.Kronk) {
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

		resp, err := krn.Response(ctx, dMedia)
		if err != nil {
			return fmt.Errorf("response: %w", err)
		}

		if err := testMediaResponseResponse(resp, krn.ModelInfo().ID, "giraffes"); err != nil {
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

func testMediaResponseStreaming(t *testing.T, krn *kronk.Kronk) {
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

		ch, err := krn.ResponseStreaming(ctx, dMedia)
		if err != nil {
			return fmt.Errorf("response streaming: %w", err)
		}

		var finalResp *kronk.ResponseResponse
		var hasTextDelta bool

		for event := range ch {
			switch event.Type {
			case "response.created":
				if event.Response == nil {
					return fmt.Errorf("response.created: expected response")
				}
				if event.Response.Status != "in_progress" {
					return fmt.Errorf("response.created: expected status in_progress, got %s", event.Response.Status)
				}

			case "response.output_text.delta":
				if event.Delta == "" {
					return fmt.Errorf("response.output_text.delta: expected delta")
				}
				hasTextDelta = true

			case "response.completed":
				if event.Response == nil {
					return fmt.Errorf("response.completed: expected response")
				}
				if event.Response.Status != "completed" {
					return fmt.Errorf("response.completed: expected status completed, got %s", event.Response.Status)
				}
				finalResp = event.Response
			}
		}

		if finalResp == nil {
			return fmt.Errorf("expected response.completed event")
		}

		if !hasTextDelta {
			return fmt.Errorf("expected output_text.delta events")
		}

		if err := testMediaResponseResponse(*finalResp, krn.ModelInfo().ID, "giraffes"); err != nil {
			t.Logf("%#v", finalResp)
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

func testMediaResponseResponse(resp kronk.ResponseResponse, modelName string, find string) error {
	if resp.ID == "" {
		return fmt.Errorf("expected id")
	}

	if resp.Object != "response" {
		return fmt.Errorf("expected object type to be response, got %s", resp.Object)
	}

	if resp.CreatedAt == 0 {
		return fmt.Errorf("expected created time")
	}

	if resp.Model != modelName {
		return fmt.Errorf("expected model to be %s, got %s", modelName, resp.Model)
	}

	if resp.Status != "completed" {
		return fmt.Errorf("expected status to be completed, got %s", resp.Status)
	}

	if len(resp.Output) == 0 {
		return fmt.Errorf("expected output, got %d", len(resp.Output))
	}

	find = strings.ToLower(find)

	for _, output := range resp.Output {
		if output.Type == "message" {
			for _, content := range output.Content {
				if content.Type == "output_text" {
					text := strings.ToLower(content.Text)
					if strings.Contains(text, find) {
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("expected to find %q in output", find)
}

// =============================================================================
// Audio Tests

func testAudio(t *testing.T, krn *kronk.Kronk) {
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

		resp, err := krn.Chat(ctx, dAudio)
		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}

		result := testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "speech", "", "", false)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", resp)
			return result.Err
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

func testAudioStreaming(t *testing.T, krn *kronk.Kronk) {
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

		ch, err := krn.ChatStreaming(ctx, dAudio)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatMedia, false, true); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		result := testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatMedia, "speech", "", "", true)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", lastResp)
			return result.Err
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
