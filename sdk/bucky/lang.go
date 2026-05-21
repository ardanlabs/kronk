package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

// DetectLanguage runs a short whisper pass on the supplied 16 kHz
// mono float32 PCM samples and returns the detected language code
// along with the per-language probability vector (length
// LangMaxID()+1) when withProbs is true.
//
// DetectLanguage participates in the per-handle backpressure
// semaphore and blocks until a slot is available.
func (w *Whisper) DetectLanguage(ctx context.Context, samples []float32, withProbs bool) (string, []float32, error) {
	m, err := w.acquireModel(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("detect-language: %w", err)
	}
	defer w.releaseModel()

	return m.DetectLanguage(ctx, samples, withProbs)
}

// =============================================================================

// LangID returns the whisper.cpp internal id for the supplied
// language code (e.g. "de" → 2). Returns -1 if the code is unknown.
//
// Init must have been called before LangID, since the FFI symbol is
// resolved by whisper.Load.
func LangID(lang string) int32 { return model.LangID(lang) }

// LangStr returns the short language code for the supplied id (e.g.
// 2 → "de"). Returns "" if the id is invalid.
func LangStr(id int32) string { return model.LangStr(id) }

// LangMaxID returns the largest language id whisper.cpp knows. The
// number of supported languages is LangMaxID()+1.
func LangMaxID() int32 { return model.LangMaxID() }
