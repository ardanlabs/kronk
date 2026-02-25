package kronk_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestUsageCounting validates that usage token counts are correct in streaming responses.
// This test checks:
// 1. Final usage matches the sum of tokens generated
// 2. Deltas have no usage (per OpenAI spec)
// 3. OutputTokens = ReasoningTokens + CompletionTokens
// 4. TotalTokens = PromptTokens + OutputTokens
func TestUsageCounting(t *testing.T) {
	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
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

// testStreamingUsage validates usage in streaming mode by counting delta tokens
// and comparing to final reported usage.
func testStreamingUsage(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
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

	// Validate final response has usage
	if finalResp.Usage == nil {
		t.Fatalf("final response has nil Usage")
	}

	if finalResp.Usage.PromptTokens == 0 {
		t.Errorf("final PromptTokens should be > 0, got %d", finalResp.Usage.PromptTokens)
	}

	// Validate OutputTokens = ReasoningTokens + CompletionTokens
	expectedOutput := finalResp.Usage.ReasoningTokens + finalResp.Usage.CompletionTokens
	if finalResp.Usage.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens mismatch: got %d, expected %d (reasoning=%d + completion=%d)",
			finalResp.Usage.OutputTokens, expectedOutput,
			finalResp.Usage.ReasoningTokens, finalResp.Usage.CompletionTokens)
	}

	// Validate TotalTokens = PromptTokens + OutputTokens
	expectedTotal := finalResp.Usage.PromptTokens + finalResp.Usage.OutputTokens
	if finalResp.Usage.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens mismatch: got %d, expected %d (prompt=%d + output=%d)",
			finalResp.Usage.TotalTokens, expectedTotal,
			finalResp.Usage.PromptTokens, finalResp.Usage.OutputTokens)
	}

	// Log for debugging
	t.Logf("Deltas received: %d (reasoning=%d, content=%d)", deltaCount, reasoningDeltas, contentDeltas)
	t.Logf("Final usage: prompt=%d, reasoning=%d, completion=%d, output=%d, total=%d",
		finalResp.Usage.PromptTokens, finalResp.Usage.ReasoningTokens,
		finalResp.Usage.CompletionTokens, finalResp.Usage.OutputTokens, finalResp.Usage.TotalTokens)

	// Check that delta count roughly matches output tokens
	outputTokens := finalResp.Usage.OutputTokens
	if outputTokens > 0 {
		ratio := float64(deltaCount) / float64(outputTokens)
		if ratio < 0.5 || ratio > 2.0 {
			t.Errorf("Delta count (%d) significantly differs from output tokens (%d), ratio=%.2f",
				deltaCount, outputTokens, ratio)
		}
	}
}

// testNonStreamingUsage validates usage in non-streaming (Chat) mode.
func testNonStreamingUsage(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
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

	// Validate usage fields
	if resp.Usage.PromptTokens == 0 {
		t.Errorf("PromptTokens should be > 0, got %d", resp.Usage.PromptTokens)
	}

	// OutputTokens = ReasoningTokens + CompletionTokens
	expectedOutput := resp.Usage.ReasoningTokens + resp.Usage.CompletionTokens
	if resp.Usage.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens mismatch: got %d, expected %d (reasoning=%d + completion=%d)",
			resp.Usage.OutputTokens, expectedOutput,
			resp.Usage.ReasoningTokens, resp.Usage.CompletionTokens)
	}

	// TotalTokens = PromptTokens + OutputTokens
	expectedTotal := resp.Usage.PromptTokens + resp.Usage.OutputTokens
	if resp.Usage.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens mismatch: got %d, expected %d (prompt=%d + output=%d)",
			resp.Usage.TotalTokens, expectedTotal,
			resp.Usage.PromptTokens, resp.Usage.OutputTokens)
	}

	// Should have some output
	if resp.Usage.OutputTokens == 0 {
		t.Errorf("OutputTokens should be > 0, got %d", resp.Usage.OutputTokens)
	}

	t.Logf("Non-streaming usage: prompt=%d, reasoning=%d, completion=%d, output=%d, total=%d",
		resp.Usage.PromptTokens, resp.Usage.ReasoningTokens,
		resp.Usage.CompletionTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
}

