package arch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// DeepSeek41 normalizes compressed-cache topology and indexed-attention dimensions.
type DeepSeek41 struct{}

// Name returns the adapter's diagnostic name.
func (DeepSeek41) Name() string { return "deepseek41" }

// Claims reports whether the architecture uses this adapter.
func (DeepSeek41) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "deepseek41")
}

// Apply normalizes DeepSeek V4.1 dimensions and compressed-cache sources.
func (DeepSeek41) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	// b11901's V4.1 graph has no embedded MTP. DSpark is a separate GGUF.
	profile.Speculation = modelprofile.Speculation{}
	profile.ComputeKind = modelprofile.ComputeDeepSeek41
	profile.Dimensions.OutputEmbeddingLength = profile.Dimensions.EmbeddingLength
	arch := profile.Architecture
	blocks := profile.Dimensions.BlockCount
	if blocks <= 0 {
		return fmt.Errorf("invalid block_count")
	}
	a := &profile.Attention
	a.CandidateSourceLayer = -1
	for suffix, dest := range map[string]*int64{
		"attention.indexer.head_count": &a.IndexerHeads,
		"attention.indexer.key_length": &a.IndexerKeyLength,
		"attention.indexer.top_k":      &a.IndexerTopK,
		"attention.q_lora_rank":        &a.QueryRank,
		"attention.output_lora_rank":   &a.OutputRank,
		"attention.output_group_count": &a.OutputGroups,
		"hyper_connection.count":       &a.HyperConnections,
	} {
		value, err := gguf.ParseInt64(metadata.values, arch+"."+suffix)
		if err != nil || value <= 0 {
			return fmt.Errorf("invalid or missing %s", suffix)
		}
		*dest = value
	}
	if a.SlidingWindow <= 0 || profile.Dimensions.HeadCountKV != 1 {
		return fmt.Errorf("invalid sliding window or KV head count")
	}
	ratios, err := gguf.ParseInt64OrArray(metadata.values, arch+".attention.compress_ratios")
	if err != nil || int64(len(ratios)) < blocks {
		return fmt.Errorf("compress_ratios is shorter than block_count")
	}
	a.CompressRatios = ratios[:blocks]
	kvSources, err := gguf.ParseInt64OrArray(metadata.values, arch+".attention.kv_source_layers")
	if err != nil {
		return err
	}
	indexSources, err := gguf.ParseInt64OrArray(metadata.values, arch+".attention.indexer.source_layers")
	if err != nil {
		return err
	}
	for _, sources := range [][]int64{kvSources, indexSources} {
		for _, source := range sources {
			if source < 0 || source >= blocks || ratios[source] == 0 {
				return fmt.Errorf("invalid compressed source layer %d", source)
			}
		}
	}
	lastKV, lastIndex := int64(-1), int64(-1)
	for layer, ratio := range a.CompressRatios {
		il := int64(layer)
		if slices.Contains(kvSources, il) {
			lastKV = il
		}
		if slices.Contains(indexSources, il) {
			lastIndex = il
		}
		if ratio == 0 {
			a.KVSources = append(a.KVSources, -1)
			a.IndexerSources = append(a.IndexerSources, -1)
			continue
		}
		if ratio < 1 || ratio > 2 || lastKV < 0 || lastIndex < lastKV ||
			ratios[lastKV] != ratio || ratios[lastIndex] != ratio {
			return fmt.Errorf("layer %d has no matching compressed KV/index source", layer)
		}
		a.KVSources = append(a.KVSources, lastKV)
		a.IndexerSources = append(a.IndexerSources, lastIndex)
	}
	if value := metadata.value(arch + ".attention.indexer.candidate_source_layer"); value != "" {
		a.CandidateSourceLayer, err = gguf.ParseInt64(metadata.values, arch+".attention.indexer.candidate_source_layer")
		if err != nil || a.CandidateSourceLayer < -1 || a.CandidateSourceLayer >= blocks {
			return fmt.Errorf("invalid candidate source layer")
		}
	}
	if source := a.CandidateSourceLayer; source >= 0 {
		a.CandidateTopKBlocks = parseInt64(metadata, arch+".attention.indexer.candidate_top_k_blocks")
		a.CandidateBlockSize = parseInt64(metadata, arch+".attention.indexer.candidate_block_size")
		if a.IndexerSources[source] != source || a.CandidateTopKBlocks <= 0 || a.CandidateBlockSize <= 0 {
			return fmt.Errorf("invalid candidate selection")
		}
		for layer := source + 1; layer < blocks; layer++ {
			if a.IndexerSources[layer] == layer && ratios[layer] != ratios[source] {
				return fmt.Errorf("candidate consumer %d has a different compression ratio", layer)
			}
		}
	}
	if metadata.value(arch+".ple.layers") != "" {
		layers, err := gguf.ParseInt64OrArray(metadata.values, arch+".ple.layers")
		if err != nil {
			return err
		}
		for i, layer := range layers {
			if layer < 0 || layer >= blocks || (i > 0 && layer <= layers[i-1]) {
				return fmt.Errorf("invalid Engram layer %d", layer)
			}
		}
		ngram := parseInt64(metadata, arch+".ple.ngram_size")
		heads := parseInt64(metadata, arch+".ple.heads_per_ngram")
		a.LayerEmbeddingLength = parseInt64(metadata, arch+".embedding_length_per_layer_input")
		if ngram < 2 || heads <= 0 || a.LayerEmbeddingLength <= 0 {
			return fmt.Errorf("invalid Engram dimensions")
		}
		a.LayerEmbeddingRows = int64(len(layers)) * (ngram - 1) * heads
		profile.LazyRowsPerToken = a.LayerEmbeddingRows
	}
	profile.MoE.HasSharedExperts = parseInt64(metadata, arch+".expert_shared_count") > 0
	return nil
}
