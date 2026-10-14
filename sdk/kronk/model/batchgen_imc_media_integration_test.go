package model_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestIMCMediaPreparationAllowsGenerationProgress(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	path, err := mdls.FullPath("unsloth/Qwen3.5-0.8B-Q8_0")
	if err != nil || len(path.ModelFiles) == 0 || path.ProjFile == "" {
		t.Skip("Qwen3.5-0.8B vision model not downloaded")
	}
	image, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples/giraffe.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	krn, err := kronk.New(model.WithModelFiles(path.ModelFiles), model.WithProjFile(path.ProjFile),
		model.WithContextWindow(16384), model.WithNSeqMax(2), model.WithPrefillBatchSize(256),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithLog(func(ctx context.Context, msg string, args ...any) {
			t.Logf("%s %v", msg, args)
		}),
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
	actx, cancelA := context.WithCancel(ctx)
	defer cancelA()
	a, err := krn.ChatStreaming(actx, model.D{
		"messages": model.Messages(model.TextMessage(model.RoleSystem, "Counting session."),
			model.TextMessage(model.RoleUser, "Count from 1 to 5000, one number per line. Output only the numbers and do not stop early.")),
		"max_tokens": 4096, "temperature": 0.0, "top_k": 1, "enable_thinking": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Admit the media request only after the other request is generating.
	for {
		resp, ok := <-a
		if !ok {
			t.Fatal("counting stream ended before output")
		}
		if len(resp.Choices) > 0 && resp.Choices[0].Delta != nil {
			choice := resp.Choices[0]
			if choice.FinishReason() == model.FinishReasonError {
				t.Fatal(choice.Delta.Content)
			}
			if choice.Delta.Content != "" || choice.Delta.Reasoning != "" {
				break
			}
		}
	}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range a {
		}
	}()
	defer func() {
		cancelA()
		<-drained
	}()

	content := model.ImageMessage("Describe the animal in a short sentence.", image, "jpg")[0]["content"].([]model.D)
	parts := []model.D{content[0], content[0], content[0], content[1]}
	doc := model.D{
		"messages":   model.Messages(model.TextMessage(model.RoleSystem, "Image session."), model.D{"role": "user", "content": parts}),
		"max_tokens": 16, "temperature": 0.0, "top_k": 1, "seed": 42, "enable_thinking": false,
	}
	results := make(chan model.ChatResponse, 1)
	errs := make(chan error, 1)
	go func() {
		resp, err := krn.Chat(ctx, doc)
		if err != nil {
			errs <- err
			return
		}
		results <- resp
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	firstGenerated := -1
	progress := false
	var response model.ChatResponse
waiting:
	for {
		select {
		case err := <-errs:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case response = <-results:
			break waiting
		case <-ticker.C:
			snapshot, ok := krn.BatchEngineSnapshot()
			if !ok {
				t.Fatal("generation scheduler unavailable")
			}
			preparing := false
			generated := -1
			for _, slot := range snapshot.Slots {
				if slot.Phase == "media-prefill" && slot.IMCPreparedTokens > 0 && slot.IMCPreparationRemaining > 0 {
					preparing = true
				}
				if slot.Phase == "generation" {
					generated = slot.GeneratedTokens
				}
			}
			if preparing && generated >= 0 {
				if firstGenerated < 0 {
					firstGenerated = generated
				}
				progress = progress || generated > firstGenerated
			}
		}
	}
	cancelA()
	<-drained
	if !progress {
		t.Error("generating request made no progress while media prefix preparation was incomplete")
	}
	if response.Usage == nil || response.Usage.CompletionTokens == 0 || len(response.Choices) == 0 {
		t.Fatalf("media response: got %+v, want generated output and usage", response)
	}
	repeated, err := krn.Chat(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Usage == nil || repeated.Usage.PromptTokensDetails.CachedTokens == 0 || repeated.Usage.PromptTokens != response.Usage.PromptTokens || len(repeated.Choices) == 0 {
		t.Fatalf("cache reuse: first=%+v repeated=%+v", response.Usage, repeated.Usage)
	}
	if got, want := repeated.Choices[0].Message.Content, response.Choices[0].Message.Content; got != want {
		t.Errorf("cached response: got %q, want %q", got, want)
	}
	t.Logf("media prompt=%d cached=%d; concurrent progress=%t",
		repeated.Usage.PromptTokens, repeated.Usage.PromptTokensDetails.CachedTokens, progress)
}
