package moe_test

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestUsageCounting(t *testing.T) {
	testlib.WithModel(t, testlib.CfgMoEVision(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("StreamingUsage", func(t *testing.T) {
			testStreamingUsage(t, krn)
		})
		t.Run("NonStreamingUsage", func(t *testing.T) {
			testNonStreamingUsage(t, krn)
		})
		t.Run("UsageOnlyInFinal", func(t *testing.T) {
			testUsageOnlyInFinal(t, krn)
		})
	})
}

func testStreamingUsage(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Count from 1 to 5, one number per line.",
			},
		},
		"max_tokens": 256,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("chat streaming: %v", err)
	}

	var (
		deltaCount      int
		reasoningDeltas int
		contentDeltas   int
		finalResp       model.ChatResponse
	)

	for resp := range ch {
		finalResp = resp

		if len(resp.Choice) == 0 {
			continue
		}

		choice := resp.Choice[0]
		if choice.Delta != nil {
			if choice.Delta.Reasoning != "" {
				reasoningDeltas++
				deltaCount++
			}
			if choice.Delta.Content != "" {
				contentDeltas++
				deltaCount++
			}
		}
	}

	if finalResp.Usage == nil {
		t.Fatalf("final response has nil Usage")
	}

	if finalResp.Usage.PromptTokens == 0 {
		t.Errorf("final PromptTokens should be > 0, got %d", finalResp.Usage.PromptTokens)
	}

	expectedOutput := finalResp.Usage.ReasoningTokens + finalResp.Usage.CompletionTokens
	if finalResp.Usage.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens mismatch: got %d, expected %d (reasoning=%d + completion=%d)",
			finalResp.Usage.OutputTokens, expectedOutput,
			finalResp.Usage.ReasoningTokens, finalResp.Usage.CompletionTokens)
	}

	expectedTotal := finalResp.Usage.PromptTokens + finalResp.Usage.OutputTokens
	if finalResp.Usage.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens mismatch: got %d, expected %d (prompt=%d + output=%d)",
			finalResp.Usage.TotalTokens, expectedTotal,
			finalResp.Usage.PromptTokens, finalResp.Usage.OutputTokens)
	}

	t.Logf("Deltas received: %d (reasoning=%d, content=%d)", deltaCount, reasoningDeltas, contentDeltas)
	t.Logf("Final usage: prompt=%d, reasoning=%d, completion=%d, output=%d, total=%d",
		finalResp.Usage.PromptTokens, finalResp.Usage.ReasoningTokens,
		finalResp.Usage.CompletionTokens, finalResp.Usage.OutputTokens, finalResp.Usage.TotalTokens)

	outputTokens := finalResp.Usage.OutputTokens
	if outputTokens > 0 {
		ratio := float64(deltaCount) / float64(outputTokens)
		if ratio < 0.5 || ratio > 2.0 {
			t.Errorf("Delta count (%d) significantly differs from output tokens (%d), ratio=%.2f",
				deltaCount, outputTokens, ratio)
		}
	}
}

func testNonStreamingUsage(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Say hello.",
			},
		},
		"max_tokens": 128,
	}

	resp, err := krn.Chat(ctx, d)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if resp.Usage == nil {
		t.Fatalf("response has nil Usage")
	}

	if resp.Usage.PromptTokens == 0 {
		t.Errorf("PromptTokens should be > 0, got %d", resp.Usage.PromptTokens)
	}

	expectedOutput := resp.Usage.ReasoningTokens + resp.Usage.CompletionTokens
	if resp.Usage.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens mismatch: got %d, expected %d (reasoning=%d + completion=%d)",
			resp.Usage.OutputTokens, expectedOutput,
			resp.Usage.ReasoningTokens, resp.Usage.CompletionTokens)
	}

	expectedTotal := resp.Usage.PromptTokens + resp.Usage.OutputTokens
	if resp.Usage.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens mismatch: got %d, expected %d (prompt=%d + output=%d)",
			resp.Usage.TotalTokens, expectedTotal,
			resp.Usage.PromptTokens, resp.Usage.OutputTokens)
	}

	if resp.Usage.OutputTokens == 0 {
		t.Errorf("OutputTokens should be > 0, got %d", resp.Usage.OutputTokens)
	}

	t.Logf("Non-streaming usage: prompt=%d, reasoning=%d, completion=%d, output=%d, total=%d",
		resp.Usage.PromptTokens, resp.Usage.ReasoningTokens,
		resp.Usage.CompletionTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
}

func testUsageOnlyInFinal(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Write a short poem about the sea.",
			},
		},
		"max_tokens": 512,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("chat streaming: %v", err)
	}

	var (
		deltaNum        int
		deltasWithUsage int
		finalResp       model.ChatResponse
	)

	for resp := range ch {
		deltaNum++
		finalResp = resp

		if len(resp.Choice) > 0 && resp.Choice[0].FinishReason() == "" {
			if resp.Usage != nil {
				deltasWithUsage++
			}
		}
	}

	if deltasWithUsage > 0 {
		t.Errorf("Found %d deltas with non-nil usage (should be nil per OpenAI spec)", deltasWithUsage)
	}

	if finalResp.Usage == nil {
		t.Fatalf("Final response missing Usage")
	}
	if finalResp.Usage.PromptTokens == 0 {
		t.Errorf("Final response missing PromptTokens")
	}
	if finalResp.Usage.OutputTokens == 0 {
		t.Errorf("Final response missing OutputTokens")
	}
	if finalResp.Usage.TotalTokens == 0 {
		t.Errorf("Final response missing TotalTokens")
	}

	t.Logf("Total deltas: %d, deltas with non-nil usage: %d", deltaNum, deltasWithUsage)
	t.Logf("Final usage: prompt=%d, output=%d, total=%d",
		finalResp.Usage.PromptTokens, finalResp.Usage.OutputTokens, finalResp.Usage.TotalTokens)
}
