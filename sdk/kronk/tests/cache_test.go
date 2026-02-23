package kronk_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func Test_CacheSPC(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	cfg := model.Config{
		ModelFiles:        mpThinkToolChat.ModelFiles,
		ContextWindow:     8192,
		NBatch:            2048,
		NUBatch:           512,
		CacheTypeK:        model.GGMLTypeQ8_0,
		CacheTypeV:        model.GGMLTypeQ8_0,
		NSeqMax:           1,
		SystemPromptCache: true,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Request 1: system + user message
		d1 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Apple",
				},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("request 1: chat streaming: %v", err)
		}

		resp1, content1, err := drainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("request 1: %v", err)
		}

		if content1 == "" {
			t.Fatal("request 1: expected non-empty content")
		}

		prompt1 := 0
		if resp1.Usage != nil {
			prompt1 = resp1.Usage.PromptTokens
		}
		t.Logf("request 1: prompt_tokens=%d content=%q", prompt1, content1)

		// Request 2: same system prompt, different user message
		d2 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Banana",
				},
			},
			"max_tokens": 256,
		}

		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("request 2: chat streaming: %v", err)
		}

		resp2, content2, err := drainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("request 2: %v", err)
		}

		if content2 == "" {
			t.Fatal("request 2: expected non-empty content")
		}

		prompt2 := 0
		if resp2.Usage != nil {
			prompt2 = resp2.Usage.PromptTokens
		}
		t.Logf("request 2: prompt_tokens=%d content=%q", prompt2, content2)
	})
}

func Test_CacheIMCDeterministic(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	cfg := model.Config{
		ModelFiles:       mpThinkToolChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Turn 1: system + user (2 messages)
		d1 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Alpha",
				},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}

		resp1, content1, err := drainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}

		if content1 == "" {
			t.Fatal("turn 1: expected non-empty content")
		}

		prompt1 := 0
		if resp1.Usage != nil {
			prompt1 = resp1.Usage.PromptTokens
		}
		t.Logf("turn 1: prompt_tokens=%d content=%q", prompt1, content1)

		// Turn 2: system + user + assistant(turn 1) + user2 (4 messages)
		d2 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Alpha",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Beta",
				},
			},
			"max_tokens": 256,
		}

		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}

		resp2, content2, err := drainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}

		if content2 == "" {
			t.Fatal("turn 2: expected non-empty content")
		}

		prompt2 := 0
		if resp2.Usage != nil {
			prompt2 = resp2.Usage.PromptTokens
		}
		t.Logf("turn 2: prompt_tokens=%d content=%q", prompt2, content2)

		// Turn 3: system + user + assistant(turn 1) + user2 + assistant(turn 2) + user3 (6 messages)
		d3 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Alpha",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Beta",
				},
				{
					"role":    "assistant",
					"content": content2,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Gamma",
				},
			},
			"max_tokens": 256,
		}

		ch3, err := krn.ChatStreaming(ctx, d3)
		if err != nil {
			t.Fatalf("turn 3: chat streaming: %v", err)
		}

		resp3, content3, err := drainChat(ctx, ch3)
		if err != nil {
			t.Fatalf("turn 3: %v", err)
		}

		if content3 == "" {
			t.Fatal("turn 3: expected non-empty content")
		}

		prompt3 := 0
		if resp3.Usage != nil {
			prompt3 = resp3.Usage.PromptTokens
		}
		t.Logf("turn 3: prompt_tokens=%d content=%q", prompt3, content3)
	})
}