// testUsageOnlyInFinal validates that usage is only present in the final response.
// Per OpenAI spec, delta chunks should have usage: null (nil in Go).
func testUsageOnlyInFinal(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
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

		// Check if this is a delta (not final)
		if len(resp.Choice) > 0 && resp.Choice[0].FinishReason() == "" {
			// Delta should have nil usage per OpenAI spec
			if resp.Usage != nil {
				deltasWithUsage++
			}
		}
	}

	// Per OpenAI spec, deltas should have usage: null (nil)
	if deltasWithUsage > 0 {
		t.Errorf("Found %d deltas with non-nil usage (should be nil per OpenAI spec)", deltasWithUsage)
	}

	// Final response SHOULD have usage
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

// TestUsageAccumulation validates that accumulated usage in streaming matches
// the final reported values by counting content tokens manually.
func TestUsageAccumulation(t *testing.T) {
	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		d := model.D{
			"messages": []model.D{
				{
					"role":    "user",
					"content": "List three colors: red, blue, green.",
				},
			},
			"max_tokens": 256,
		}

		ch, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("chat streaming: %v", err)
		}

		var (
			contentTokens   int
			reasoningTokens int
			finalResp       model.ChatResponse
		)

		for resp := range ch {
			finalResp = resp

			if len(resp.Choice) == 0 {
				continue
			}

			choice := resp.Choice[0]
			if choice.FinishReason() != "" {
				continue
			}

			if choice.Delta != nil {
				if choice.Delta.Content != "" {
					contentTokens++
				}
				if choice.Delta.Reasoning != "" {
					reasoningTokens++
				}
			}
		}

		if finalResp.Usage == nil {
			t.Fatalf("final response has nil Usage")
		}

		t.Logf("Counted: reasoning=%d, content=%d", reasoningTokens, contentTokens)
		t.Logf("Reported: reasoning=%d, completion=%d, output=%d",
			finalResp.Usage.ReasoningTokens, finalResp.Usage.CompletionTokens, finalResp.Usage.OutputTokens)

		if finalResp.Usage.ReasoningTokens > 0 && reasoningTokens == 0 {
			t.Errorf("Model reported %d reasoning tokens but we counted 0 reasoning deltas",
				finalResp.Usage.ReasoningTokens)
		}

		if finalResp.Usage.CompletionTokens > 0 && contentTokens == 0 {
			t.Errorf("Model reported %d completion tokens but we counted 0 content deltas",
				finalResp.Usage.CompletionTokens)
		}
	})
}

// TestUsageWithToolCalls validates usage counting when tool calls are made.
func TestUsageWithToolCalls(t *testing.T) {
	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		ch, err := krn.ChatStreaming(ctx, dChatTool)
		if err != nil {
			t.Fatalf("chat streaming: %v", err)
		}

		var finalResp model.ChatResponse
		for resp := range ch {
			finalResp = resp
		}

		if len(finalResp.Choice) == 0 {
			t.Fatalf("expected at least one choice")
		}

		if finalResp.Choice[0].FinishReason() != "tool_calls" {
			t.Logf("Warning: expected finish_reason=tool_calls, got %s", finalResp.Choice[0].FinishReason())
		}

		if finalResp.Usage == nil {
			t.Fatalf("final response has nil Usage")
		}

		if finalResp.Usage.PromptTokens == 0 {
			t.Errorf("PromptTokens should be > 0 for tool call request")
		}

		if finalResp.Usage.OutputTokens == 0 {
			t.Errorf("OutputTokens should be > 0 for tool call response")
		}

		expectedOutput := finalResp.Usage.ReasoningTokens + finalResp.Usage.CompletionTokens
		if finalResp.Usage.OutputTokens != expectedOutput {
			t.Errorf("OutputTokens mismatch in tool call: got %d, expected %d",
				finalResp.Usage.OutputTokens, expectedOutput)
		}

		t.Logf("Tool call usage: prompt=%d, reasoning=%d, completion=%d, output=%d, total=%d",
			finalResp.Usage.PromptTokens, finalResp.Usage.ReasoningTokens,
			finalResp.Usage.CompletionTokens, finalResp.Usage.OutputTokens, finalResp.Usage.TotalTokens)
	})
}

