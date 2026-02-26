package gpt_test

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_CacheIMCNonDeterministic(t *testing.T) {
	cfg := model.Config{
		ModelFiles:       testlib.MPGPTChat.ModelFiles,
		ContextWindow:    8192,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
		CacheMinTokens:   100,
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

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
	})
}
