package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// Kokoro identifies the noncausal phoneme encoder and its speech decoder.
type Kokoro struct{}

// Name returns the adapter's diagnostic name.
func (Kokoro) Name() string { return "kokoro" }

// Claims reports whether the architecture can contain a Kokoro artifact.
func (Kokoro) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "kokoro") || strings.EqualFold(architecture, "clip")
}

// Apply identifies the backbone and its prosody/vocoder companion independently.
func (Kokoro) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	switch {
	case profile.Role == modelprofile.RoleLanguage && profile.Decision.Type == "":
		profile.Purpose, profile.ComputeKind = modelprofile.PurposeSpeech, modelprofile.ComputeKokoroBackbone
		// b12503 exposes unused logits through a duplicated token table.
		profile.TiedOutputFallback = true
	case profile.Role == modelprofile.RoleVisionEncoder &&
		metadata.value("clip.has_gen_audio_encoder") == "true" &&
		metadata.value("clip.gen.audio.projector_type") == "kokoro":
		profile.ComputeKind = modelprofile.ComputeKokoroDecoder
	}
	if profile.ComputeKind == modelprofile.ComputeKokoroBackbone || profile.ComputeKind == modelprofile.ComputeKokoroDecoder {
		profile.Warnings = append(profile.Warnings, "Kokoro memory estimates are provisional for one segment of at most 510 mapped phonemes at fixed speed 1.0; decoder workspace uses Metal observation plus source liveness at twice the observed duration, not the theoretical duration bound; longer outputs or other backends can exceed this estimate; Speech.MaxFrames does not apply")
	}
	return nil
}
