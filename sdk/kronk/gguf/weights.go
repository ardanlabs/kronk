package gguf

import (
	"regexp"
	"strconv"
	"strings"
)

// WeightBreakdown provides per-category weight size information.
type WeightBreakdown struct {
	TotalBytes         int64
	AlwaysActiveBytes  int64
	ExpertBytesTotal   int64
	ExpertBytesByLayer []int64
}

// Broad pattern for VRAM accounting: catches all routed expert tensors.
// Covers standard (_exps) and channel (_chexps) expert variants as defined
// in llama.cpp and yzma's MoEExpertTensorPattern. The blk.<n>. prefix is
// required for per-layer attribution in ExpertBytesByLayer. The suffix
// boundary (\. or end) prevents accidental matches inside longer names.
var expertTensorPattern = regexp.MustCompile(`blk\.\d+\.ffn_(up|down|gate|gate_up|norm)_(ch)?exps(\.|$)`)

// blockIndexPattern extracts the block index from a tensor name.
var blockIndexPattern = regexp.MustCompile(`^blk\.(\d+)\.`)

// CategorizeWeights builds a WeightBreakdown from parsed tensor info.
// blockCount is the number of transformer layers from metadata.
func CategorizeWeights(tensors []TensorInfo, blockCount int64) WeightBreakdown {
	wb := WeightBreakdown{
		ExpertBytesByLayer: make([]int64, blockCount),
	}

	for _, t := range tensors {
		if expertTensorPattern.MatchString(t.Name) {
			wb.ExpertBytesTotal += t.Bytes

			if m := blockIndexPattern.FindStringSubmatch(t.Name); len(m) == 2 {
				idx, err := strconv.ParseInt(m[1], 10, 64)
				if err == nil && idx >= 0 && idx < blockCount {
					wb.ExpertBytesByLayer[idx] += t.Bytes
				}
			}

			continue
		}

		wb.AlwaysActiveBytes += t.Bytes
	}

	wb.TotalBytes = wb.AlwaysActiveBytes + wb.ExpertBytesTotal

	return wb
}

// DetectMoEFromTensors returns true if tensor names contain expert
// patterns, providing a fallback when GGUF metadata keys are missing.
func DetectMoEFromTensors(tensors []TensorInfo) bool {
	for _, t := range tensors {
		if expertTensorPattern.MatchString(t.Name) {
			return true
		}
	}

	return false
}

// DetectSharedExpertsFromTensors checks for shared expert tensors in names.
func DetectSharedExpertsFromTensors(tensors []TensorInfo) bool {
	for _, t := range tensors {
		lower := strings.ToLower(t.Name)
		if strings.Contains(lower, "shared") || strings.Contains(lower, "shexp") {
			return true
		}
	}

	return false
}
