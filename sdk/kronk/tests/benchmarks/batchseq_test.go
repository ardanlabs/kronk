package benchmarks_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func cfgSequenceModel(mp models.Path, nSeqMax int) model.Config {
	return model.Config{
		Log:                 benchLog,
		ModelFiles:          mp.ModelFiles,
		PtrContextWindow:    new(2048),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(nSeqMax),
	}
}

func BenchmarkRerank_Qwen3_ContextPoolFallback(b *testing.B) {
	if len(benchRerankFallback.ModelFiles) == 0 {
		b.Skip("model qwen3-reranker-0.6b-q8_0 not downloaded")
	}
	benchmarkRerank(b, withBenchModel(b, cfgSequenceModel(benchRerankFallback, 4)))
}

func BenchmarkRerank_BGE_BatchSeq(b *testing.B) {
	if len(benchRerankBatchSeq.ModelFiles) == 0 {
		b.Skip("model bge-reranker-v2-m3-Q8_0 not downloaded")
	}
	benchmarkRerank(b, withBenchModel(b, cfgSequenceModel(benchRerankBatchSeq, 4)))
}

func benchmarkRerank(b *testing.B, krn *kronk.Kronk) {
	doc := model.D{
		"query": "What is the capital of France?",
		"documents": []string{
			"Paris is the capital of France.",
			"Berlin is the capital of Germany.",
			"The Eiffel Tower is located in Paris.",
			"France is a country in Western Europe.",
		},
	}
	if _, err := krn.Rerank(context.Background(), doc); err != nil {
		b.Fatalf("rerank warm-up: %v", err)
	}

	var requests atomic.Int64
	var promptTokens atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := krn.Rerank(context.Background(), doc)
			if err != nil {
				b.Errorf("rerank: %v", err)
				return
			}
			requests.Add(1)
			promptTokens.Add(int64(resp.Usage.PromptTokens))
		}
	})
	b.StopTimer()
	reportWorkload(b, requests.Load(), promptTokens.Load(), 4)
}

func reportWorkload(b *testing.B, requests, promptTokens int64, inputs int) {
	if requests > 0 {
		b.ReportMetric(float64(promptTokens)/float64(requests), "prompt-tok/op")
	}
	b.ReportMetric(float64(inputs), "inputs/op")
}
