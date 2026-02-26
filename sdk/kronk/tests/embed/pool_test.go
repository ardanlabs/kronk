package embed_test

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

func Test_PooledEmbeddings(t *testing.T) {
	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:     testlib.MPEmbed.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
		NSeqMax:        numInstances,
	})
	if err != nil {
		t.Fatalf("Failed to create embedding model with NSeqMax=%d: %v", numInstances, err)
	}
	defer krn.Unload(ctx)

	t.Logf("Testing pooled embeddings with NSeqMax=%d", numInstances)

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

			resp, err := krn.Embeddings(ctx, model.D{
				"input": "The quick brown fox jumps over the lazy dog",
			})
			if err != nil {
				errors[idx] = fmt.Errorf("goroutine %d: %w", idx, err)
				return
			}

			durations[idx] = time.Since(start)

			if len(resp.Data) != 1 {
				errors[idx] = fmt.Errorf("goroutine %d: expected 1 embedding, got %d", idx, len(resp.Data))
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

	t.Logf("All %d concurrent embedding requests completed successfully", numInstances)
}
