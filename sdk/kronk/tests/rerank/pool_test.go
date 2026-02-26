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

func Test_PooledRerank(t *testing.T) {
	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:     testlib.MPRerank.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
		NSeqMax:        numInstances,
	})
	if err != nil {
		t.Fatalf("Failed to create rerank model with NSeqMax=%d: %v", numInstances, err)
	}
	defer krn.Unload(ctx)

	t.Logf("Testing pooled rerank with NSeqMax=%d", numInstances)

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
