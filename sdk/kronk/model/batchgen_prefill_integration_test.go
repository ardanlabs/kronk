package model_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestTextPrefillCancellationAndReuse(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	path, err := mdls.FullPath("unsloth/Qwen3-0.6B-Q8_0")
	if err != nil || len(path.ModelFiles) == 0 {
		t.Skip("Qwen3-0.6B model not downloaded")
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}

	for _, imc := range []bool{false, true} {
		t.Run(fmt.Sprintf("IMC=%t", imc), func(t *testing.T) {
			krn, err := kronk.New(model.WithModelFiles(path.ModelFiles),
				model.WithContextWindow(8192), model.WithNSeqMax(1),
				model.WithPrefillBatchSize(64), model.WithPhysicalBatchSize(32),
				model.WithIncrementalCache(imc), model.WithCacheMinTokens(1),
				model.WithSpeculationMode(model.MustParseSpeculationMode("disabled")))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := krn.Unload(context.Background()); err != nil {
					t.Errorf("unload: %v", err)
				}
			})
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			cancelCtx, cancelPrefill := context.WithCancel(ctx)
			defer cancelPrefill()
			background := strings.Repeat("This is background information about crates in a warehouse.\n", 300)
			stream, err := krn.ChatStreaming(cancelCtx, model.D{
				"messages": model.Messages(model.TextMessage(model.RoleUser, background+
					"The secret code is VIOLET-739. Reply with only the secret code.")),
				"max_tokens": 24, "temperature": 0.0, "top_k": 1, "enable_thinking": false,
			})
			if err != nil {
				t.Fatal(err)
			}
			drained := make(chan struct{})
			go func() {
				defer close(drained)
				for range stream {
				}
			}()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
		waiting:
			for {
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-drained:
					t.Fatal("request finished before partial prefill was observed")
				case <-ticker.C:
					snapshot, ok := krn.BatchEngineSnapshot()
					if !ok {
						t.Fatal("generation scheduler unavailable")
					}
					for _, slot := range snapshot.Slots {
						if (slot.PrefilledTokens > 0 && slot.PrefillRemaining > 0) ||
							(slot.IMCPreparedTokens > 0 && slot.IMCPreparationRemaining > 0) {
							cancelPrefill()
							break waiting
						}
					}
				}
			}
			select {
			case <-drained:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			// Reuse the same slot with a different answer after partial prefill
			// cancellation, then exercise a published IMC snapshot on repetition.
			doc := model.D{
				"messages": model.Messages(model.TextMessage(model.RoleUser, background+
					"The secret code is AMBER-482. Reply with only the secret code.")),
				"max_tokens": 24, "temperature": 0.0, "top_k": 1, "seed": 42, "enable_thinking": false,
			}
			for run := range 2 {
				resp, err := krn.Chat(ctx, doc)
				if err != nil {
					t.Fatal(err)
				}
				if len(resp.Choices) != 1 || resp.Choices[0].Message == nil {
					t.Fatalf("response: got %+v, want one answer", resp)
				}
				if got := strings.TrimSpace(resp.Choices[0].Message.Content); got != "AMBER-482" {
					t.Errorf("answer after cancellation: got %q, want AMBER-482", got)
				}
				if resp.Usage == nil || resp.Usage.PromptTokens <= 64 || resp.Usage.CompletionTokens == 0 {
					t.Fatalf("usage: got %+v, want multi-chunk prompt and completion", resp.Usage)
				}
				if imc && run == 1 && resp.Usage.PromptTokensDetails.CachedTokens == 0 {
					t.Error("repeated request did not reuse the IMC snapshot")
				}
			}
		})
	}
}
