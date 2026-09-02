package qwen3_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_BatchChatConcurrent(t *testing.T) {
	// ROCm decodes two sequences that share one batch incorrectly: both come
	// back with corrupted logits, so one request repeats a single token until
	// it reaches max_tokens while the other stops after a handful. Either
	// shape can leave the message empty, which is what the check below trips
	// on. Deterministic with two concurrent requests on a fresh context, and
	// never reproduced on Vulkan against the same GPU, host and llama.cpp
	// build (b10760, gfx1151) — the report is upstream2.md. A backend defect,
	// not a regression here.
	//
	// The condition is two sequences in one decode batch, nothing narrower:
	// the same run is clean with NSeqMax=1, and clean at NSeqMax=2 as long as
	// the requests never overlap. So this suite is the one that meets it —
	// everything else the GPU workflow runs is serial. Drop the skip when the
	// fix lands, because this test is what notices.
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")

	testlib.WithModel(t, testlib.CfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
		g := 10

		t.Logf("Testing batch inference with %d concurrent requests", g)

		var wg sync.WaitGroup
		wg.Add(g)

		startBarrier := make(chan struct{})

		results := make([]struct {
			id       int
			duration time.Duration
			err      error
			content  string
		}, g)

		for i := range g {
			go func(idx int) {
				defer wg.Done()

				<-startBarrier

				ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
				defer cancel()

				start := time.Now()

				ch, err := krn.ChatStreaming(ctx, testlib.DChatNoTool)
				if err != nil {
					results[idx].err = fmt.Errorf("goroutine %d: chat streaming error: %w", idx, err)
					return
				}

				var lastResp model.ChatResponse
				for resp := range ch {
					lastResp = resp
				}

				results[idx].duration = time.Since(start)
				results[idx].id = idx

				if lastResp.Choices[0].FinishReason() == model.FinishReasonError {
					errContent := ""
					if lastResp.Choices[0].Delta != nil {
						errContent = lastResp.Choices[0].Delta.Content
					}
					results[idx].err = fmt.Errorf("goroutine %d: got error response: %s", idx, errContent)
					return
				}

				msg := testlib.GetMsg(lastResp.Choices[0], true)
				results[idx].content = msg.Content
			}(i)
		}

		close(startBarrier)
		wg.Wait()

		var errors []error
		var totalDuration time.Duration
		for _, r := range results {
			if r.err != nil {
				errors = append(errors, r.err)
				continue
			}

			totalDuration += r.duration
			t.Logf("Request %d completed in %s", r.id, r.duration)

			if r.content == "" {
				errors = append(errors, fmt.Errorf("request %d: empty content", r.id))
			}
		}

		if len(errors) > 0 {
			for _, err := range errors {
				t.Error(err)
			}
			t.FailNow()
		}

		avgDuration := totalDuration / time.Duration(g)
		t.Logf("All %d requests completed. Average duration: %s", g, avgDuration)
	})
}
