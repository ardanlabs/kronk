package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// TinyVoca identifies the noncausal phoneme encoder and its speech decoder.
type TinyVoca struct{}

// Name returns the adapter's diagnostic name.
func (TinyVoca) Name() string { return "tiny-voca" }

// Claims reports whether the architecture can contain a Tiny-Voca artifact.
func (TinyVoca) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "tiny-voca") || strings.EqualFold(architecture, "clip")
}

// Apply identifies the backbone and its frame-stack/source-filter companion.
func (TinyVoca) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	switch {
	case profile.Role == modelprofile.RoleLanguage && profile.Decision.Type == "":
		profile.Purpose, profile.ComputeKind = modelprofile.PurposeSpeech, modelprofile.ComputeTinyVocaBackbone
		// Native exposes unused logits through a duplicated token table.
		profile.TiedOutputFallback = true
	case profile.Role == modelprofile.RoleVisionEncoder &&
		metadata.value("clip.has_gen_audio_encoder") == "true" &&
		metadata.value("clip.gen.audio.projector_type") == "tiny_voca":
		profile.ComputeKind = modelprofile.ComputeTinyVocaDecoder
	}
	if profile.ComputeKind == modelprofile.ComputeTinyVocaBackbone || profile.ComputeKind == modelprofile.ComputeTinyVocaDecoder {
		profile.Warnings = append(profile.Warnings, "Tiny-Voca memory estimates are provisional source-derived allowances, not measured allocator peaks; require fixed speed 1.0, at most 509 mapped phonemes across sentences, at most 1527 total padded tokens and 512 per segment; native rejects segments exceeding 3000 duration frames; Speech.MaxFrames is not an autoregressive output cap; host output reservation grows per slot and may be large")
	}
	return nil
}
