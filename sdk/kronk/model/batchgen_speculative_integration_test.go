package model_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestClassicDraftSamplingTransitions(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	target, err := mdls.FullPath("unsloth/Qwen3-1.7B-Q4_K_M")
	if err != nil || len(target.ModelFiles) == 0 {
		t.Skip("Qwen3-1.7B model not downloaded")
	}
	draft, err := mdls.FullPath("unsloth/Qwen3-0.6B-Q8_0")
	if err != nil || len(draft.ModelFiles) == 0 {
		t.Skip("Qwen3-0.6B draft model not downloaded")
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	krn, err := kronk.New(model.WithModelFiles(target.ModelFiles),
		model.WithDraftModel(&model.DraftModelConfig{ModelFiles: draft.ModelFiles, NDraft: 4}),
		model.WithSpeculationMode(model.SpeculationClassic), model.WithNSeqMax(1),
		model.WithContextWindow(8192), model.WithPrefillBatchSize(512),
		model.WithCacheTypeK(model.GGMLTypeQ8_0), model.WithCacheTypeV(model.GGMLTypeQ8_0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unload: %v", err)
		}
	})

	// All requests share the same model and slot. Repeated greedy requests
	// exercise the steady state; stochastic requests replace its sampler.
	for _, tc := range []struct {
		name        string
		temperature float64
		word        string
		cancel      bool
	}{
		{"greedy", 0, "ALPHA", false},
		{"greedy-reuse", 0, "BRAVO", false},
		{"stochastic", .7, "", false},
		{"greedy-after-stochastic", 0, "COPPER", false},
		{"cancel-stochastic", .7, "", true},
		{"greedy-after-cancellation", 0, "DELTA", false},
	} {
		if !t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			prompt := "Explain how vegetable gardens recover from drought. Give concrete examples."
			if tc.word != "" {
				prompt = "Reply with only the uppercase word " + tc.word + ", without punctuation or explanation."
			}
			stream, err := krn.ChatStreaming(ctx, model.D{
				"messages":    model.Messages(model.TextMessage(model.RoleUser, prompt)),
				"temperature": tc.temperature, "seed": 42, "enable_thinking": false,
				"max_tokens": 128, "stream_options": model.D{"include_usage": true},
			})
			if err != nil {
				t.Fatal(err)
			}
			var text strings.Builder
			var usage *model.Usage
			var finished, cancelled bool
			deadline := time.NewTimer(time.Minute)
			defer deadline.Stop()
			for {
				select {
				case <-deadline.C:
					t.Fatal("stream did not terminate")
				case resp, ok := <-stream:
					if !ok {
						if tc.cancel {
							if !cancelled {
								t.Fatal("stream ended before cancellation during generation")
							}
							return
						}
						if !finished || usage == nil || usage.DraftTokens == 0 || strings.TrimSpace(text.String()) == "" {
							t.Fatalf("incomplete classic response: finished=%t usage=%+v text=%q", finished, usage, text.String())
						}
						if tc.word != "" && strings.TrimSpace(text.String()) != tc.word {
							t.Errorf("answer: got %q, want %q", text.String(), tc.word)
						}
						return
					}
					for _, choice := range resp.Choices {
						if choice.Delta != nil {
							text.WriteString(choice.Delta.Content)
						}
						if reason := choice.FinishReason(); reason != "" {
							if reason == model.FinishReasonError && !cancelled {
								t.Fatalf("generation error: %+v", choice)
							}
							finished = true
						}
					}
					if resp.Usage != nil {
						usage = resp.Usage
					}
					if tc.cancel && !cancelled && len(strings.Fields(text.String())) >= 12 {
						cancelled = true
						cancel()
					}
				}
			}
		}) {
			t.FailNow()
		}
	}
}
