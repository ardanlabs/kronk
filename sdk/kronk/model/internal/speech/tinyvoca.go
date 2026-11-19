package speech

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speech/normalization"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// TinyVoca owns English normalization and bounded phoneme admission for the
// noncausal 512-position encoder. Its unchanged TTS template supplies sentence
// boundaries, dictionaries and pronunciation rules; the decoder supplies G2P
// for unknown words. M2 and F2 are the supported voices; recordings are not.
// Requests admit at most 509 phoneme symbols total across sentences and at most
// 512 bytes per unknown word sent to the native pronunciation model. Native
// execution adds sentence padding and vocodes the complete result at speed 1.
// There is no autoregressive frame budget; MaxFrames overrides are rejected.
type TinyVoca struct {
	renderer *normalization.Renderer
	g2p      func(string) (string, error)
}

// NewTinyVoca borrows the model dictionaries and decoder pronunciation model.
// Both native handles must outlive this adapter and its executions.
func NewTinyVoca(mdl llama.Model, decoder mtmd.Context, maxFrames int, count func(llama.Model, string) (int, error), value func(llama.Model, string, int32) (string, bool), g2p func(mtmd.Context, string) (string, error)) (*TinyVoca, error) {
	if maxFrames != 512 {
		return nil, fmt.Errorf("%w: Tiny-Voca is non-autoregressive and does not support MaxFrames overrides", ErrInvalidConfig)
	}
	if llama.ModelNCtxTrain(mdl) != 512 {
		return nil, errors.New("Tiny-Voca requires a 512-position encoder")
	}
	if _, err := g2p(decoder, ""); err != nil {
		return nil, fmt.Errorf("Tiny-Voca pronunciation support: %w", err)
	}
	renderer, err := newTTSNormalizer(mdl, count, value)
	if err != nil {
		return nil, err
	}
	t := TinyVoca{renderer: renderer, g2p: func(word string) (string, error) {
		if len(word) > 512 {
			return "", errors.New("Tiny-Voca unknown words must be at most 512 bytes")
		}
		return g2p(decoder, word)
	}}
	return &t, nil
}

// NewExecution creates Tiny-Voca's whole-sentence encoder/synthesizer protocol.
func (*TinyVoca) NewExecution(_ context.Context, _ applog.Logger, cfg ExecutionConfig) (Execution, error) {
	return NewNonAutoregressive(cfg.Context, cfg.Decoder)
}

// Validate accepts English and rejects unsupported language and transcript controls.
func (*TinyVoca) Validate(language, referenceText string) (string, error) {
	if referenceText != "" {
		return language, errors.New("Tiny-Voca does not support reference recordings or transcripts")
	}
	language = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(language)), "_", "-")
	switch language {
	case "", "en", "en-us", "english":
		return "en", nil
	default:
		return language, fmt.Errorf("unsupported Tiny-Voca speech language %q: use en", language)
	}
}

// SupportsVoice accepts Tiny-Voca's two speaker presets.
func (*TinyVoca) SupportsVoice(name string) bool {
	return name == "M2" || name == "F2"
}

// Prepare renders English to space-separated, stressed phoneme symbols. The
// total limit bounds retained sentence states and output, not raw text length.
func (t *TinyVoca) Prepare(ctx context.Context, _ llama.Vocab, prompt Prompt, input *mtmd.HelperGenAudioInput) (int, error) {
	if !t.SupportsVoice(prompt.Voice) {
		return 0, fmt.Errorf("unsupported Tiny-Voca voice %q: use M2 or F2", prompt.Voice)
	}
	phonemes, err := t.renderer.Render(ctx, prompt.Text, prompt.Language, prompt.Voice, t.g2p)
	if err != nil {
		return 0, fmt.Errorf("Tiny-Voca text normalization: %w", err)
	}
	var count int
	for symbol := range strings.FieldsSeq(phonemes) {
		switch symbol {
		case "<bos>", "<pad>", "<eos>":
		default:
			count++
		}
	}
	if count == 0 || count > 509 {
		return 0, fmt.Errorf("Tiny-Voca requires 1 through 509 phoneme symbols after normalization, got %d", count)
	}
	if !strings.HasSuffix(strings.TrimSpace(phonemes), "<eos>") {
		return 0, errors.New("Tiny-Voca normalization must supply explicit sentence boundaries")
	}
	input.SetPrompt(phonemes)
	input.Speed = 1
	return count, nil
}
