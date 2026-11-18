package speech

import (
	"context"
	"errors"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// ChatterboxTurbo accepts speaker references but not language selection
// or transcripts. Native speaker-reference preparation is shared by the engine.
type ChatterboxTurbo struct{}

// NewExecution creates Chatterbox's externally sampled autoregressive protocol.
func (*ChatterboxTurbo) NewExecution(ctx context.Context, log applog.Logger, cfg ExecutionConfig) (Execution, error) {
	return NewAutoregressive(ctx, log, cfg)
}

// Validate rejects conditioning that Chatterbox does not support.
func (*ChatterboxTurbo) Validate(language, referenceText string) (string, error) {
	if language != "" || referenceText != "" {
		return language, errors.New("language and reference transcripts require Qwen3-TTS")
	}
	return language, nil
}

// Prepare checks the autoregressive text budget without transcript conditioning.
func (*ChatterboxTurbo) Prepare(_ context.Context, vocab llama.Vocab, prompt Prompt, input *mtmd.HelperGenAudioInput) (int, error) {
	return prepareAutoregressivePrompt(vocab, prompt, 0, input)
}
