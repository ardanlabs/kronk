package vision_test

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

func Test_PooledVision(t *testing.T) {
	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:    testlib.MPSimpleVision.ModelFiles,
		ProjFile:      testlib.MPSimpleVision.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
		NSeqMax:       numInstances,
	})
	if err != nil {
		t.Fatalf("Failed to create vision model with NSeqMax=%d: %v", numInstances, err)
	}
	defer krn.Unload(ctx)

	t.Logf("Testing pooled vision with NSeqMax=%d", numInstances)

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

			resp, err := krn.Chat(ctx, testlib.DMedia)
			if err != nil {
				errors[idx] = fmt.Errorf("goroutine %d: %w", idx, err)
				return
			}

			durations[idx] = time.Since(start)

			if len(resp.Choice) == 0 {
				errors[idx] = fmt.Errorf("goroutine %d: expected response choices, got none", idx)
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

	t.Logf("All %d concurrent vision requests completed successfully", numInstances)
}
