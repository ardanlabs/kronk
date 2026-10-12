package modelprofile

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

func resolveGeneric(metadata metadata) Profile {
	arch := gguf.DetectArchitecture(metadata.values)
	blockCount, _ := gguf.ParseInt64WithFallback(metadata.values, arch+".block_count", ".block_count")
	headCount, _ := gguf.ParseInt64(metadata.values, arch+".attention.head_count")
	headCountKV, err := gguf.ParseInt64OrArrayAvg(metadata.values, arch+".attention.head_count_kv")
	if err != nil {
		headCountKV = headCount
	}
	keyLength, valueLength, _ := gguf.ResolveKVLengths(metadata.values, arch)

	profile := Profile{
		Architecture:    arch,
		Role:            RoleLanguage,
		Purpose:         PurposeGeneration,
		MemorySemantics: MemoryAttention,
		Dimensions: Dimensions{
			BlockCount:            blockCount,
			ContextLength:         parseInt64WithFallback(metadata, arch+".context_length", ".context_length"),
			EmbeddingLength:       parseInt64WithFallback(metadata, arch+".embedding_length", ".embedding_length"),
			OutputEmbeddingLength: parseInt64(metadata, arch+".embedding_length_out"),
			FeedForwardLength:     parseInt64WithFallback(metadata, arch+".feed_forward_length", ".feed_forward_length"),
			VocabularySize:        parseInt64WithFallback(metadata, arch+".vocab_size", "tokenizer.ggml.tokens"),
			HeadCount:             headCount,
			HeadCountKV:           headCountKV,
			KeyLength:             keyLength,
			ValueLength:           valueLength,
		},
		Decision:        resolveDecision(metadata, arch),
		Attention:       gguf.ParseAttentionFacts(metadata.values, arch, blockCount),
		MoE:             gguf.DetectMoE(metadata.values),
		Rope:            gguf.ParseRopeFacts(metadata.values, arch),
		Speculation:     Speculation{NextNPredictLayers: metadata.positiveNextNPredictLayers()},
		FileType:        parseInt64(metadata, "general.file_type"),
		HasChatTemplate: gguf.HasChatTemplate(metadata.values),
	}
	if arch == "gemma4" || arch == "gemma4-assistant" {
		profile.ComputeKind = ComputeGemma4
	}
	if arch == "gemma4" && parseInt64(metadata, arch+".embedding_length_per_layer_input") > 0 {
		// The assistant does not create a lazy table. The target gathers
		// one row containing the embeddings for all layers per token.
		profile.LazyRowsPerToken = 1
	}
	if arch == "llama" {
		profile.ComputeKind = ComputeLlama
	}
	if arch == "clef" {
		profile.ComputeKind = ComputeClef
	}
	if arch == "modern-bert" {
		profile.ComputeKind = ComputeModernBERT
	}
	if arch == "gemma3" && profile.Attention.SlidingWindow <= 0 {
		// RNJ uses Gemma3's ordinary KV graph. SWA Gemma3 graphs remain
		// on the fallback until their separate mask policy is validated.
		profile.ComputeKind = ComputeGemma3
	}
	if arch == "mistral3" {
		profile.ComputeKind = ComputeMistral3
	}
	if arch == "glm4moe" {
		profile.ComputeKind = ComputeGLM4MoE
	}
	if arch == "gpt-oss" {
		profile.ComputeKind = ComputeGPTOSS
		// b11513 alternates SWA on even blocks and full attention on odd
		// blocks; sliding_window alone does not make every block SWA.
		profile.Attention.SWAPattern = make([]bool, max(blockCount, 0))
		profile.Attention.SlidingWindowLayers = (max(blockCount, 0) + 1) / 2
		profile.Attention.FullAttentionLayers = blockCount - profile.Attention.SlidingWindowLayers
		for layer := range profile.Attention.SWAPattern {
			profile.Attention.SWAPattern[layer] = layer%2 == 0
		}
	}

	// These b11513 loaders duplicate token_embd for a missing output.
	// Missing output.weight alone is not sufficient (for example, BERT).
	switch strings.ToLower(arch) {
	case "llama", "qwen2", "qwen3", "qwen3moe", "qwen35", "qwen35moe",
		"gemma", "gemma2", "gemma3", "gemma4", "gemma-embedding", "phi3":
		profile.TiedOutputFallback = true
	}

	applyGenericPurposeAndModalities(metadata, &profile)
	if metadata.hasKeyFragment(".ssm.", ".mamba.", ".recurrent.") {
		profile.MemorySemantics = MemoryRecurrent
	}

	return profile
}

func applyGenericPurposeAndModalities(metadata metadata, profile *Profile) {
	hint := strings.ToLower(strings.Join([]string{
		gguf.GeneralName(metadata.values),
		gguf.GeneralBasename(metadata.values),
		gguf.GeneralTags(metadata.values),
	}, " "))

	if strings.Contains(hint, "embed") {
		profile.Purpose = PurposeEmbedding
	}

	anyToAny := strings.Contains(hint, "any-to-any") || strings.Contains(hint, "omni")
	profile.Modalities.Audio = anyToAny || strings.Contains(hint, "audio")
	profile.Modalities.Video = anyToAny || strings.Contains(hint, "video")
}

func parseInt64(metadata metadata, key string) int64 {
	value, _ := gguf.ParseInt64(metadata.values, key)
	return value
}

func parseInt64WithFallback(metadata metadata, key, suffix string) int64 {
	value, _ := gguf.ParseInt64WithFallback(metadata.values, key, suffix)
	return value
}
