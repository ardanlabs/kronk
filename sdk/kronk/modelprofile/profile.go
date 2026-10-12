// Package modelprofile owns the translation from raw GGUF metadata into
// architecture-independent model capabilities. Runtime and tooling packages
// consume Profile values and must not interpret architecture names directly.
package modelprofile

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// Class describes the execution-state shape of a model.
type Class string

const (
	ClassDense  Class = "dense"
	ClassMoE    Class = "moe"
	ClassHybrid Class = "hybrid"
)

// Role describes the role an artifact plays at runtime.
type Role string

const (
	RoleLanguage      Role = "language"
	RoleVisionEncoder Role = "vision_encoder"
)

// ComputeKind identifies the workspace allocation family, independently of
// dense/MoE weight layout and recurrent/attention cache layout.
type ComputeKind uint8

const (
	ComputeGeneric    ComputeKind = iota // Unvalidated architectures use the generic allowance.
	ComputeIndexed                       // Qwen4Exp indexed-attention workspaces.
	ComputeGemma4                        // Gemma4 target and shared-KV assistant workspaces.
	ComputeQwen35                        // Qwen3.5 target and own-KV MTP workspaces.
	ComputeGPTOSS                        // GPT-OSS alternating full/SWA attention workspaces.
	ComputeMistral3                      // Mistral3 dense full-attention workspaces.
	ComputeLFM2                          // LFM2 attention and short-convolution workspaces.
	ComputeGLM4MoE                       // GLM4 MoE full-attention and routed FFN workspaces.
	ComputeGemma3                        // Gemma3 non-SWA attention workspaces (RNJ).
	ComputeLlama                         // Llama full-attention GQA workspaces.
	ComputeClef                          // Clef batch-local decision graph without persistent cache.
	ComputeModernBERT                    // ModernBERT batch-local encoder and decision workspaces.
)

// Purpose describes the endpoint a model serves.
type Purpose string

const (
	PurposeGeneration Purpose = "generation"
	PurposeEmbedding  Purpose = "embedding"
	PurposeRerank     Purpose = "rerank"
)

// MemorySemantics describes how model state must be managed between sequences.
type MemorySemantics string

const (
	MemoryAttention MemorySemantics = "attention"
	MemoryRecurrent MemorySemantics = "recurrent"
)

// MTPArchitecture identifies the runtime contract used by an MTP head.
type MTPArchitecture string

const (
	MTPArchitectureNone          MTPArchitecture = ""
	MTPArchitectureQwen35OwnKV   MTPArchitecture = "qwen35-own-kv"
	MTPArchitectureQwen4ExpOwnKV MTPArchitecture = "qwen4exp-own-kv"
	MTPArchitectureNemotronOwnKV MTPArchitecture = "nemotron-own-kv"
	MTPArchitectureGemmaSharedKV MTPArchitecture = "gemma-shared-kv"
	MTPArchitectureGLM5Next      MTPArchitecture = "glm5-next"
)

// MTPEnabled reports whether Kronk enables an identified MTP runtime.
func MTPEnabled(architecture MTPArchitecture) bool {
	switch architecture {
	case MTPArchitectureQwen35OwnKV, MTPArchitectureQwen4ExpOwnKV, MTPArchitectureNemotronOwnKV, MTPArchitectureGemmaSharedKV:
		return true
	default:
		return false
	}
}

// Dimensions contains normalized model dimensions used by runtime and tooling
// consumers.
type Dimensions struct {
	BlockCount            int64
	ContextLength         int64
	EmbeddingLength       int64
	OutputEmbeddingLength int64 // Hidden-state width exposed to an MTP context.
	FeedForwardLength     int64
	VocabularySize        int64
	HeadCount             int64
	HeadCountKV           int64
	KeyLength             int64
	ValueLength           int64
}

// Speculation contains normalized MTP metadata.
type Speculation struct {
	NextNPredictLayers int64
	MTPArchitecture    MTPArchitecture
	SharedKVCompanion  bool
	OwnKVCompanion     bool
}

