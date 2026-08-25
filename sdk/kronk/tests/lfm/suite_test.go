package lfm_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgLFMChat(), func(t *testing.T, krn *kronk.Kronk) {
		if got := krn.ModelInfo().Type; got != model.ModelTypeHybrid {
			t.Fatalf("model type = %s, want hybrid", got)
		}
		if got := krn.ModelInfo().Metadata["general.architecture"]; !strings.EqualFold(got, "lfm2") {
			t.Fatalf("architecture = %q, want lfm2", got)
		}

		t.Run("Chat", func(t *testing.T) { testChat(t, krn) })
		t.Run("RecurrentStateMultiSlot", func(t *testing.T) { testRecurrentStateMultiSlot(t, krn) })
	})
}

func testChat(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	resp, err := krn.Chat(ctx, lfmRequest("CHARLIE", 42, false))
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, false, false); err != nil {
		t.Fatal(err)
	}

	content := testlib.GetMsg(resp.Choices[0], false).Content
	if !strings.Contains(strings.ToUpper(content), "CHARLIE") {
		t.Fatalf("content = %q, want code word CHARLIE", content)
	}
}

func testRecurrentStateMultiSlot(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	requests := []model.D{
		lfmRequest("ALPHA", 101, true),
		lfmRequest("BRAVO", 202, true),
	}

	first := runConcurrentStreaming(ctx, krn, requests)
	second := runConcurrentStreaming(ctx, krn, requests)

	for i, codeWord := range []string{"ALPHA", "BRAVO"} {
		if first[i].err != nil {
			t.Errorf("first pass slot %d: %v", i, first[i].err)
			continue
		}
		if second[i].err != nil {
			t.Errorf("restored pass slot %d: %v", i, second[i].err)
			continue
		}
		if !strings.Contains(strings.ToUpper(first[i].content), codeWord) {
			t.Errorf("first pass slot %d content = %q, want code word %s", i, first[i].content, codeWord)
		}
		if first[i].content != second[i].content {
			t.Errorf("restored pass slot %d content = %q, want deterministic replay %q", i, second[i].content, first[i].content)
		}
		if second[i].usage == nil || second[i].usage.PromptTokensDetails.CachedTokens == 0 {
			t.Errorf("restored pass slot %d did not report cached prompt tokens", i)
		}
	}
}

type streamResult struct {
	content string
	usage   *model.Usage
	err     error
}

func runConcurrentStreaming(ctx context.Context, krn *kronk.Kronk, requests []model.D) []streamResult {
	results := make([]streamResult, len(requests))
	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(len(requests))
	for i, request := range requests {
		go func() {
			defer wg.Done()
			<-start

			ch, err := krn.ChatStreaming(ctx, request)
			if err != nil {
				results[i].err = fmt.Errorf("chat streaming: %w", err)
				return
			}

			last, content, err := testlib.DrainChat(ctx, ch)
			if err != nil {
				results[i].err = err
				return
			}
			if content == "" {
				results[i].err = fmt.Errorf("empty content")
				return
			}

			results[i].content = content
			results[i].usage = last.Usage
		}()
	}

	close(start)
	wg.Wait()
	return results
}

func lfmRequest(codeWord string, seed uint32, streaming bool) model.D {
	d := model.D{
		"messages": []model.D{
			{"role": "system", "content": "This conversation's code word is " + codeWord + ". Follow the user's formatting instruction exactly."},
			{"role": "user", "content": "Reply with only the uppercase code word " + codeWord + "."},
		},
		"max_tokens":  16,
		"temperature": 0,
		"seed":        seed,
	}
	if streaming {
		d["stream_options"] = model.D{"include_usage": true}
	}
	return d
}
