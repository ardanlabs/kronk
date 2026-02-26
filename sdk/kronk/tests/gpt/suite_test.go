package gpt_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgGPTChat(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("GPTChat", func(t *testing.T) { testChat(t, krn, testlib.DChatNoTool, false) })
		t.Run("GPTStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatNoTool, false) })
		t.Run("ToolGPTChat", func(t *testing.T) { testChat(t, krn, testlib.DChatToolGPT, true) })
		t.Run("ToolGPTStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatToolGPT, true) })
	})
}

func testChat(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
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

		reasoning := testlib.HasReasoningField(krn)

		var result testlib.TestResult
		switch tooling {
		case true:
			result = testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, "London", "get_weather", "location", false, reasoning)
		case false:
			result = testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, "Gorilla", "", "", false, reasoning)
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
	for range testlib.Goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testChatStreaming(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
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

		reasoning := testlib.HasReasoningField(krn)

		var acc testlib.StreamAccumulator
		var lastResp model.ChatResponse
		for resp := range ch {
			acc.Accumulate(resp)
			lastResp = resp

			if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatText, reasoning, true); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		var result testlib.TestResult
		switch tooling {
		case true:
			result = testlib.TestStreamingToolCall(&acc, lastResp, "London", "get_weather", "location")
		case false:
			result = testlib.TestStreamingContent(&acc, lastResp, "Gorilla")
		}

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("accumulated content: %q", acc.Content.String())
			t.Logf("%#v", lastResp)
			return result.Err
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
