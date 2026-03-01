package hybrid_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_CacheIMCHybrid(t *testing.T) {
	cfg := model.Config{
		ModelFiles:       testlib.MPHybridVision.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
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

		resp1, content1, err := testlib.DrainChat(ctx, ch1)
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

		resp2, content2, err := testlib.DrainChat(ctx, ch2)
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

		resp3, content3, err := testlib.DrainChat(ctx, ch3)
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

func Test_CacheIMCHybridMultiSlot(t *testing.T) {
	cfg := model.Config{
		ModelFiles:       testlib.MPHybridVision.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		NSeqMax:          2,
		IncrementalCache: true,
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
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

			_, content, err := testlib.DrainChat(ctx, ch)
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

			_, content, err := testlib.DrainChat(ctx, ch)
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
