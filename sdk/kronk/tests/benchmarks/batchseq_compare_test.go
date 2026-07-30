//go:build kronk_benchmark

package benchmarks_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const embeddingBenchmarkNSeqMax = 4

func BenchmarkEmbedding_Qwen3_ContextPoolFallback(b *testing.B) {
	if len(benchEmbedBatchSeq.ModelFiles) == 0 {
		b.Skip("model Qwen3-Embedding-0.6B-Q8_0 not downloaded")
	}

	b.Setenv("KRONK_BENCHMARK_DISABLE_BATCHSEQ", "true")
	cfg := cfgSequenceModel(benchEmbedBatchSeq, embeddingBenchmarkNSeqMax)
	benchmarkEmbeddings(b, withBenchModel(b, cfg), embeddingBenchmarkNSeqMax)
}

func benchmarkEmbeddings(b *testing.B, krn *kronk.Kronk, concurrency int) {
	doc := model.D{"input": []string{
		"Paris is the capital of France.",
		"Berlin is the capital of Germany.",
		"The Eiffel Tower is located in Paris.",
		"France is a country in Western Europe.",
	}}
	warmEmbeddings(b, krn, doc, concurrency)

	var requests atomic.Int64
	var promptTokens atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := krn.Embeddings(context.Background(), doc)
			if err != nil {
				b.Errorf("embeddings: %v", err)
				return
			}
			requests.Add(1)
			promptTokens.Add(int64(resp.Usage.PromptTokens))
		}
	})
	b.StopTimer()
	reportWorkload(b, requests.Load(), promptTokens.Load(), 4)
}

func warmEmbeddings(b *testing.B, krn *kronk.Kronk, doc model.D, concurrency int) {
	b.Helper()

	ready := make(chan struct{}, concurrency)
	start := make(chan struct{})
	errs := make(chan error, concurrency)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			_, err := krn.Embeddings(context.Background(), doc)
			errs <- err
		}()
	}

	for range concurrency {
		<-ready
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			b.Fatalf("embedding warm-up: %v", err)
		}
	}
}

func BenchmarkEmbedding_Qwen3_BatchSeq(b *testing.B) {
	if len(benchEmbedBatchSeq.ModelFiles) == 0 {
		b.Skip("model Qwen3-Embedding-0.6B-Q8_0 not downloaded")
	}

	b.Setenv("KRONK_BENCHMARK_DISABLE_BATCHSEQ", "false")
	cfg := cfgSequenceModel(benchEmbedBatchSeq, embeddingBenchmarkNSeqMax)
	benchmarkEmbeddings(b, withBenchModel(b, cfg), embeddingBenchmarkNSeqMax)
}
