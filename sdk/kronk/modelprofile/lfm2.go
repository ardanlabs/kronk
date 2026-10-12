package modelprofile

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

type lfm2Adapter struct{}

func (lfm2Adapter) Name() string { return "lfm2" }

func (lfm2Adapter) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "lfm2")
}

func (lfm2Adapter) Apply(metadata metadata, profile *Profile) error {
	profile.ComputeKind = ComputeLFM2
	profile.MemorySemantics = MemoryRecurrent
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
	a := &profile.Attention
	a.RecurrentPattern = make([]bool, blocks)
	a.HeadCountKV = make([]int64, blocks)
	a.RecurrentLayers = 0
	var maxHeads int64
	for layer := range blocks {
		headsForLayer := heads[0]
		if len(heads) > 1 {
			headsForLayer = heads[layer]
		}
		a.HeadCountKV[layer] = headsForLayer
		a.RecurrentPattern[layer] = headsForLayer == 0
		if a.RecurrentPattern[layer] {
			a.RecurrentLayers++
		}
		maxHeads = max(maxHeads, headsForLayer)
	}
	a.FullAttentionLayers = blocks - a.RecurrentLayers
	a.FullHeadCountKV = maxHeads
	profile.Dimensions.HeadCountKV = maxHeads
	if a.RecurrentLayers > 0 {
		window, err := gguf.ParseInt64(metadata.values, profile.Architecture+".shortconv.l_cache")
		if err != nil {
			return fmt.Errorf("reading short-convolution window: %w", err)
		}
		if window <= 1 || profile.Dimensions.EmbeddingLength <= 0 {
			return fmt.Errorf("invalid short-convolution window or embedding length")
		}
		// b11513 stores only B×X history, not all three projected B/C/X
		// channels. Each layer keeps window−1 F32 values per hidden unit.
		a.RecurrentStateBytes = 4 * profile.Dimensions.EmbeddingLength * (window - 1)
	}
	return nil
}
