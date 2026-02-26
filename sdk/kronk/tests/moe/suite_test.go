package moe_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgMoEChat(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("ThinkChat", func(t *testing.T) { testChat(t, krn, testlib.DChatNoTool, false) })
		t.Run("ThinkStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatNoTool, false) })
		t.Run("ToolChat", func(t *testing.T) { testChat(t, krn, testlib.DChatTool, true) })
		t.Run("ToolStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatTool, true) })
		t.Run("ThinkResponse", func(t *testing.T) { testResponse(t, krn, testlib.DResponseNoTool, false) })
		t.Run("ThinkStreamingResponse", func(t *testing.T) { testResponseStreaming(t, krn, testlib.DResponseNoTool, false) })
		t.Run("ToolResponse", func(t *testing.T) { testResponse(t, krn, testlib.DResponseTool, true) })
		t.Run("ToolStreamingResponse", func(t *testing.T) { testResponseStreaming(t, krn, testlib.DResponseTool, true) })
		t.Run("GrammarJSON", func(t *testing.T) { testGrammarJSON(t, krn) })
		t.Run("GrammarJSONStreaming", func(t *testing.T) { testGrammarJSONStreaming(t, krn) })
	})
}

// =============================================================================

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

func testResponse(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
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

		resp, err := krn.Response(ctx, d)
		if err != nil {
			return fmt.Errorf("response: %w", err)
		}

		if tooling {
			if err := testlib.TestResponseResponse(resp, krn.ModelInfo().ID, "London", "get_weather", "location"); err != nil {
				t.Logf("%#v", resp)
				return err
			}
			return nil
		}

		if err := testlib.TestResponseResponse(resp, krn.ModelInfo().ID, "Gorilla", "", ""); err != nil {
			t.Logf("%#v", resp)
			return err
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

func testResponseStreaming(t *testing.T, krn *kronk.Kronk, d model.D, tooling bool) {
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

		ch, err := krn.ResponseStreaming(ctx, d)
		if err != nil {
			return fmt.Errorf("response streaming: %w", err)
		}

		var finalResp *kronk.ResponseResponse
		var hasTextDelta bool
		var hasReasoningDelta bool
		var hasFunctionCallDone bool

		for event := range ch {
			switch event.Type {
			case "response.created":
				if event.Response == nil {
					return fmt.Errorf("response.created: expected response")
				}
				if event.Response.Status != "in_progress" {
					return fmt.Errorf("response.created: expected status in_progress, got %s", event.Response.Status)
				}

			case "response.reasoning_summary_text.delta":
				if event.Delta == "" {
					return fmt.Errorf("response.reasoning_summary_text.delta: expected delta")
				}
				hasReasoningDelta = true

			case "response.output_text.delta":
				if event.Delta == "" {
					return fmt.Errorf("response.output_text.delta: expected delta")
				}
				hasTextDelta = true

			case "response.function_call_arguments.done":
				if event.Name == "" {
					return fmt.Errorf("response.function_call_arguments.done: expected name")
				}
				if event.Arguments == "" {
					return fmt.Errorf("response.function_call_arguments.done: expected arguments")
				}
				hasFunctionCallDone = true

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

		if tooling {
			if !hasFunctionCallDone {
				return fmt.Errorf("expected function_call_arguments.done event for tooling")
			}
			if err := testlib.TestResponseResponse(*finalResp, krn.ModelInfo().ID, "London", "get_weather", "location"); err != nil {
				t.Logf("%#v", finalResp)
				return err
			}
			return nil
		}

		if !hasTextDelta {
			return fmt.Errorf("expected output_text.delta events")
		}

		if testlib.HasReasoningField(krn) && !hasReasoningDelta {
			return fmt.Errorf("expected reasoning_summary_text.delta events")
		}

		if err := testlib.TestResponseResponse(*finalResp, krn.ModelInfo().ID, "Gorilla", "", ""); err != nil {
			t.Logf("%#v", finalResp)
			return err
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

func testGrammarJSON(t *testing.T, krn *kronk.Kronk) {
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

		resp, err := krn.Chat(ctx, testlib.DGrammarJSON)
		if err != nil {
			return fmt.Errorf("grammar chat: %w", err)
		}

		result := testlib.TestGrammarJSONResponse(resp, krn.ModelInfo().ID)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", resp)
			return result.Err
		}

		return nil
	}

	if err := f(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testGrammarJSONStreaming(t *testing.T, krn *kronk.Kronk) {
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

		ch, err := krn.ChatStreaming(ctx, testlib.DGrammarJSON)
		if err != nil {
			return fmt.Errorf("grammar streaming: %w", err)
		}

		var content strings.Builder
		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if resp.Choice[0].FinishReason() == model.FinishReasonStop {
				break
			}

			if len(resp.Choice) > 0 && resp.Choice[0].Delta != nil {
				content.WriteString(resp.Choice[0].Delta.Content)
			}
		}

		result := testlib.TestGrammarStreamingContent(content.String(), krn.ModelInfo().ID)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("accumulated content: %q", content.String())
			t.Logf("%#v", lastResp)
			return result.Err
		}

		return nil
	}

	if err := f(); err != nil {
		t.Errorf("error: %v", err)
	}
}
