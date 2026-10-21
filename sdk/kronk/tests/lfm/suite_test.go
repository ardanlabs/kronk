package lfm_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgLFMChat(), func(t *testing.T, krn *kronk.Kronk) {
		if got := krn.ModelInfo().Type; got != "Dense" {
			t.Fatalf("parameter activation: got %q, want Dense", got)
		}
		if got := krn.ModelInfo().SequenceProcessing; got != "Hybrid" {
			t.Fatalf("sequence processing: got %q, want Hybrid", got)
		}
		if got := krn.ModelInfo().Architecture; got != "lfm2" {
			t.Fatalf("architecture: got %q, want lfm2", got)
		}

		t.Run("Chat", func(t *testing.T) { testChat(t, krn) })
		t.Run("RecurrentStateMultiSlot", func(t *testing.T) { testRecurrentStateMultiSlot(t, krn) })
	})
}

func testChat(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	resp, err := krn.Chat(ctx, testlib.StateRequest("CHARLIE", "ZULU", 42))
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, false, false); err != nil {
		t.Fatal(err)
	}

	content := testlib.GetMsg(resp.Choices[0], false).Content
	if err := testlib.CheckStateReply(resp, content, "CHARLIE"); err != nil {
		t.Fatal(err)
	}
}

func testRecurrentStateMultiSlot(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	words := []string{"ALPHA", "BRAVO", "COPPER"}
	requests := []model.D{
		testlib.StateRequest("ALPHA", "ZULU", 101),
		testlib.StateRequest("BRAVO", "ZULU", 202),
		testlib.StateRequest("COPPER", "ZULU", 303),
	}
	baseline := make([]string, len(requests))
	for i, request := range requests {
		resp, err := krn.Chat(ctx, request)
		if err != nil {
			t.Fatalf("serial baseline %d: %v", i, err)
		}
		if len(resp.Choices) != 1 {
			t.Fatalf("serial baseline %d: expected one choice", i)
		}
		content := testlib.GetMsg(resp.Choices[0], false).Content
		if err := testlib.CheckStateReply(resp, content, words[i]); err != nil {
			t.Fatalf("serial baseline %d: %v", i, err)
		}
		baseline[i] = content
	}

	// Three warmed conversations oversubscribe two execution slots. Reverse
	// request order on replay; the barrier does not guarantee arrival order.
	first := runConcurrentStreaming(ctx, krn, requests)
	second := runConcurrentStreaming(ctx, krn, []model.D{requests[2], requests[1], requests[0]})

	for i, codeWord := range words {
		for pass, result := range []streamResult{first[i], second[len(words)-1-i]} {
			if result.err != nil {
				t.Errorf("pass %d conversation %s: %v", pass, codeWord, result.err)
				continue
			}
			if err := testlib.CheckStateReply(result.response, result.content, codeWord); err != nil {
				t.Errorf("pass %d conversation %s: %v", pass, codeWord, err)
				continue
			}
			if result.content != baseline[i] {
				t.Errorf("pass %d conversation %s: got %q, want serial baseline %q", pass, codeWord, result.content, baseline[i])
			}
			if result.response.Usage.PromptTokensDetails.CachedTokens == 0 {
				t.Errorf("pass %d conversation %s did not reuse cached prompt tokens", pass, codeWord)
			}
		}
	}
}

type streamResult struct {
	content  string
	response model.ChatResponse
	err      error
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

			last, content, err := testlib.DrainStateChat(ctx, ch)
			if err != nil {
				results[i].err = err
				return
			}
			if content == "" {
				results[i].err = fmt.Errorf("empty content")
				return
			}

			results[i].content = content
			results[i].response = last
		}()
	}

	close(start)
	wg.Wait()
	return results
}
