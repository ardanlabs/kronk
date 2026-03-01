package moe_test

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
	testlib.WithModel(t, testlib.CfgMoEVision(), func(t *testing.T, krn *kronk.Kronk) {
		g := 10

		t.Logf("Testing MoE batch inference with %d concurrent requests", g)

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

				if lastResp.Choice[0].FinishReason() == model.FinishReasonError {
					errContent := ""
					if lastResp.Choice[0].Delta != nil {
						errContent = lastResp.Choice[0].Delta.Content
					}
					results[idx].err = fmt.Errorf("goroutine %d: got error response: %s", idx, errContent)
					return
				}

				msg := testlib.GetMsg(lastResp.Choice[0], true)
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
		t.Logf("All %d MoE requests completed. Average duration: %s", g, avgDuration)
	})
}
