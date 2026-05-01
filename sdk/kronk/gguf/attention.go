package gguf

import "strings"

// AttentionFacts contains attention-specific metadata extracted from
// GGUF metadata.
type AttentionFacts struct {
	SlidingWindow       int64
	SlidingWindowLayers int64
	FullAttentionLayers int64
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

	// Count SWA layers from the sliding_window_pattern array if present.
	if pattern, ok := metadata[arch+".attention.sliding_window_pattern"]; ok {
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

	if v, err := ParseFloat64(metadata, arch+".final_logit_softcapping"); err == nil {
		a.LogitSoftcapping = v
	}

	return a
}

// CountSWALayers counts true values in a stringified bool array like
// "[true true true true true false true ...]".
func CountSWALayers(pattern string) int64 {
	trimmed := strings.TrimSpace(pattern)
	trimmed = strings.Trim(trimmed, "[]")
	fields := strings.Fields(trimmed)

	var count int64
	for _, f := range fields {
		if f == "true" {
			count++
		}
	}

	return count
}
