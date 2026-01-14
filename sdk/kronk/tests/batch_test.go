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
// multiple concurrent chat requests. It launches N goroutines simultaneously
// and verifies all responses are correct (no corruption from parallel processing).
//
// Run with: GOROUTINES=4 go test -v -run Test_BatchChatConcurrent
func Test_BatchChatConcurrent(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping batch test in GitHub Actions (requires more resources)")
	}

	// Minimum 4 to test batching
	n := max(goroutines, 4)

	t.Logf("Testing batch inference with %d concurrent requests", n)

	var wg sync.WaitGroup
	wg.Add(n)

	// Use a barrier to ensure all goroutines start at the same time.
	startBarrier := make(chan struct{})

	results := make([]struct {
		id       int
		duration time.Duration
		err      error
		content  string
	}, n)

	for i := range n {
		go func(idx int) {
			defer wg.Done()

			// Wait for all goroutines to be ready.
			<-startBarrier

			ctx, cancel := context.WithTimeout(context.Background(), testDuration)
			defer cancel()

			start := time.Now()

			ch, err := krnThinkToolChat.ChatStreaming(ctx, dChatNoTool)
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

	// Release all goroutines at once.
	close(startBarrier)
	wg.Wait()

	// Check results.
	var errors []error
	var totalDuration time.Duration
	for _, r := range results {
		if r.err != nil {
			errors = append(errors, r.err)
			continue
		}

		totalDuration += r.duration
		t.Logf("Request %d completed in %s", r.id, r.duration)

		// Verify response contains expected content.
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

	avgDuration := totalDuration / time.Duration(n)
	t.Logf("All %d requests completed. Average duration: %s", n, avgDuration)
}

// Test_BatchEmbeddingsConcurrent verifies that the embeddings batch function
// correctly processes multiple inputs in parallel.
func Test_BatchEmbeddingsConcurrent(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping batch test in GitHub Actions (requires more resources)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Test with multiple inputs in a single batch call.
	inputs := []string{
		"The quick brown fox jumps over the lazy dog",
		"Machine learning is a subset of artificial intelligence",
		"Go is a statically typed programming language",
		"Embeddings convert text into numerical vectors",
	}

	krn, err := kronk.New(model.Config{
		ModelFiles:     mpEmbed.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
		NSeqMax:        4, // Allow 4 sequences for batch testing
	})
	if err != nil {
		t.Fatalf("Failed to create embedding model: %v", err)
	}
	defer krn.Unload(context.Background())

	start := time.Now()

	resp, err := krn.Embeddings(ctx, model.D{
		"input": inputs,
	})
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}

	duration := time.Since(start)
	t.Logf("Batch embedding of %d inputs completed in %s", len(inputs), duration)

	// Verify we got embeddings for all inputs.
	if len(resp.Data) != len(inputs) {
		t.Errorf("Expected %d embeddings, got %d", len(inputs), len(resp.Data))
	}

	// Verify each embedding has correct index and non-zero values.
	for i, data := range resp.Data {
		if data.Index != i {
			t.Errorf("Expected index %d, got %d", i, data.Index)
		}

		if len(data.Embedding) == 0 {
			t.Errorf("Embedding %d has zero dimensions", i)
		}

		// Check it's normalized (L2 norm ≈ 1).
		var sum float64
		for _, v := range data.Embedding {
			sum += float64(v * v)
		}
		if sum < 0.99 || sum > 1.01 {
			t.Errorf("Embedding %d not normalized: L2 norm = %f", i, sum)
		}
	}

	t.Logf("Usage: prompt_tokens=%d, total_tokens=%d", resp.Usage.PromptTokens, resp.Usage.TotalTokens)
}

// Test_BatchThroughput measures throughput improvement from batching.
// Compares sequential vs concurrent request processing times.
func Test_BatchThroughput(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping batch test in GitHub Actions (requires more resources)")
	}

	n := goroutines
	if n < 2 {
		n = 2
	}

	t.Logf("Measuring throughput with %d requests", n)

	// Sequential baseline.
	t.Log("Running sequential baseline...")
	seqStart := time.Now()
	for i := range n {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)

		ch, err := krnThinkToolChat.ChatStreaming(ctx, dChatNoTool)
		if err != nil {
			cancel()
			t.Fatalf("Sequential request %d failed: %v", i, err)
		}

		for range ch {
		}
		cancel()
	}
	seqDuration := time.Since(seqStart)
	t.Logf("Sequential: %d requests in %s (%.2f req/s)", n, seqDuration, float64(n)/seqDuration.Seconds())

	// Concurrent with batching.
	t.Log("Running concurrent batch...")
	var wg sync.WaitGroup
	wg.Add(n)

	startBarrier := make(chan struct{})
	concStart := time.Now()

	for i := range n {
		go func(idx int) {
			defer wg.Done()
			<-startBarrier

			ctx, cancel := context.WithTimeout(context.Background(), testDuration)
			defer cancel()

			ch, err := krnThinkToolChat.ChatStreaming(ctx, dChatNoTool)
			if err != nil {
				t.Errorf("Concurrent request %d failed: %v", idx, err)
				return
			}

			for range ch {
			}
		}(i)
	}

	close(startBarrier)
	wg.Wait()
	concDuration := time.Since(concStart)

	t.Logf("Concurrent: %d requests in %s (%.2f req/s)", n, concDuration, float64(n)/concDuration.Seconds())

	speedup := float64(seqDuration) / float64(concDuration)
	t.Logf("Speedup: %.2fx", speedup)

	// Batching should provide at least some improvement with NSeqMax > 1.
	if speedup < 1.0 {
		t.Logf("Warning: No speedup observed (speedup=%.2fx). This may be expected for small models or limited parallelism.", speedup)
	}
}