func Test_CacheIMCDeterministicMultiSlot(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	cfg := model.Config{
		ModelFiles:       mpThinkToolChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          2,
		IncrementalCache: true,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		dA := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": "You are a math tutor.",
				},
				{
					"role":    "user",
					"content": "What is 2+2?",
				},
			},
			"max_tokens": 256,
		}

		dB := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": "You are a poet.",
				},
				{
					"role":    "user",
					"content": "Write one line about the sea.",
				},
			},
			"max_tokens": 256,
		}

		var wg sync.WaitGroup
		wg.Add(2)

		startBarrier := make(chan struct{})
		contents := make([]string, 2)
		errors := make([]error, 2)

		go func() {
			defer wg.Done()
			<-startBarrier

			ch, err := krn.ChatStreaming(ctx, dA)
			if err != nil {
				errors[0] = fmt.Errorf("conversation A: chat streaming: %w", err)
				return
			}

			_, content, err := drainChat(ctx, ch)
			if err != nil {
				errors[0] = fmt.Errorf("conversation A: %w", err)
				return
			}
			contents[0] = content
		}()

		go func() {
			defer wg.Done()
			<-startBarrier

			ch, err := krn.ChatStreaming(ctx, dB)
			if err != nil {
				errors[1] = fmt.Errorf("conversation B: chat streaming: %w", err)
				return
			}

			_, content, err := drainChat(ctx, ch)
			if err != nil {
				errors[1] = fmt.Errorf("conversation B: %w", err)
				return
			}
			contents[1] = content
		}()

		close(startBarrier)
		wg.Wait()

		for i, err := range errors {
			if err != nil {
				t.Errorf("slot %d: %v", i, err)
			}
		}

		if t.Failed() {
			return
		}

		for i, content := range contents {
			if content == "" {
				t.Errorf("slot %d: expected non-empty content", i)
			}
			t.Logf("slot %d: content=%q", i, content)
		}
	})
}

func Test_CacheIMCNonDeterministic(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	cfg := model.Config{
		ModelFiles:       mpGPTChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
		CacheMinTokens:   100,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Turn 1: system + user
		d1 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Delta",
				},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}

		resp1, content1, err := drainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}

		if content1 == "" {
			t.Fatal("turn 1: expected non-empty content")
		}

		prompt1 := 0
		if resp1.Usage != nil {
			prompt1 = resp1.Usage.PromptTokens
		}
		t.Logf("turn 1: prompt_tokens=%d content=%q", prompt1, content1)

		// Turn 2: system + user + assistant(turn 1) + user2
		d2 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Delta",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Echo",
				},
			},
			"max_tokens": 256,
		}

		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}

		resp2, content2, err := drainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}

		if content2 == "" {
			t.Fatal("turn 2: expected non-empty content")
		}

		prompt2 := 0
		if resp2.Usage != nil {
			prompt2 = resp2.Usage.PromptTokens
		}
		t.Logf("turn 2: prompt_tokens=%d content=%q", prompt2, content2)
	})
}

func Test_CacheIMCMoE(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	if len(mpMoEChat.ModelFiles) == 0 {
		t.Skip("model Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL not downloaded")
	}

	cfg := model.Config{
		ModelFiles:       mpMoEChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Turn 1: system + user
		d1 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: North",
				},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}

		resp1, content1, err := drainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}

		if content1 == "" {
			t.Fatal("turn 1: expected non-empty content")
		}

		prompt1 := 0
		if resp1.Usage != nil {
			prompt1 = resp1.Usage.PromptTokens
		}
		t.Logf("turn 1: prompt_tokens=%d content=%q", prompt1, content1)

		// Turn 2: system + user + assistant(turn 1) + user2
		d2 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: North",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: South",
				},
			},
			"max_tokens": 256,
		}

		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}

		resp2, content2, err := drainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}

		if content2 == "" {
			t.Fatal("turn 2: expected non-empty content")
		}

		prompt2 := 0
		if resp2.Usage != nil {
			prompt2 = resp2.Usage.PromptTokens
		}
		t.Logf("turn 2: prompt_tokens=%d content=%q", prompt2, content2)

		// Turn 3: system + user + assistant(turn 1) + user2 + assistant(turn 2) + user3
		d3 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: North",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: South",
				},
				{
					"role":    "assistant",
					"content": content2,
				},
				{
					"role":    "user",
					"content": "Echo back the word: East",
				},
			},
			"max_tokens": 256,
		}

		ch3, err := krn.ChatStreaming(ctx, d3)
		if err != nil {
			t.Fatalf("turn 3: chat streaming: %v", err)
		}

		resp3, content3, err := drainChat(ctx, ch3)
		if err != nil {
			t.Fatalf("turn 3: %v", err)
		}

		if content3 == "" {
			t.Fatal("turn 3: expected non-empty content")
		}

		prompt3 := 0
		if resp3.Usage != nil {
			prompt3 = resp3.Usage.PromptTokens
		}
		t.Logf("turn 3: prompt_tokens=%d content=%q", prompt3, content3)
	})
}

