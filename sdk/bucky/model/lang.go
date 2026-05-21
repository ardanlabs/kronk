package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// LangID returns the whisper.cpp internal id for the supplied
// language code (e.g. "de" → 2). Returns -1 if the code is unknown.
//
// The bucky Init function must have been called before LangID, since
// the underlying FFI symbol is resolved by whisper.Load.
func LangID(lang string) int32 {
	return whisper.LangID(lang)
}

// LangStr returns the short language code for the supplied id (e.g.
// 2 → "de"). Returns "" if the id is invalid.
func LangStr(id int32) string {
	return whisper.LangStr(id)
}

// LangMaxID returns the largest language id whisper.cpp knows. The
// number of supported languages is LangMaxID()+1.
func LangMaxID() int32 {
	return whisper.LangMaxID()
}

// =============================================================================

// DetectLanguage runs a short whisper pass on the supplied 16 kHz
// mono float32 PCM samples and returns the detected language code
// along with the per-language probability vector (length
// LangMaxID()+1) when withProbs is true.
//
// DetectLanguage acquires a whisper.State from the model's internal
// pool, so up to Config.NSeqMax goroutines may run DetectLanguage in
// parallel against the same Model.
func (m *Model) DetectLanguage(ctx context.Context, samples []float32, withProbs bool) (string, []float32, error) {
	if m.handle == 0 {
		return "", nil, fmt.Errorf("detect-language: model has been unloaded")
	}
	if len(samples) == 0 {
		return "", nil, fmt.Errorf("detect-language: empty samples")
	}

	// A short Full pass with DetectLanguage=1 / SingleSegment=1
	// populates the mel spectrogram and selects the top language id
	// without running the full decoder.
	params := whisper.FullDefaultParams(whisper.SamplingGreedy)
	params.PrintProgress = 0
	params.PrintRealtime = 0
	params.PrintTimestamps = 0
	params.NoTimestamps = 1
	params.SingleSegment = 1
	params.DetectLanguage = 1

	if m.cfg.NThreads > 0 {
		params.NThreads = m.cfg.NThreads
	}

	ps, err := m.pool.acquire(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("detect-language: %w", err)
	}
	defer m.pool.release(ps)

	if err := whisper.FullWithState(m.handle, ps.state, params, samples); err != nil {
		return "", nil, fmt.Errorf("detect-language: %w", err)
	}

	var probs []float32
	if withProbs {
		probs = make([]float32, whisper.LangMaxID()+1)
		if id := whisper.LangAutoDetectWithState(m.handle, ps.state, 0, params.NThreads, probs); id < 0 {
			return "", nil, fmt.Errorf("detect-language: whisper_lang_auto_detect_with_state returned %d", id)
		}
	}

	return whisper.LangStr(whisper.FullLangIDFromState(ps.state)), probs, nil
}
