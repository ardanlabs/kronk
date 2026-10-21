// Package modelprofile owns the translation from raw GGUF metadata into
// architecture-independent model capabilities. Runtime and tooling packages
// consume Profile values and must not interpret architecture names directly.
package modelprofile

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

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
	SequenceProcessing     string // Attention-only, Recurrent, Hybrid, or Unknown.
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
	profile.SequenceProcessing = sequenceProcessing(metadata, profile)

	return profile
}

// ParameterActivation reports feed-forward parameter activation independently
// of sequence processing and runtime memory semantics.
func (p Profile) ParameterActivation() string {
	if p.MoE.IsMoE {
		return "MoE"
	}
	return "Dense"
}

// sequenceProcessing describes topology without changing runtime memory facts.
func sequenceProcessing(metadata metadata, p Profile) string {
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
	if p.MemorySemantics != MemoryRecurrent {
		return "Attention-only"
	}
	if strings.HasPrefix(strings.ToLower(p.Architecture), "mamba") {
		return "Recurrent"
	}
	if (knownRecurrentAdapter{}).Claims(p.Architecture) || (qwenHybridAdapter{}).Claims(p.Architecture) {
		return "Hybrid"
	}
	return "Unknown"
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
