package kronk

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Speech generates a complete PCM16 WAV using a speech model and matching
// audio decoder. The returned audio belongs to the caller; failed calls
// return no audio. Cancellation may wait for an in-flight native call.
//
// See [model.SpeechRequest] for input and conditioning, [model.SpeechConfig]
// for generation limits, and [model.SpeechResponse] for output details.
// Reaching the frame limit returns [model.ErrSpeechLimit].
func (krn *Kronk) Speech(ctx context.Context, req model.SpeechRequest) (model.SpeechResponse, error) {
	call := func(m *model.Model) (model.SpeechResponse, error) {
		return m.Speech(ctx, req)
	}
	return nonStreaming(ctx, krn, call)
}
