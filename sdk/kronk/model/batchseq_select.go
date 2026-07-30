//go:build !kronk_benchmark

package model

func useBatchSeq(mi ModelInfo) bool {
	return supportsBatchSeq(mi)
}
