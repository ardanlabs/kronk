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
	// This is the only suite that decodes two sequences concurrently, and on
	// ROCm it hits upstream2.md's defect: two sequences sharing one decode
	// batch come back with corrupted logits, which leaves a message empty.
	//
	// Confirmed across two ROCm major versions at llama.cpp b10798, so the
	// runtime is not what carries it:
	//
	//   7.2.4  run 33928022827  pass
	//   7.2.4  run 33929435557  fail, 1 empty (request 6)
	//   10.0   run 33931215130  fail, 2 empty (requests 0 and 9)
	//
	// Intermittent, so a green run does not clear it — dropping this skip
	// needs several consecutive green ROCm runs, not one. It also runs first
	// in gpu.yml's suite list, so while it is un-skipped a failure here aborts
	// the step and costs the answer for every suite behind it.
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
