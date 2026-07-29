package rerank_test

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

func TestConcurrentRerank(t *testing.T) {
	const numInstances = 2

	tests := []struct {
		name      string
		available bool
		cfg       model.Config
	}{
		{"Qwen3ContextPoolFallback", len(testlib.MPRerankFallback.ModelFiles) > 0, testlib.CfgRerankFallback()},
		{"BGEBatchSeq", len(testlib.MPRerankBatchSeq.ModelFiles) > 0, testlib.CfgRerankBatchSeq()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.available {
				t.Skip("model not downloaded")
			}

			testlib.WithModel(t, tt.cfg, func(t *testing.T, krn *kronk.Kronk) {
				testConcurrentRerank(t, krn, numInstances)
			})
		})
	}
}

func testConcurrentRerank(t *testing.T, krn *kronk.Kronk, numInstances int) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	t.Logf("Testing concurrent rerank with NSeqMax=%d", krn.ModelConfig().NSeqMax())

	query := "What is the capital of France?"
	documents := []string{
		"Paris is the capital of France.",
		"Berlin is the capital of Germany.",
	}

	var wg sync.WaitGroup
	wg.Add(numInstances)

	startBarrier := make(chan struct{})
	durations := make([]time.Duration, numInstances)
	errors := make([]error, numInstances)

	for i := range numInstances {
		go func(idx int) {
			defer wg.Done()

			<-startBarrier

			start := time.Now()

			resp, err := krn.Rerank(ctx, model.D{
				"query":     query,
				"documents": documents,
			})
			if err != nil {
				errors[idx] = fmt.Errorf("goroutine %d: %w", idx, err)
				return
			}

			durations[idx] = time.Since(start)

			if len(resp.Data) == 0 {
				errors[idx] = fmt.Errorf("goroutine %d: expected rerank results, got none", idx)
			}
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
	}

	if t.Failed() {
		return
	}

	for i, d := range durations {
		t.Logf("Request %d completed in %s", i, d)
	}

	t.Logf("All %d concurrent rerank requests completed successfully", numInstances)
}