// TestUsageDeltaNil validates that delta chunks have nil usage per OpenAI spec.
func TestUsageDeltaNil(t *testing.T) {
	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		d := model.D{
			"messages": []model.D{
				{
					"role":    "user",
					"content": "Count: 1 2 3 4 5",
				},
			},
			"max_tokens": 128,
		}

		ch, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("chat streaming: %v", err)
		}

		var (
			deltaCount      int
			deltasWithUsage int
			finalResp       model.ChatResponse
		)

		for resp := range ch {
			finalResp = resp

			if len(resp.Choice) == 0 {
				continue
			}

			choice := resp.Choice[0]
			if choice.FinishReason() == "" {
				deltaCount++
				// Per OpenAI spec, deltas should have usage: null (nil in Go)
				if resp.Usage != nil {
					deltasWithUsage++
				}
			}
		}

		t.Logf("Total deltas: %d", deltaCount)
		t.Logf("Deltas with non-nil usage: %d", deltasWithUsage)

		if finalResp.Usage != nil {
			t.Logf("Final usage: output=%d (reasoning=%d, completion=%d)",
				finalResp.Usage.OutputTokens, finalResp.Usage.ReasoningTokens, finalResp.Usage.CompletionTokens)
		}

		// All deltas should have nil usage
		if deltasWithUsage > 0 {
			t.Errorf("Expected all deltas to have nil usage, but %d had non-nil values", deltasWithUsage)
		}

		// Final should have usage
		switch {
		case finalResp.Usage == nil:
			t.Errorf("Expected final response to have Usage")

		case finalResp.Usage.OutputTokens == 0:
			t.Errorf("Expected final response to have OutputTokens > 0")
		}
	})
}

// TestUsageCountingMoE validates usage token counts for MoE models.
func TestUsageCountingMoE(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping MoE usage test in GitHub Actions (requires more resources)")
	}

	if len(mpMoEChat.ModelFiles) == 0 {
		t.Skip("model Qwen3.5-35B-A3B-UD-Q8_K_XL not downloaded")
	}

	withModel(t, cfgMoEChat(), func(t *testing.T, krn *kronk.Kronk) {
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

// TestUsageCountingHybrid validates usage token counts for Hybrid models.
func TestUsageCountingHybrid(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping Hybrid usage test in GitHub Actions (requires more resources)")
	}

	if len(mpHybridChat.ModelFiles) == 0 {
		t.Skip("model Qwen3-Coder-Next-UD-Q6_K_XL not downloaded")
	}

	withModel(t, cfgHybridChat(), func(t *testing.T, krn *kronk.Kronk) {
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

// TestUsageConsistencyAcrossRequests validates that usage reporting is consistent
// across multiple requests to the same model.
func TestUsageConsistencyAcrossRequests(t *testing.T) {
	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
		prompt := model.D{
			"messages": []model.D{
				{
					"role":    "user",
					"content": "What is 2+2?",
				},
			},
			"max_tokens": 64,
		}

		var promptTokens []int
		for i := range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), testDuration)

			resp, err := krn.Chat(ctx, prompt)
			cancel()

			if err != nil {
				t.Fatalf("request %d: chat failed: %v", i, err)
			}

			if resp.Usage == nil {
				t.Fatalf("request %d: response has nil Usage", i)
			}

			promptTokens = append(promptTokens, resp.Usage.PromptTokens)

			time.Sleep(100 * time.Millisecond)
		}

		// PromptTokens should be identical for same prompt
		for i := 1; i < len(promptTokens); i++ {
			if promptTokens[i] != promptTokens[0] {
				t.Errorf("PromptTokens inconsistent: request 0=%d, request %d=%d",
					promptTokens[0], i, promptTokens[i])
			}
		}

		t.Logf("Prompt tokens across %d requests: consistent at %d", len(promptTokens), promptTokens[0])
	})
}
