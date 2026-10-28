package arch

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// LFM3 normalizes short-convolution, full, and sliding-window attention layers.
type LFM3 struct{}

// Name returns the adapter's diagnostic name.
func (LFM3) Name() string { return "lfm3" }

// Claims reports whether the architecture uses this adapter.
func (LFM3) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "lfm3")
}

// Apply normalizes the b11960 LFM3 topology without borrowing LFM2's calibrated
// compute policy. Non-causal decision graphs retain their topology facts, but
// do not retain either attention or convolution history between evaluations.
func (LFM3) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	profile.MemorySemantics = modelprofile.MemoryRecurrent
	profile.TiedOutputFallback = true
	blocks := profile.Dimensions.BlockCount
	if blocks <= 0 {
		return nil
	}
	heads, err := gguf.ParseInt64OrArray(metadata.values, profile.Architecture+".attention.head_count_kv")
	if err != nil {
		return fmt.Errorf("reading per-layer KV heads: %w", err)
	}
	if len(heads) != 1 && int64(len(heads)) != blocks {
		return fmt.Errorf("KV head array length %d does not match block count %d", len(heads), blocks)
	}
	// The loader requires a per-layer array, not an inferred periodic pattern.
	pattern := profile.Attention.SWAPattern
	fields := strings.FieldsFunc(strings.Trim(metadata.value(profile.Architecture+".attention.sliding_window_pattern"), "[] \t\n"), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	if int64(len(pattern)) != blocks || len(fields) != len(pattern) {
		return fmt.Errorf("SWA pattern length %d does not match block count %d", len(pattern), blocks)
	}
	if profile.Dimensions.KeyLength != profile.Dimensions.ValueLength {
		return fmt.Errorf("LFM3 requires equal key and value lengths")
	}
	a := &profile.Attention
	a.HeadCountKV = make([]int64, blocks)
	a.RecurrentPattern = make([]bool, blocks)
	a.RecurrentLayers, a.FullAttentionLayers, a.SlidingWindowLayers = 0, 0, 0
	a.FullHeadCountKV, a.SWAHeadCountKV = 0, 0
	var maxHeads int64
	for layer := range blocks {
		h := heads[0]
		if len(heads) > 1 {
			h = heads[layer]
		}
		if h < 0 {
			return fmt.Errorf("negative KV head count at layer %d", layer)
		}
		a.HeadCountKV[layer] = h
		a.RecurrentPattern[layer] = h == 0
		maxHeads = max(maxHeads, h)
		switch {
		case h == 0:
			a.RecurrentLayers++
		case pattern[layer]:
			a.SlidingWindowLayers++
			a.SWAHeadCountKV = max(a.SWAHeadCountKV, h)
		default:
			a.FullAttentionLayers++
			a.FullHeadCountKV = max(a.FullHeadCountKV, h)
		}
	}
	profile.Dimensions.HeadCountKV = maxHeads
	if a.SlidingWindowLayers == 0 || a.SlidingWindow <= 0 {
		return fmt.Errorf("LFM3 requires sliding-window attention layers and a positive window")
	}
	window, err := gguf.ParseInt64(metadata.values, profile.Architecture+".shortconv.l_cache")
	if err != nil || window <= 1 {
		return fmt.Errorf("invalid or missing short-convolution window")
	}
	if a.RecurrentLayers > 0 {
		if profile.Dimensions.EmbeddingLength <= 0 {
			return fmt.Errorf("invalid short-convolution embedding length")
		}
		// llama_hparams::n_embd_r stores only B×X history, in F32.
		a.RecurrentStateBytes = 4 * profile.Dimensions.EmbeddingLength * (window - 1)
	}
	return nil
}
