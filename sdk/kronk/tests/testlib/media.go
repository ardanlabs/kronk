package testlib

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestMediaState checks media replay, history extension and replacement on a
// serial IMC instance. Expected subjects are fixture facts, never prompt hints.
func TestMediaState(t *testing.T, krn *kronk.Kronk) {
	t.Helper()
	TestMediaHistory(t, func(ctx context.Context, request model.D) (model.ChatResponse, string, error) {
		ch, err := krn.ChatStreaming(ctx, request)
		if err != nil {
			return model.ChatResponse{}, "", err
		}
		return DrainStateChat(ctx, ch)
	})
}

// TestMediaHistory checks the same image-state contracts through SDK or HTTP
// streaming. The caller supplies transport validation, not expected answers.
func TestMediaHistory(t *testing.T, chat func(context.Context, model.D) (model.ChatResponse, string, error)) {
	t.Helper()
	workspace := os.Getenv("GITHUB_WORKSPACE")
	if workspace == "" {
		t.Fatal("GITHUB_WORKSPACE is required for media fixtures")
	}
	const prompt = "Name the animal in the most recent picture. Reply with only the animal name."
	image := func(path string) []model.D {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(workspace, "examples", "samples", path))
		if err != nil {
			t.Fatalf("reading media fixture %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("empty media fixture %s", path)
		}
		return model.ImageMessage(prompt, data, "jpeg")
	}
	giraffe := image("giraffe.jpg")
	deer := image("deer/image10.jpg")

	run := func(name string, messages []model.D, want, foreign string, reuse bool) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
		defer cancel()
		resp, content, err := chat(ctx, model.D{
			"messages": messages, "temperature": 0.0, "enable_thinking": false,
			"max_tokens": 64, "stream_options": model.D{"include_usage": true},
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// Reuse terminal/usage validation without demanding identical wording.
		if strings.TrimSpace(content) == "" {
			t.Fatalf("%s: empty answer content", name)
		}
		if err := CheckStateReply(resp, content, strings.TrimSpace(content)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		answer := strings.ToLower(content)
		if !strings.Contains(answer, want) || (foreign != "" && strings.Contains(answer, foreign)) {
			t.Fatalf("%s: got %q, want %s without %s", name, content, want, foreign)
		}
		cached := resp.Usage.PromptTokensDetails.CachedTokens
		if reuse && cached <= 0 {
			t.Fatalf("%s: got %d cached tokens, want positive prefix reuse", name, cached)
		}
		t.Logf("%s: content=%q prompt=%d cached=%d", name, content, resp.Usage.PromptTokens, cached)
		return content
	}
	appendTurn := func(messages []model.D, answer, question string) []model.D {
		return append(slices.Clone(messages),
			model.D{"role": "assistant", "content": answer},
			model.D{"role": "user", "content": question})
	}

	first := run("image cold", giraffe, "giraffe", "deer", false)
	run("image exact replay", giraffe, "giraffe", "deer", true)
	history := appendTurn(giraffe, first, prompt)
	answer := run("image append", history, "giraffe", "deer", true)
	history = appendTurn(history, answer, "What is the capital of France? Reply with only the city name.")
	answer = run("unrelated text within image history", history, "paris", "", true)
	history = appendTurn(history, answer, prompt)
	run("image recall after unrelated text", history, "giraffe", "deer", true)

	text := []model.D{{"role": "user", "content": "What is the capital of Japan? Reply with only the city name."}}
	answer = run("independent text", text, "tokyo", "", false)
	transition := append(slices.Clone(text), model.D{"role": "assistant", "content": answer})
	transition = append(transition, giraffe...)
	answer = run("text to image", transition, "giraffe", "deer", false)
	transition = appendTurn(transition, answer, prompt)
	run("text to image append", transition, "giraffe", "deer", true)

	changed := append(slices.Clone(giraffe), model.D{"role": "assistant", "content": first})
	changed = append(changed, deer...)
	answer = run("new image in existing history", changed, "deer", "giraffe", false)
	run("new image history append", appendTurn(changed, answer, "What animal is in the second image? Reply with only its name."), "deer", "giraffe", true)

	answer = run("independent deer image", deer, "deer", "giraffe", false)
	run("deer exact replay", deer, "deer", "giraffe", true)
	run("deer append", appendTurn(deer, answer, prompt), "deer", "giraffe", true)
	run("original giraffe after replacement", giraffe, "giraffe", "deer", false)
}
