package kronk

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Speech generates a complete PCM16 mono WAV from text and an optional preset
// voice. The returned bytes belong to the caller. Cancellation can wait for an
// in-flight native step or vocoder call to finish; failed calls return no audio.
func (krn *Kronk) Speech(ctx context.Context, req model.SpeechRequest) (model.SpeechResponse, error) {
	call := func(m *model.Model) (model.SpeechResponse, error) {
		return m.Speech(ctx, req)
	}
	return nonStreaming(ctx, krn, call)
}
