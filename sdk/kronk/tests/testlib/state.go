package testlib

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestModelProfile checks independent fixture facts after loading a model.
func TestModelProfile(t *testing.T, krn *kronk.Kronk, activation, sequence, architecture string) {
	t.Helper()
	info := krn.ModelInfo()
	if info.Type != activation || info.SequenceProcessing != sequence || info.Architecture != architecture {
		t.Fatalf("profile: got %s / %s / %s, want %s / %s / %s",
			info.Type, info.SequenceProcessing, info.Architecture, activation, sequence, architecture)
	}
}

// StateRequest keeps the answers in the conversation prefix, not in the final
// user question. Both words matter: an appended turn asks for the second one.
func StateRequest(first, second string, seed uint32) model.D {
	return model.D{
		"messages": []model.D{
			{"role": "system", "content": "Your first code word is " + first + ". Your second code word is " + second + ". Reply with only the requested uppercase code word, without punctuation or explanation."},
			{"role": "user", "content": "What is your first code word?"},
		},
		"enable_thinking": false,
		"temperature":     0.0,
		"seed":            seed,
		"max_tokens":      32,
		"stream_options":  model.D{"include_usage": true},
	}
}

// DrainStateChat reconstructs answer text without a reasoning fallback and
// retains the terminal choice together with its trailing usage-only response.
func DrainStateChat(ctx context.Context, ch <-chan model.ChatResponse) (model.ChatResponse, string, error) {
	var terminal model.ChatResponse
	var content strings.Builder
	for {
		select {
		case <-ctx.Done():
			return terminal, content.String(), ctx.Err()
		case resp, ok := <-ch:
			if !ok {
				return terminal, content.String(), nil
			}
			for _, choice := range resp.Choices {
				if choice.FinishReason() == model.FinishReasonError {
					return resp, content.String(), fmt.Errorf("model error: %s", GetMsg(choice, true).Content)
				}
				if choice.Delta != nil {
					content.WriteString(choice.Delta.Content)
				}
				if choice.FinishReason() != "" {
					terminal = resp
				}
			}
			if resp.Usage != nil {
				terminal.Usage = resp.Usage
			}
		}
	}
}

// CheckStateReply rejects wrong, stale or mixed-session answers, incomplete
// streams and invalid usage. Exact answer matching excludes every foreign word.
func CheckStateReply(resp model.ChatResponse, content, want string) error {
	if len(resp.Choices) != 1 || resp.Choices[0].FinishReason() != model.FinishReasonStop {
		return fmt.Errorf("expected one completed stop choice, got %+v", resp.Choices)
	}
	if strings.TrimSpace(content) != want {
		return fmt.Errorf("content: got %q, want only %q", content, want)
	}
	if resp.Usage == nil {
		return fmt.Errorf("missing usage")
	}
	u := resp.Usage
	if u.PromptTokens <= 0 || u.CompletionTokens <= 0 || u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		return fmt.Errorf("invalid token accounting: %+v", u)
	}
	if cached := u.PromptTokensDetails.CachedTokens; cached < 0 || cached > u.PromptTokens {
		return fmt.Errorf("cached tokens %d outside prompt budget %d", cached, u.PromptTokens)
	}
	return nil
}

