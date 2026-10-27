package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// SequenceProcessing describes topology without changing runtime memory facts.
func SequenceProcessing(metadata Metadata, p modelprofile.Profile) string {
	if p.Architecture == "" {
		return "Unknown"
	}
	// Some architectures expose a layer pattern without a runtime adapter.
	// Use it for catalog classification without altering memory accounting.
	if p.Attention.RecurrentPattern == nil && p.Dimensions.BlockCount > 0 {
		if pattern, ok := parseBoolPattern(metadata.value(p.Architecture+".attention.recurrent_layers"), p.Dimensions.BlockCount); ok {
			p.Attention.RecurrentPattern = pattern
			p.Attention.RecurrentLayers = 0
			trunkLayers := max(p.Dimensions.BlockCount-p.Speculation.NextNPredictLayers, 0)
			for _, recurrent := range pattern[:trunkLayers] {
				if recurrent {
					p.Attention.RecurrentLayers++
				}
			}
			p.Attention.FullAttentionLayers = trunkLayers - p.Attention.RecurrentLayers
			p.Attention.SlidingWindowLayers = 0
		}
	}
	if p.Attention.RecurrentPattern != nil {
		switch {
		case p.Attention.RecurrentLayers == 0:
			return "Attention-only"
		case p.Attention.FullAttentionLayers+p.Attention.SlidingWindowLayers > 0:
			return "Hybrid"
		default:
			return "Recurrent"
		}
	}
	if p.MemorySemantics != modelprofile.MemoryRecurrent {
		return "Attention-only"
	}
	if strings.HasPrefix(strings.ToLower(p.Architecture), "mamba") {
		return "Recurrent"
	}
	if (KnownRecurrent{}).Claims(p.Architecture) || (QwenHybrid{}).Claims(p.Architecture) {
		return "Hybrid"
	}
	return "Unknown"
}
