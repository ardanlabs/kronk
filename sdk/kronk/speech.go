package kronk

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Speech generates a complete PCM16 mono WAV from text and an optional preset
// voice. The returned bytes belong to the caller. Cancellation can wait for an
// in-flight native step or vocoder call to finish; failed calls return no audio.
// A speech model and matching audio decoder are required; model.WithSpeech is
// optional and only overrides limits. The default frame ceiling is 512, and
// exhaustion returns model.ErrSpeechLimit rather than truncated audio.
// Input/voice/context errors return model.ErrInvalidRequest; unsupported speech
// capability returns model.ErrSpeechUnsupported. Match errors with errors.Is.
// Speech uses the handle's normal admission and active-use protection, including
// when acquired from sdk/kronk/pool. Audio streaming is not exposed.
func (krn *Kronk) Speech(ctx context.Context, req model.SpeechRequest) (model.SpeechResponse, error) {
	call := func(m *model.Model) (model.SpeechResponse, error) {
		return m.Speech(ctx, req)
	}
	return nonStreaming(ctx, krn, call)
}
