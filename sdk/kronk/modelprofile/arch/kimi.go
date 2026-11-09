package arch

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// KimiK3 normalizes Kimi K3/K3.1's KDA, MLA, and optional DSA topology.
type KimiK3 struct{}

// Name returns the adapter's diagnostic name.
func (KimiK3) Name() string { return "kimi-k3" }

// Claims reports whether this is a Kimi K3 text architecture.
func (KimiK3) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "kimi-k3") || strings.EqualFold(architecture, "kimi-k31")
}

// Apply follows the b12223 loaders: zero KV heads identify KDA layers, while
// attention layers retain a compressed K-only MLA cache. DSA sources identify
// the layers that own indexer keys, not an extra cache on every consumer.
func (KimiK3) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	profile.RequiresFileMetadata = true
	profile.MemorySemantics = modelprofile.MemoryRecurrent
	profile.ComputeKind = modelprofile.ComputeKimiK3
	profile.Warnings = append(profile.Warnings, "Kimi persistent state follows the native cache layout, but compute workspace is uncalibrated; fit recommendations require validation on the deployment hardware")
	arch := profile.Architecture
	blocks := profile.Dimensions.BlockCount
	if blocks <= 0 {
		return fmt.Errorf("invalid block_count")
	}
	heads, err := gguf.ParseInt64OrArray(metadata.values, arch+".attention.head_count_kv")
	if err != nil || (len(heads) != 1 && int64(len(heads)) != blocks) {
		return fmt.Errorf("invalid or missing per-layer KV heads")
	}
	a := &profile.Attention
	a.HeadCountKV = make([]int64, blocks)
	a.RecurrentPattern = make([]bool, blocks)
	a.RecurrentLayers, a.FullAttentionLayers, a.SlidingWindowLayers = 0, 0, 0
	a.SlidingWindow = 0
	a.FullHeadCountKV = 0
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
		if h == 0 {
			a.RecurrentLayers++
			continue
		}
		a.FullAttentionLayers++
		a.FullHeadCountKV = max(a.FullHeadCountKV, h)
	}
	profile.Dimensions.HeadCountKV = a.FullHeadCountKV
	// Older accepted GGUFs omit value_length; the native loader uses the
	// latent rank, not embedding_length/head_count. V is a view, not storage.
	latent := parseInt64(metadata, arch+".attention.kv_lora_rank")
	if latent <= 0 {
		return fmt.Errorf("invalid or missing KV latent rank")
	}
	a.FullValueLength = latent
	profile.Dimensions.ValueLength = latent
	if a.RecurrentLayers > 0 {
		conv := parseInt64(metadata, arch+".ssm.conv_kernel")
		dim := parseInt64(metadata, arch+".kda.head_dim")
		n := profile.Dimensions.HeadCount
		if conv <= 0 || dim <= 0 || n <= 0 {
			return fmt.Errorf("invalid or missing KDA state dimensions")
		}
		a.RecurrentStateBytes = 4 * (3*(conv-1)*n*dim + dim*dim*n)
	}
	profile.MoE.HasSharedExperts = parseInt64(metadata, arch+".expert_shared_count") > 0
	if arch != "kimi-k31" {
		return nil
	}
	a.IndexerKeyLength = parseInt64(metadata, arch+".attention.indexer.key_length")
	a.IndexerHeads = parseInt64(metadata, arch+".attention.indexer.head_count")
	a.IndexerTopK, err = gguf.ParseInt64(metadata.values, arch+".attention.indexer.top_k")
	if err != nil {
		return fmt.Errorf("invalid or missing indexer top_k: %w", err)
	}
	a.QueryRank = parseInt64(metadata, arch+".attention.q_lora_rank")
	if a.IndexerKeyLength <= 0 || a.IndexerHeads <= 0 || a.IndexerTopK < 0 || (a.IndexerTopK > 0 && a.QueryRank <= 0) {
		return fmt.Errorf("invalid or missing DSA dimensions")
	}
	pattern := make([]bool, blocks)
	for layer := range pattern {
		pattern[layer] = true
	}
	if value := metadata.value(arch + ".attention.indexer.types"); value != "" {
		var ok bool
		pattern, ok = parseBoolPattern(value, blocks)
		if !ok {
			return fmt.Errorf("invalid indexer layer pattern")
		}
	}
	last := int64(-1)
	for layer := range blocks {
		if a.RecurrentPattern[layer] {
			a.IndexerSources = append(a.IndexerSources, -1)
			continue
		}
		if pattern[layer] {
			last = layer
		}
		if last < 0 && a.IndexerTopK > 0 {
			return fmt.Errorf("shared indexer layer %d has no source", layer)
		}
		a.IndexerSources = append(a.IndexerSources, last)
	}
	return nil
}