func Test_CacheIMCHybrid(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping cache test in GitHub Actions (requires more resources)")
	}

	if len(mpHybridChat.ModelFiles) == 0 {
		t.Skip("model Qwen3-Coder-Next-UD-Q6_K_XL not downloaded")
	}

	cfg := model.Config{
		ModelFiles:       mpHybridChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	withModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Turn 1: system + user
		d1 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Red",
				},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}

		resp1, content1, err := drainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}

		if content1 == "" {
			t.Fatal("turn 1: expected non-empty content")
		}

		prompt1 := 0
		if resp1.Usage != nil {
			prompt1 = resp1.Usage.PromptTokens
		}
		t.Logf("turn 1: prompt_tokens=%d content=%q", prompt1, content1)

		// Turn 2: system + user + assistant(turn 1) + user2
		d2 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Red",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Blue",
				},
			},
			"max_tokens": 256,
		}

		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}

		resp2, content2, err := drainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}

		if content2 == "" {
			t.Fatal("turn 2: expected non-empty content")
		}

		prompt2 := 0
		if resp2.Usage != nil {
			prompt2 = resp2.Usage.PromptTokens
		}
		t.Logf("turn 2: prompt_tokens=%d content=%q", prompt2, content2)

		// Turn 3: system + user + assistant(turn 1) + user2 + assistant(turn 2) + user3
		d3 := model.D{
			"messages": []model.D{
				{
					"role":    "system",
					"content": systemPrompt,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Red",
				},
				{
					"role":    "assistant",
					"content": content1,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Blue",
				},
				{
					"role":    "assistant",
					"content": content2,
				},
				{
					"role":    "user",
					"content": "Echo back the word: Green",
				},
			},
			"max_tokens": 256,
		}

		ch3, err := krn.ChatStreaming(ctx, d3)
		if err != nil {
			t.Fatalf("turn 3: chat streaming: %v", err)
		}

		resp3, content3, err := drainChat(ctx, ch3)
		if err != nil {
			t.Fatalf("turn 3: %v", err)
		}

		if content3 == "" {
			t.Fatal("turn 3: expected non-empty content")
		}

		prompt3 := 0
		if resp3.Usage != nil {
			prompt3 = resp3.Usage.PromptTokens
		}
		t.Logf("turn 3: prompt_tokens=%d content=%q", prompt3, content3)
	})
}

// =============================================================================

func drainChat(ctx context.Context, ch <-chan model.ChatResponse) (model.ChatResponse, string, error) {
	var lastResp model.ChatResponse
	var reasoning strings.Builder
	var content strings.Builder

	for resp := range ch {
		lastResp = resp
		if len(resp.Choice) > 0 && resp.Choice[0].Delta != nil {
			reasoning.WriteString(resp.Choice[0].Delta.Reasoning)
			content.WriteString(resp.Choice[0].Delta.Content)
		}
	}

	if len(lastResp.Choice) > 0 && lastResp.Choice[0].FinishReason() == model.FinishReasonError {
		errMsg := ""
		if lastResp.Choice[0].Delta != nil {
			errMsg = lastResp.Choice[0].Delta.Content
		}
		return lastResp, content.String(), fmt.Errorf("model error: %s", errMsg)
	}

	// Use reasoning output as content if the model produced only thinking tokens.
	if content.Len() == 0 && reasoning.Len() > 0 {
		return lastResp, reasoning.String(), nil
	}

	return lastResp, content.String(), nil
}
