package bucky

import (
	"context"
	"fmt"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// LangID returns the whisper.cpp internal id for the supplied
// language code (e.g. "de" → 2). Returns -1 if the code is unknown.
//
// Init must have been called before LangID, since the FFI symbol is
// resolved by whisper.Load.
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

// DetectLanguage runs a short whisper pass on the supplied 16 kHz
// mono float32 PCM samples and returns the detected language code
// along with the per-language probability vector (length
// LangMaxID()+1) when withProbs is true.
//
// DetectLanguage takes the same backpressure slot as Transcribe.
func (w *Whisper) DetectLanguage(ctx context.Context, samples []float32, withProbs bool) (string, []float32, error) {
	if len(samples) == 0 {
		return "", nil, fmt.Errorf("detect-language: empty samples")
	}

	handle, err := w.acquire(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("detect-language: %w", err)
	}
	defer w.release()

	// A short encode-only Full pass populates the mel spectrogram so
	// LangAutoDetect can score the candidates.
	params := whisper.FullDefaultParams(whisper.SamplingGreedy)
	params.PrintProgress = 0
	params.PrintRealtime = 0
	params.PrintTimestamps = 0
	params.NoTimestamps = 1
	params.SingleSegment = 1
	params.DetectLanguage = 1

	if w.cfg.NThreads > 0 {
		params.NThreads = w.cfg.NThreads
	}

	if err := whisper.Full(handle, params, samples); err != nil {
		return "", nil, fmt.Errorf("detect-language: %w", err)
	}

	var probs []float32
	if withProbs {
		probs = make([]float32, whisper.LangMaxID()+1)
		if id := whisper.LangAutoDetect(handle, 0, params.NThreads, probs); id < 0 {
			return "", nil, fmt.Errorf("detect-language: whisper_lang_auto_detect returned %d", id)
		}
	}

	return whisper.LangStr(whisper.FullLangID(handle)), probs, nil
}