// Modalities contains modalities declared by model metadata. Image support
// still depends on the presence of a projection artifact.
type Modalities struct {
	Audio bool
	Video bool
}

// Profile is the normalized view of one model artifact.
type Profile struct {
	Architecture           string
	Class                  Class
	Role                   Role
	Purpose                Purpose
	MemorySemantics        MemorySemantics
	ComputeKind            ComputeKind
	Dimensions             Dimensions
	Decision               Decision
	Attention              gguf.AttentionFacts
	MoE                    gguf.MoEInfo
	Rope                   gguf.RopeFacts
	Speculation            Speculation
	Modalities             Modalities
	Batch                  BatchCapabilities
	FileType               int64
	HasChatTemplate        bool
	SupportsTensorParallel bool
	TiedOutputFallback     bool  // Loader reuses token embeddings when output.weight is absent.
	LazyRowsPerToken       int64 // Row gathers from the eligible per-layer embedding table; 0 = unsupported.

	issues []error
}

// Resolve translates raw GGUF metadata into normalized model capabilities.
// Missing optional fields retain their zero values; consumers validate the
// subset required for their operation.
func Resolve(values map[string]string) Profile {
	metadata := newMetadata(values)
	profile := resolveGeneric(metadata)

	for _, adapter := range architectureAdapters {
		if !adapter.Claims(profile.Architecture) {
			continue
		}
		if err := adapter.Apply(metadata, &profile); err != nil {
			profile.issues = append(profile.issues, fmt.Errorf("%s profile: %w", adapter.Name(), err))
		}
	}
	if profile.Role == RoleLanguage {
		profile.Batch = batchCapabilities(profile.Architecture)
	}
	profile.SupportsTensorParallel = profile.Role == RoleLanguage && supportsTensorParallel(profile.Architecture)

	profile.Attention.NextNPredictLayers = profile.Speculation.NextNPredictLayers
	switch {
	case profile.MemorySemantics == MemoryRecurrent:
		profile.Class = ClassHybrid
	case profile.MoE.IsMoE:
		profile.Class = ClassMoE
	default:
		profile.Class = ClassDense
	}

	return profile
}

// ValidateForVRAM verifies the facts required to calculate language-model
// memory. Vision encoders do not allocate a language-model KV cache.
func (p Profile) ValidateForVRAM() error {
	if err := errors.Join(p.issues...); err != nil {
		return err
	}
	if p.Architecture == "" {
		return fmt.Errorf("unable to detect model architecture")
	}
	if p.Role == RoleVisionEncoder {
		return nil
	}
	if p.Dimensions.BlockCount <= 0 {
		return fmt.Errorf("invalid or missing block_count")
	}
	headCountKV := p.Dimensions.HeadCountKV
	if p.Attention.FullHeadCountKV > 0 {
		headCountKV = p.Attention.FullHeadCountKV
	}
	if headCountKV <= 0 {
		return fmt.Errorf("invalid or missing head_count_kv and head_count fallback")
	}
	keyLength := p.Dimensions.KeyLength
	if p.Attention.FullKeyLength > 0 {
		keyLength = p.Attention.FullKeyLength
	}
	valueLength := p.Dimensions.ValueLength
	if p.Attention.FullValueLength > 0 {
		valueLength = p.Attention.FullValueLength
	}
	if keyLength <= 0 || valueLength <= 0 {
		return fmt.Errorf("invalid or missing key/value length metadata")
	}
	return nil
}

// ValidateForAnalysis verifies the facts required by model analysis.
func (p Profile) ValidateForAnalysis() error {
	if err := errors.Join(p.issues...); err != nil {
		return err
	}
	if p.Architecture == "" {
		return fmt.Errorf("unable to detect architecture")
	}
	if p.Dimensions.BlockCount <= 0 {
		return fmt.Errorf("invalid or missing block_count")
	}
	return nil
}
