package arch

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// Inkling normalizes attention and short-convolution state, including the
// independent cache slots that share physical weights in loop models.
type Inkling struct{}

// Name returns the adapter's diagnostic name.
func (Inkling) Name() string { return "inkling" }

// Claims reports whether the architecture belongs to the Inkling2 family.
func (Inkling) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "inkling2") || strings.EqualFold(architecture, "inkling2_loop")
}

// Apply preserves the physical block count for weight accounting and expands
// the per-layer attention arrays to the native cache-slot order. Every slot
// retains both K/V and convolution history; none is recurrent-only.
func (Inkling) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	profile.RequiresFileMetadata = true
	profile.MemorySemantics = modelprofile.MemoryRecurrent
	profile.ComputeKind = modelprofile.ComputeInkling
	blocks := profile.Dimensions.BlockCount
	if blocks <= 0 || blocks > 512 {
		return fmt.Errorf("invalid or missing block count")
	}
	prefix := profile.Architecture
	heads, err := inklingHeads(metadata, prefix+".attention.head_count", blocks)
	if err != nil {
		return err
	}
	kvHeads := heads
	if metadata.value(prefix+".attention.head_count_kv") != "" {
		kvHeads, err = inklingHeads(metadata, prefix+".attention.head_count_kv", blocks)
		if err != nil {
			return err
		}
	}
	for layer, count := range heads {
		if count%kvHeads[layer] != 0 {
			return fmt.Errorf("query head count is not divisible by KV head count at layer %d", layer)
		}
	}
	d := &profile.Dimensions
	d.HeadCount, d.HeadCountKV = slices.Max(heads), slices.Max(kvHeads)
	if d.EmbeddingLength <= 0 || d.KeyLength <= 0 || d.KeyLength != d.ValueLength {
		return fmt.Errorf("invalid embedding or key/value dimensions")
	}
	a := &profile.Attention
	if a.SWAKeyLength != a.SWAValueLength || a.SWAKeyLength <= 0 || a.SlidingWindow <= 0 {
		return fmt.Errorf("invalid sliding-window dimensions")
	}
	fields := strings.FieldsFunc(strings.Trim(metadata.value(prefix+".attention.sliding_window_pattern"), "[] \t\n"), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	if int64(len(fields)) != blocks {
		return fmt.Errorf("SWA pattern length %d does not match block count %d", len(fields), blocks)
	}
	pattern := make([]bool, blocks)
	for layer, field := range fields {
		if field != "true" && field != "false" {
			return fmt.Errorf("invalid SWA pattern element %q", field)
		}
		pattern[layer] = field == "true"
	}
	window := parseInt64(metadata, prefix+".shortconv.l_cache")
	if window < 2 || window > 64 {
		return fmt.Errorf("invalid or missing short-convolution window")
	}
	var maxChannels int64
	for layer, count := range kvHeads {
		keyLength := d.KeyLength
		if pattern[layer] {
			keyLength = a.SWAKeyLength
		}
		maxChannels = max(maxChannels, 2*count*keyLength+2*d.EmbeddingLength)
	}
	a.RecurrentStateBytes = 4 * maxChannels * (window - 1)

	if strings.EqualFold(prefix, "inkling2_loop") {
		start := parseInt64(metadata, prefix+".loop.start")
		end := parseInt64(metadata, prefix+".loop.end")
		passes := parseInt64(metadata, prefix+".loop.count")
		blockSize := parseInt64(metadata, prefix+".attn_res.block_size")
		gateRank := parseInt64(metadata, prefix+".loop.gate_rank")
		varying := false
		if value := metadata.value(prefix + ".loop.varying"); value != "" {
			varying, err = strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("reading variable loop flag: %w", err)
			}
		}
		minPasses := int64(2)
		if varying {
			minPasses = 1
		}
		if start <= 0 || start >= end || end > blocks || blockSize <= 0 || start%blockSize != 0 || end%blockSize != 0 ||
			passes < minPasses || gateRank <= 0 || profile.Speculation.NextNPredictLayers > 0 {
			return fmt.Errorf("invalid loop core, passes, gate rank or NextN layers")
		}
		if passes > 1+(512-blocks)/(end-start) {
			return fmt.Errorf("expanded loop exceeds the native limit of 512 cache slots")
		}
		// The native loader appends the extra passes after the physical layers.
		for pass := int64(1); pass < passes; pass++ {
			kvHeads = append(kvHeads, kvHeads[start:end]...)
			pattern = append(pattern, pattern[start:end]...)
		}
	}
	a.HeadCountKV, a.SWAPattern = kvHeads, pattern
	a.RecurrentPattern = nil
	a.RecurrentLayers = int64(len(kvHeads))
	a.FullAttentionLayers, a.SlidingWindowLayers = 0, 0
	a.FullHeadCountKV, a.SWAHeadCountKV = 0, 0
	for layer, swa := range pattern {
		if swa {
			a.SlidingWindowLayers++
			a.SWAHeadCountKV = max(a.SWAHeadCountKV, kvHeads[layer])
			continue
		}
		a.FullAttentionLayers++
		a.FullHeadCountKV = max(a.FullHeadCountKV, kvHeads[layer])
	}
	return nil
}

func inklingHeads(metadata Metadata, key string, blocks int64) ([]int64, error) {
	values, err := gguf.ParseInt64OrArray(metadata.values, key)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", key, err)
	}
	if len(values) != 1 && int64(len(values)) != blocks {
		return nil, fmt.Errorf("%s array length %d does not match block count %d", key, len(values), blocks)
	}
	heads := make([]int64, blocks)
	for layer := range heads {
		count := values[0]
		if len(values) > 1 {
			count = values[layer]
		}
		if count <= 0 {
			return nil, fmt.Errorf("non-positive %s at layer %d", key, layer)
		}
		heads[layer] = count
	}
	return heads, nil
}
