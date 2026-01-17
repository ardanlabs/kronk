package kronk_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Test_BatchChatConcurrent verifies that the batch engine correctly handles
// multiple concurrent chat requests. It launches 10 goroutines simultaneously
// and verifies all responses are correct (no corruption from parallel processing).
//
// Run with: go test -v -run Test_BatchChatConcurrent
func Test_BatchChatConcurrent(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping batch test in GitHub Actions (requires more resources)")
	}

	withModel(t, cfgThinkToolChat(), func(t *testing.T, krn *kronk.Kronk) {
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

				ctx, cancel := context.WithTimeout(context.Background(), testDuration)
				defer cancel()

				start := time.Now()

				ch, err := krn.ChatStreaming(ctx, dChatNoTool)
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

				if lastResp.Choice[0].FinishReason == model.FinishReasonError {
					results[idx].err = fmt.Errorf("goroutine %d: got error response: %s", idx, lastResp.Choice[0].Delta.Content)
					return
				}

				msg := getMsg(lastResp.Choice[0], true)
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
