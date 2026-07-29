//go:build kronk_benchmark

package model

import "os"

// useBatchSeq allows controlled benchmarks to compare supported models on the
// context-pool fallback without exposing a production runtime override.
func useBatchSeq(mi ModelInfo) bool {
	return os.Getenv("KRONK_BENCHMARK_DISABLE_BATCHSEQ") != "true" && supportsBatchSeq(mi)
}
