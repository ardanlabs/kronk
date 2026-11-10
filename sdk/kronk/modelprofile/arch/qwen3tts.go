package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// Qwen3TTS identifies the dedicated speech-generation architecture.
type Qwen3TTS struct{}

// Name returns the adapter's diagnostic name.
func (Qwen3TTS) Name() string { return "qwen3tts" }

// Claims reports whether the architecture uses this adapter.
func (Qwen3TTS) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "qwen3tts") || strings.EqualFold(architecture, "clip")
}

// Apply identifies the backbone and its predictor/codec companion independently.
func (Qwen3TTS) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	switch {
	case profile.Role == modelprofile.RoleLanguage && profile.Decision.Type == "":
		profile.Purpose, profile.ComputeKind = modelprofile.PurposeSpeech, modelprofile.ComputeQwen3TTSBackbone
	case profile.Role == modelprofile.RoleVisionEncoder &&
		metadata.value("clip.has_gen_audio_encoder") == "true" &&
		metadata.value("clip.gen.audio.projector_type") == "qwen3tts_gen":
		profile.ComputeKind = modelprofile.ComputeQwen3TTSDecoder
	}
	return nil
}
