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

// Dimensions contains normalized model dimensions used by runtime and tooling
// consumers.
type Dimensions struct {
	BlockCount        int64
	ContextLength     int64
	EmbeddingLength   int64
	FeedForwardLength int64
	VocabularySize    int64
	HeadCount         int64
	HeadCountKV       int64
	KeyLength         int64
	ValueLength       int64
}

// Speculation contains normalized MTP metadata.
type Speculation struct {
	NextNPredictLayers int64
	SharedKVCompanion  bool
}

// Modalities contains modalities declared by model metadata. Image support
// still depends on the presence of a projection artifact.
type Modalities struct {
	Audio bool
	Video bool
}

// Profile is the normalized view of one model artifact.
type Profile struct {
	Architecture    string
	Class           Class
	Role            Role
	Purpose         Purpose
	MemorySemantics MemorySemantics
	Dimensions      Dimensions
	Attention       gguf.AttentionFacts
	MoE             gguf.MoEInfo
	Rope            gguf.RopeFacts
	Speculation     Speculation
	Modalities      Modalities
	FileType        int64
	HasChatTemplate bool

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
