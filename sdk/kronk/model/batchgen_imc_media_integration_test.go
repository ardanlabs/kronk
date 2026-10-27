package model_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestIMCSystemCacheAcrossMediaRequests(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	path, err := mdls.FullPath("unsloth/Qwen3.5-0.8B-Q8_0")
	if err != nil || len(path.ModelFiles) == 0 || path.ProjFile == "" {
		t.Skip("Qwen3.5-0.8B vision model not downloaded")
	}
	imageA, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples/giraffe.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	imageB, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples/adetailer-face.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	// Use a prefix-compatible ChatML template: the model's bundled template
	// rejects system-only renders, independently of whether media is present.
	const template = `{% for message in messages %}{{ '<|im_start|>' + message.role + '\n' + message.content + '<|im_end|>\n' }}{% endfor %}{% if add_generation_prompt %}{{ '<|im_start|>assistant\n<think>\n\n</think>\n\n' }}{% endif %}`
	system := strings.Repeat("Describe only what is visible. Be brief and accurate. ", 30)
	tests := []struct {
		name     string
		system   string
		image    []byte
		prompt   string
		wantText string
		reuse    string
	}{
		{name: "first image", system: system, image: imageA, prompt: "Name the animal.", wantText: "giraffe"},
		{name: "different image", system: system, image: imageB, prompt: "Describe the subject.", wantText: "suit", reuse: "system"},
		{name: "text after media", system: system, prompt: "Say hello.", wantText: "hello", reuse: "system"},
		{name: "media after text", system: system, image: imageA, prompt: "Describe the animal's neck.", wantText: "neck", reuse: "system"},
		{name: "changed system", system: "Use one short sentence. " + system, image: imageB, prompt: "Describe the subject.", wantText: "suit"},
		{name: "exact media replay", system: "Use one short sentence. " + system, image: imageB, prompt: "Describe the subject.", wantText: "suit", reuse: "complete"},
	}
	coldPromptTokens := make([]int, len(tests))
	for _, caching := range []bool{false, true} {
		name := "uncached"
		if caching {
			name = "cached"
		}
		t.Run(name, func(t *testing.T) {
			krn, err := kronk.New(model.WithModelFiles(path.ModelFiles), model.WithProjFile(path.ProjFile),
				model.WithContextWindow(8192), model.WithNSeqMax(1), model.WithQueueDepth(1), model.WithIMCSessionCapacity(1),
				model.WithPrefillBatchSize(256), model.WithIncrementalCache(caching), model.WithCacheMinTokens(1),
				model.WithJinjaScript(template),
				model.WithSpeculationMode(model.MustParseSpeculationMode("disabled")))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := krn.Unload(context.Background()); err != nil {
					t.Errorf("unload: %v", err)
				}
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			for i, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					messages := model.Messages(model.TextMessage(model.RoleSystem, tt.system))
					if tt.image != nil {
						messages = append(messages, model.ImageMessage(tt.prompt, tt.image, "image")...)
					} else {
						messages = append(messages, model.TextMessage(model.RoleUser, tt.prompt))
					}
					response, err := krn.Chat(ctx, model.D{"messages": messages,
						"max_tokens": 24, "temperature": 0.0, "top_k": 1, "enable_thinking": false})
					if err != nil {
						t.Fatal(err)
					}
					if response.Usage == nil || response.Usage.CompletionTokens == 0 || len(response.Choices) == 0 {
						t.Fatalf("response: got %+v, want generated output and usage", response)
					}
					// Decode boundaries and backend arithmetic can change prose.
					// Check the subject instead of requiring byte-identical output.
					if got := response.Choices[0].Message.Content; !strings.Contains(strings.ToLower(got), tt.wantText) {
						t.Errorf("content: got %q, want mention of %q", got, tt.wantText)
					}
					if !caching {
						coldPromptTokens[i] = response.Usage.PromptTokens
						return
					}
					if got, want := response.Usage.PromptTokens, coldPromptTokens[i]; got != want {
						t.Errorf("prompt tokens: got %d, want uncached %d", got, want)
					}
					caches := krn.IMCSystemCaches()
					if len(caches) != 1 || caches[0].Tokens == 0 || caches[0].SnapshotBytes == 0 || caches[0].ActiveRestores != 0 {
						t.Fatalf("system caches: got %+v, want one published, unreserved snapshot", caches)
					}
					cached := response.Usage.PromptTokensDetails.CachedTokens
					switch tt.reuse {
					case "system":
						if cached != caches[0].Tokens {
							t.Errorf("cached tokens: got %d, want system prefix %d", cached, caches[0].Tokens)
						}
					case "complete":
						if cached <= caches[0].Tokens {
							t.Errorf("cached tokens: got %d, want complete media snapshot larger than %d", cached, caches[0].Tokens)
						}
					default:
						if cached != 0 {
							t.Errorf("cached tokens: got %d, want 0", cached)
						}
					}
					t.Logf("prompt=%d cached=%d system=%d", response.Usage.PromptTokens, cached, caches[0].Tokens)
				})
			}
		})
	}
}

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
	setupEntered := make(chan struct{})
	setupRelease := make(chan struct{})
	releaseSetup := sync.OnceFunc(func() { close(setupRelease) })
	defer releaseSetup()
	var setupOnce sync.Once
	krn, err := kronk.New(model.WithModelFiles(path.ModelFiles), model.WithProjFile(path.ProjFile),
		model.WithContextWindow(16384), model.WithNSeqMax(2), model.WithPrefillBatchSize(256),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithLog(func(ctx context.Context, msg string, args ...any) {
			t.Logf("%s %v", msg, args)
			if msg == "imc-media-cache" && len(args) >= 2 && args[0] == "status" && args[1] == "prompt-created" {
				// Hold CPU preparation at an existing logging boundary. Assert
				// actual generation progress, not timing or buffered delivery.
				setupOnce.Do(func() {
					close(setupEntered)
					select {
					case <-setupRelease:
					case <-ctx.Done():
					}
				})
			}
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
	// Cover both requests and cache reuse on slower CI inference backends.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
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
		releaseSetup()
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
	setupTimeout := time.NewTimer(15 * time.Second)
	setupTimeout.Stop()
	defer setupTimeout.Stop()
	setupSignal := setupEntered
	setupFirstGenerated := -1
	setupProgress := false
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
		case <-setupTimeout.C:
			t.Fatal("generating request made no progress while CPU media preparation was held")
		case <-setupSignal:
			setupSignal = nil
			setupTimeout.Reset(15 * time.Second)
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
			if setupSignal == nil && !setupProgress && generated >= 0 {
				if setupFirstGenerated < 0 {
					setupFirstGenerated = generated
				}
				if generated > setupFirstGenerated {
					setupProgress = true
					setupTimeout.Stop()
					releaseSetup()
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
	if !setupProgress {
		t.Error("generating request made no progress during CPU media preparation")
	}
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