// TestIMCState checks a fresh one-slot instance through replay, history growth,
// divergence and replacement by another conversation, with an IMC-off control.
func TestIMCState(t *testing.T, krn *kronk.Kronk, enabled bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
	defer cancel()

	run := func(name string, request model.D, want string, reuse bool) (string, int) {
		t.Helper()
		ch, err := krn.ChatStreaming(ctx, request)
		if err != nil {
			t.Fatalf("%s: chat streaming: %v", name, err)
		}
		resp, content, err := DrainStateChat(ctx, ch)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := CheckStateReply(resp, content, want); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cached := resp.Usage.PromptTokensDetails.CachedTokens
		switch {
		case !enabled && cached != 0:
			t.Errorf("%s: IMC off, got %d cached tokens, want 0", name, cached)
		case enabled && reuse && cached == 0:
			t.Errorf("%s: expected cached prefix reuse", name)
		}
		t.Logf("%s: content=%q prompt=%d cached=%d", name, content, resp.Usage.PromptTokens, cached)
		return content, cached
	}

	initial := StateRequest("ALPHA", "DELTA", 101)
	first, cold := run("cold", initial, "ALPHA", false)
	if cold != 0 {
		t.Errorf("fresh instance: got %d cached tokens, want 0", cold)
	}
	_, replay := run("exact replay", initial, "ALPHA", true)

	extended := initial.Clone()
	messages := extended["messages"].([]model.D)
	extended["messages"] = append(messages,
		model.D{"role": "assistant", "content": first},
		model.D{"role": "user", "content": "What is your second code word?"},
	)
	second, _ := run("append actual assistant output", extended, "DELTA", true)
	_, grown := run("extended replay", extended, "DELTA", true)
	if enabled && grown <= replay {
		t.Errorf("extended prefix: got %d cached tokens, want more than initial %d", grown, replay)
	}

	continued := extended.Clone()
	continued["messages"] = append(continued["messages"].([]model.D),
		model.D{"role": "assistant", "content": second},
		model.D{"role": "user", "content": "What is your first code word?"},
	)
	run("second append using actual assistant output", continued, "ALPHA", true)

	// Change earlier history, not just the inference tail. Define the new word
	// in the System message so the User does not contradict a higher-priority
	// instruction. A shared token prefix may survive; stale answers must not.
	diverged := extended.Clone()
	divergedMessages := diverged["messages"].([]model.D)
	divergedMessages[0]["content"] = strings.Replace(divergedMessages[0]["content"].(string), "ALPHA", "COPPER", 1)
	divergedMessages[1]["content"] = "What is your second code word?"
	divergedMessages[2]["content"] = second
	divergedMessages[3]["content"] = "What is your first code word?"
	run("divergent history", diverged, "COPPER", false)
	run("independent conversation", StateRequest("BRAVO", "ECHO", 202), "BRAVO", false)
	run("original after slot reuse", initial, "ALPHA", false)
}

// TestChatIsolation compares isolated serial baselines with a synchronized
// burst of distinct histories. Callers provide more requests than decode slots;
// the barrier releases a burst, but does not prove native execution overlaps.
// requireReuse is for warmed IMC cases whose working set fits session capacity.
func TestChatIsolation(t *testing.T, krn *kronk.Kronk, words []string, requireReuse bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
	defer cancel()

	requests := make([]model.D, len(words))
	baseline := make([]string, len(words))
	run := func(request model.D, want string) (string, error) {
		ch, err := krn.ChatStreaming(ctx, request)
		if err != nil {
			return "", err
		}
		resp, content, err := DrainStateChat(ctx, ch)
		if err != nil {
			return "", err
		}
		return content, CheckStateReply(resp, content, want)
	}
	for i, word := range words {
		requests[i] = StateRequest(word, "ZULU", uint32(100+i))
		content, err := run(requests[i], word)
		if err != nil {
			t.Fatalf("serial baseline %d: %v", i, err)
		}
		baseline[i] = content
	}

	results := make([]struct {
		content  string
		err      error
		duration time.Duration
	}, len(words))
	var wg sync.WaitGroup
	wg.Add(len(words))
	start := make(chan struct{})
	for i, request := range requests {
		go func() {
			defer wg.Done()
			<-start
			began := time.Now()
			ch, err := krn.ChatStreaming(ctx, request)
			if err != nil {
				results[i].err = err
				return
			}
			resp, content, err := DrainStateChat(ctx, ch)
			if err == nil {
				err = CheckStateReply(resp, content, words[i])
			}
			if err == nil && requireReuse && resp.Usage.PromptTokensDetails.CachedTokens == 0 {
				err = fmt.Errorf("warmed conversation did not reuse its cached prefix")
			}
			results[i].content, results[i].err = content, err
			results[i].duration = time.Since(began)
		}()
	}
	close(start)
	wg.Wait()
	for i, result := range results {
		if result.err != nil {
			t.Errorf("concurrent request %d (%s): %v", i, words[i], result.err)
			continue
		}
		if result.content != baseline[i] {
			t.Errorf("concurrent request %d: got %q, want serial baseline %q", i, result.content, baseline[i])
		}
		t.Logf("request %d: code=%s duration=%s", i, words[i], result.duration)
	}
}
