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

// Test_BatchEmbeddingsConcurrentParallel verifies that embeddings handles
// multiple concurrent goroutine calls correctly, similar to Test_BatchChatConcurrent.
//
// Run with: go test -v -run Test_BatchEmbeddingsConcurrentParallel
func Test_BatchEmbeddingsConcurrentParallel(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping batch test in GitHub Actions (requires more resources)")
	}

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
	})
	if err != nil {
		t.Fatalf("Failed to create embedding model: %v", err)
	}
	defer krn.Unload(ctx)

	g := 4

	t.Logf("Testing batch embeddings with %d concurrent requests", g)

	var wg sync.WaitGroup
	wg.Add(g)

	startBarrier := make(chan struct{})

	results := make([]struct {
		id         int
		duration   time.Duration
		err        error
		embeddings [][]float32
	}, g)

	inputs := []string{
		"The quick brown fox jumps over the lazy dog",
		"Machine learning is a subset of artificial intelligence",
		"Go is a statically typed programming language",
		"Embeddings convert text into numerical vectors",
	}

	for i := range g {
		go func(idx int) {
			defer wg.Done()

			<-startBarrier

			start := time.Now()

			resp, err := krn.Embeddings(ctx, model.D{
				"input": inputs,
			})
			if err != nil {
				results[idx].err = fmt.Errorf("goroutine %d: embeddings error: %w", idx, err)
				return
			}

			results[idx].duration = time.Since(start)
			results[idx].id = idx

			if len(resp.Data) != len(inputs) {
				results[idx].err = fmt.Errorf("goroutine %d: expected %d embeddings, got %d", idx, len(inputs), len(resp.Data))
				return
			}

			results[idx].embeddings = make([][]float32, len(resp.Data))
			for i, data := range resp.Data {
				results[idx].embeddings[i] = data.Embedding
			}
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
		t.Logf("Request %d completed in %s (embeddings=%d, dims=%d)", r.id, r.duration, len(r.embeddings), len(r.embeddings[0]))

		for i, emb := range r.embeddings {
			if len(emb) == 0 {
				errors = append(errors, fmt.Errorf("request %d embedding %d: empty", r.id, i))
				continue
			}

			// Check normalization.
			var sum float64
			for _, v := range emb {
				sum += float64(v * v)
			}

			if sum < 0.99 || sum > 1.01 {
				errors = append(errors, fmt.Errorf("request %d embedding %d: not normalized (L2=%f)", r.id, i, sum))
			}
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
}
