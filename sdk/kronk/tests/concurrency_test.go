package kronk_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

func Test_ConTest1(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions (requires more resources)")
	}

	// This test cancels the context before the channel loop starts.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	t.Log("cancel context before channel loop")
	cancel()

	var lastResp model.ChatResponse
	for resp := range ch {
		if resp.Choice[0].FinishReason() == model.FinishReasonError {
			lastResp = resp // Only capture the error response
		}
	}

	t.Log("check conditions")

	if len(lastResp.Choice) == 0 {
		t.Log("WARNING: Didn't get any response from the api call, but channel is closed")
		return
	}

	if v := lastResp.Choice[0].FinishReason(); v != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", v)
	}

	if lastResp.Choice[0].Delta == nil || lastResp.Choice[0].Delta.Content != "context canceled" {
		errContent := ""
		if lastResp.Choice[0].Delta != nil {
			errContent = lastResp.Choice[0].Delta.Content
		}
		t.Errorf("expected error context canceled, got %s", errContent)
	}
}

func Test_ConTest2(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions (requires more resources)")
	}

	// This test cancels the context inside the channel loop.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	var lastResp model.ChatResponse
	var index int
	for resp := range ch {
		if resp.Choice[0].FinishReason() == model.FinishReasonError {
			lastResp = resp // Only capture the error response
		}

		index++
		if index == 2 {
			t.Log("cancel context inside channel loop")
			cancel()
		}
	}

	t.Log("check conditions")

	if len(lastResp.Choice) == 0 {
		t.Log("WARNING: Didn't get any response from the api call, but channel is closed")
		return
	}

	if v := lastResp.Choice[0].FinishReason(); v != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", v)
	}

	if lastResp.Choice[0].Delta == nil || lastResp.Choice[0].Delta.Content != "context canceled" {
		errContent := ""
		if lastResp.Choice[0].Delta != nil {
			errContent = lastResp.Choice[0].Delta.Content
		}
		t.Errorf("expected error context canceled, got %s", errContent)
	}

	if t.Failed() {
		fmt.Printf("%#v\n", lastResp)
	}
}

func Test_ConTest3(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions (requires more resources)")
	}

	// This test breaks out the channel loop before the context is canceled.
	// Then the context is cancelled and checks the system shuts down properly.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	var index int
	for range ch {
		index++
		if index == 2 {
			break
		}
	}

	t.Log("attempt to unload Knonk, should get error")

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()

	if err := krn.Unload(shortCtx); err == nil {
		t.Errorf("should receive an error unloading Kronk: %s", err)
	}

	t.Log("cancel context after breaking channel loop")
	cancel()

	t.Log("check if the channel is closed")
	var closed bool
	for range 3 {
		_, open := <-ch
		if !open {
			closed = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Log("check conditions")

	if !closed {
		t.Errorf("expected channel to be closed")
	}
}

// =============================================================================
// Context pool tests for embedding/rerank models with NSeqMax > 1

// Test_PooledEmbeddings verifies that NSeqMax creates an internal context pool
// for embedding models and that inputs are processed in parallel.
func Test_PooledEmbeddings(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping pool test in GitHub Actions (requires more resources)")
	}

	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:     mpEmbed.ModelFiles,
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

// Test_PooledRerank verifies that NSeqMax creates multiple model instances
// for rerank models and that concurrent requests execute in parallel.
func Test_PooledRerank(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping pool test in GitHub Actions (requires more resources)")
	}

	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:     mpRerank.ModelFiles,
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

// Test_PooledVision verifies that NSeqMax creates multiple model instances
// for vision models (ProjFile set) and that concurrent requests execute in parallel.
func Test_PooledVision(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping pool test in GitHub Actions (requires more resources)")
	}

	const numInstances = 2

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	krn, err := kronk.New(model.Config{
		ModelFiles:    mpSimpleVision.ModelFiles,
		ProjFile:      mpSimpleVision.ProjFile,
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

			resp, err := krn.Chat(ctx, dMedia)
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
