package speech

import (
	"errors"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// ChatterboxTurbo accepts speaker references but not language selection
// or transcripts. Native speaker-reference preparation is shared by the engine.
type ChatterboxTurbo struct{}

// Validate rejects conditioning that Chatterbox does not support.
func (*ChatterboxTurbo) Validate(language, referenceText string) (string, error) {
	if language != "" || referenceText != "" {
		return language, errors.New("language and reference transcripts require Qwen3-TTS")
	}
	return language, nil
}

// Prepare requires no extra native settings or transcript-token budget.
func (*ChatterboxTurbo) Prepare(llama.Vocab, string, string, *mtmd.HelperGenAudioInput) (int, error) {
	return 0, nil
}
