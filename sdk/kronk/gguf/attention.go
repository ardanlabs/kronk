package gguf

import (
	"strconv"
	"strings"
)

// AttentionFacts contains attention-specific metadata extracted from
// GGUF metadata.
type AttentionFacts struct {
	SlidingWindow       int64
	SlidingWindowLayers int64
	FullAttentionLayers int64
	RecurrentLayers     int64
	NextNPredictLayers  int64
	SharedKVLayers      int64
	SWAPattern          []bool
	RecurrentPattern    []bool
	HeadCountKV         []int64
	FullHeadCountKV     int64
	SWAHeadCountKV      int64
	FullKeyLength       int64
	FullValueLength     int64
	SWAKeyLength        int64
	SWAValueLength      int64
	RecurrentStateBytes int64
	LogitSoftcapping    float64
}

// ParseAttentionFacts extracts attention-specific metadata for the given
// architecture. blockCount is used to derive the full-attention layer
// count when the model carries a sliding window pattern.
func ParseAttentionFacts(metadata map[string]string, arch string, blockCount int64) AttentionFacts {
	var a AttentionFacts

	if v, err := ParseInt64(metadata, arch+".attention.sliding_window"); err == nil {
		a.SlidingWindow = v
	}

	pattern := metadata[arch+".attention.sliding_window_pattern"]
	if pattern != "" {
		a.SWAPattern = parseSWAPattern(pattern)
		swaCount := CountSWALayers(pattern)
		a.SlidingWindowLayers = swaCount
		a.FullAttentionLayers = blockCount - swaCount
	} else if a.SlidingWindow > 0 {
		// If there's a sliding window but no pattern, assume all layers are SWA.
		a.SlidingWindowLayers = blockCount
		a.FullAttentionLayers = 0
	} else {
		a.FullAttentionLayers = blockCount
	}
	a.SharedKVLayers, _ = ParseInt64(metadata, arch+".attention.shared_kv_layers")
	parseRecurrentFacts(metadata, arch, blockCount, &a)

	a.FullKeyLength, a.FullValueLength, _ = ResolveKVLengths(metadata, arch)
	a.SWAKeyLength, _ = ParseInt64(metadata, arch+".attention.key_length_swa")
	a.SWAValueLength, _ = ParseInt64(metadata, arch+".attention.value_length_swa")
	if a.SWAKeyLength == 0 {
		a.SWAKeyLength = a.FullKeyLength
	}
	if a.SWAValueLength == 0 {
		a.SWAValueLength = a.FullValueLength
	}

	a.FullHeadCountKV, a.SWAHeadCountKV = splitHeadCountKV(metadata, arch, pattern)
	a.HeadCountKV, _ = ParseInt64OrArray(metadata, arch+".attention.head_count_kv")

	if v, err := ParseFloat64(metadata, arch+".final_logit_softcapping"); err == nil {
		a.LogitSoftcapping = v
	}

	return a
}

func parseRecurrentFacts(metadata map[string]string, arch string, blockCount int64, a *AttentionFacts) {
	if arch != "qwen35" && arch != "qwen35moe" {
		return
	}

	a.NextNPredictLayers, _ = ParseInt64(metadata, arch+".nextn_predict_layers")
	trunkLayers := max(blockCount-max(a.NextNPredictLayers, 0), 0)

	pattern, ok := parseBoolPattern(metadata[arch+".attention.recurrent_layers"], blockCount)
	if !ok {
		interval := int64(4)
		if configured, err := ParseInt64(metadata, arch+".full_attention_interval"); err == nil && configured > 0 {
			interval = configured
		}

		pattern = make([]bool, blockCount)
		for layer := range trunkLayers {
			pattern[layer] = (layer+1)%interval != 0
		}
	}

	// Appended NextN/MTP blocks are not part of the target context's
	// attention or recurrent memory.
	for layer := trunkLayers; layer < blockCount; layer++ {
		pattern[layer] = false
	}

	a.RecurrentPattern = pattern
	for layer := range trunkLayers {
		if pattern[layer] {
			a.RecurrentLayers++
		}
	}
	a.FullAttentionLayers = trunkLayers - a.RecurrentLayers - min(a.SlidingWindowLayers, trunkLayers)

	convKernel, convErr := ParseInt64(metadata, arch+".ssm.conv_kernel")
	innerSize, innerErr := ParseInt64(metadata, arch+".ssm.inner_size")
	stateSize, stateErr := ParseInt64(metadata, arch+".ssm.state_size")
	groupCount, groupErr := ParseInt64(metadata, arch+".ssm.group_count")
	if convErr != nil || innerErr != nil || stateErr != nil || groupErr != nil ||
		convKernel <= 0 || innerSize <= 0 || stateSize <= 0 || groupCount <= 0 {
		return
	}

	rWidth := (convKernel - 1) * (innerSize + 2*groupCount*stateSize)
	sWidth := stateSize * innerSize
	a.RecurrentStateBytes = 4 * (rWidth + sWidth)
}

func parseBoolPattern(value string, count int64) ([]bool, bool) {
	if value == "" {
		return nil, false
	}

	if scalar, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
		pattern := make([]bool, count)
		for i := range pattern {
			pattern[i] = scalar
		}
		return pattern, true
	}

	pattern := parseSWAPattern(value)
	return pattern, int64(len(pattern)) == count
}

func splitHeadCountKV(metadata map[string]string, arch string, pattern string) (int64, int64) {
	heads, err := ParseInt64OrArray(metadata, arch+".attention.head_count_kv")
	if err != nil {
		return 0, 0
	}
	if len(heads) == 1 {
		return heads[0], heads[0]
	}

	layers := parseSWAPattern(pattern)
	if len(layers) != len(heads) {
		avg := average(heads)
		return avg, avg
	}

	var full, swa []int64
	for i, isSWA := range layers {
		if isSWA {
			swa = append(swa, heads[i])
		} else {
			full = append(full, heads[i])
		}
	}

	return average(full), average(swa)
}

func average(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var sum int64
	for _, value := range values {
		sum += value
	}
	return sum / int64(len(values))
}

func parseSWAPattern(pattern string) []bool {
	trimmed := strings.Trim(strings.TrimSpace(pattern), "[]")
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	layers := make([]bool, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "true":
			layers = append(layers, true)
		case "false":
			layers = append(layers, false)
		}
	}
	return layers
}

// CountSWALayers counts true values in a stringified bool array like
// "[true true true true true false true ...]".
func CountSWALayers(pattern string) int64 {
	var count int64
	for _, isSWA := range parseSWAPattern(pattern) {
		if isSWA {
			count++
		}
	}

	return count
}
