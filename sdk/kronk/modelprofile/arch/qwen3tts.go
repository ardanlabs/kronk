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
	return strings.EqualFold(architecture, "qwen3tts")
}

// Apply identifies speech without requiring optional tags or a decoder file.
func (Qwen3TTS) Apply(_ Metadata, profile *modelprofile.Profile) error {
	if profile.Role == modelprofile.RoleLanguage && profile.Decision.Type == "" {
		profile.Purpose = modelprofile.PurposeSpeech
	}
	return nil
}
