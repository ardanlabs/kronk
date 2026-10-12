package modelprofile

import (
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

type qwenHybridAdapter struct{}

func (qwenHybridAdapter) Name() string { return "qwen-hybrid" }

func (qwenHybridAdapter) Claims(architecture string) bool {
	switch strings.ToLower(architecture) {
	case "qwen35", "qwen35moe", "qwen4exp":
		return true
	default:
		return false
	}
}

func (qwenHybridAdapter) Apply(metadata metadata, profile *Profile) error {
	profile.MemorySemantics = MemoryRecurrent

	arch := profile.Architecture
	if arch == "qwen35" || arch == "qwen35moe" {
		profile.ComputeKind = ComputeQwen35
	}
	blockCount := profile.Dimensions.BlockCount
	if blockCount <= 0 {
		return nil
	}
	a := &profile.Attention
	if arch == "qwen4exp" {
		pleLayers, _ := gguf.ParseInt64OrArray(metadata.values, arch+".ple.layers")
		ngramSize := parseInt64(metadata, arch+".ple.ngram_size")
		heads := parseInt64(metadata, arch+".ple.heads_per_ngram")
		if len(pleLayers) == 1 && ngramSize > 1 && heads > 0 && parseInt64(metadata, arch+".embedding_length_per_layer_input") > 0 {
			profile.LazyRowsPerToken = (ngramSize - 1) * heads
		}
		// b11513 stores one raw and one pooled key per cell; query head
		// count does not multiply the persistent indexer cache.
		keyLength := parseInt64(metadata, arch+".attention.indexer.key_length")
		a.IndexerKeyLength = 2 * max(keyLength, 0)
		if a.IndexerKeyLength > 0 {
			profile.ComputeKind = ComputeIndexed
		}
		a.IndexerTopK = parseInt64(metadata, arch+".attention.indexer.top_k")
		ratios, _ := gguf.ParseInt64OrArray(metadata.values, arch+".attention.compress_ratios")
		for _, ratio := range ratios {
			if ratio > 0 && (a.IndexerCompression == 0 || ratio < a.IndexerCompression) {
				a.IndexerCompression = ratio
			}
		}
		connections := max(parseInt64(metadata, arch+".hyper_connection.count"), 1)
		profile.Dimensions.OutputEmbeddingLength = profile.Dimensions.EmbeddingLength * connections
	}
	trunkLayers := max(blockCount-max(profile.Speculation.NextNPredictLayers, 0), 0)

	pattern, ok := parseBoolPattern(metadata.value(arch+".attention.recurrent_layers"), blockCount)
	if !ok {
		interval := int64(4)
		if configured, err := gguf.ParseInt64(metadata.values, arch+".full_attention_interval"); err == nil && configured > 0 {
			interval = configured
		}

		pattern = make([]bool, blockCount)
		for layer := range trunkLayers {
			pattern[layer] = (layer+1)%interval != 0
		}
	}

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

	convKernel, convErr := gguf.ParseInt64(metadata.values, arch+".ssm.conv_kernel")
	innerSize, innerErr := gguf.ParseInt64(metadata.values, arch+".ssm.inner_size")
	stateSize, stateErr := gguf.ParseInt64(metadata.values, arch+".ssm.state_size")
	groupCount, groupErr := gguf.ParseInt64(metadata.values, arch+".ssm.group_count")
	if convErr != nil || innerErr != nil || stateErr != nil || groupErr != nil ||
		convKernel <= 0 || innerSize <= 0 || stateSize <= 0 || groupCount <= 0 {
		return nil
	}

	rWidth := (convKernel - 1) * (innerSize + 2*groupCount*stateSize)
	sWidth := stateSize * innerSize
	a.RecurrentStateBytes = 4 * (rWidth + sWidth)
	return nil
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

	trimmed := strings.Trim(strings.TrimSpace(value), "[]")
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	pattern := make([]bool, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "true":
			pattern = append(pattern, true)
		case "false":
			pattern = append(pattern, false)
		}
	}
	return pattern, int64(len(pattern)) == count
}
