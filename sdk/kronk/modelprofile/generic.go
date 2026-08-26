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
			BlockCount:        blockCount,
			ContextLength:     parseInt64WithFallback(metadata, arch+".context_length", ".context_length"),
			EmbeddingLength:   parseInt64WithFallback(metadata, arch+".embedding_length", ".embedding_length"),
			FeedForwardLength: parseInt64WithFallback(metadata, arch+".feed_forward_length", ".feed_forward_length"),
			VocabularySize:    parseInt64WithFallback(metadata, arch+".vocab_size", "tokenizer.ggml.tokens"),
			HeadCount:         headCount,
			HeadCountKV:       headCountKV,
			KeyLength:         keyLength,
			ValueLength:       valueLength,
		},
		Attention:       gguf.ParseAttentionFacts(metadata.values, arch, blockCount),
		MoE:             gguf.DetectMoE(metadata.values),
		Rope:            gguf.ParseRopeFacts(metadata.values, arch),
		Speculation:     Speculation{NextNPredictLayers: metadata.positiveNextNPredictLayers()},
		FileType:        parseInt64(metadata, "general.file_type"),
		HasChatTemplate: gguf.HasChatTemplate(metadata.values),
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
