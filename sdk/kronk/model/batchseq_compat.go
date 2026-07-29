package model

import "strings"

// supportsBatchSeq reports whether a model architecture is proven safe for the
// multi-sequence batch runtime. Models not explicitly listed use the context
// pool as a fallback because some unsupported llama.cpp architectures assert
// during multi-sequence context initialization instead of returning a
// recoverable error. EmbeddingGemma intentionally remains on that fallback.
func supportsBatchSeq(mi ModelInfo) bool {
	architecture := strings.ToLower(strings.TrimSpace(mi.Metadata["general.architecture"]))

	switch {
	case mi.IsEmbedModel:
		return architecture == "qwen3"
	case mi.IsRerankModel:
		return architecture == "bert"
	default:
		return false
	}
}
