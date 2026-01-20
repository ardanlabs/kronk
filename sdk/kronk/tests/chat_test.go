package kronk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

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

		var result testResult
		if tooling {
			result = testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatText, "London", "get_weather", "location", false)
		} else {
			result = testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatText, "Gorilla", "", "", false)
		}

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

		var result testResult
		if tooling {
			result = testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatText, "London", "get_weather", "location", true)
		} else {
			result = testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatText, "Gorilla", "", "", true)
		}

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
