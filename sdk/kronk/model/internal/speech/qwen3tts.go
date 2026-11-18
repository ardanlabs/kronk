package speech

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Qwen3TTS owns Qwen's language tokens and transcript conditioning.
// A decoder with the codec reference encoder uses transcripts for in-context
// conditioning; older decoders fall back to speaker-embedding conditioning.
type Qwen3TTS struct{}

// NewExecution creates Qwen's externally sampled autoregressive protocol.
func (*Qwen3TTS) NewExecution(ctx context.Context, log applog.Logger, cfg ExecutionConfig) (Execution, error) {
	return NewAutoregressive(ctx, log, cfg)
}

// Validate normalizes Qwen language names and codes. Reference transcripts are
// supported; common transcript validation remains with the request owner.
func (*Qwen3TTS) Validate(language, _ string) (string, error) {
	if language != "" {
		switch strings.ToLower(strings.TrimSpace(language)) {
		case "en", "english":
			language = "english"
		case "zh", "chinese":
			language = "chinese"
		case "de", "german":
			language = "german"
		case "it", "italian":
			language = "italian"
		case "pt", "portuguese":
			language = "portuguese"
		case "es", "spanish":
			language = "spanish"
		case "ja", "japanese":
			language = "japanese"
		case "ko", "korean":
			language = "korean"
		case "fr", "french":
			language = "french"
		case "ru", "russian":
			language = "russian"
		default:
			return language, fmt.Errorf("unsupported speech language %q", language)
		}
	}
	return language, nil
}

// Prepare applies validated conditioning and checks the autoregressive budget.
// The returned count describes the text to speak, excluding conditioning.
func (*Qwen3TTS) Prepare(_ context.Context, vocab llama.Vocab, prompt Prompt, input *mtmd.HelperGenAudioInput) (int, error) {
	if prompt.Language != "" {
		piece := "<|codec_language_" + prompt.Language + "|>"
		ids := llama.Tokenize(vocab, piece, false, true)
		if len(ids) != 1 || llama.VocabGetText(vocab, ids[0]) != piece {
			return 0, fmt.Errorf("speech bundle does not support language %q", prompt.Language)
		}
		if err := input.SetLang(prompt.Language); err != nil {
			return 0, fmt.Errorf("speech language: %w", err)
		}
	}
	if err := input.SetSpeakerRefText(prompt.ReferenceText); err != nil {
		return 0, fmt.Errorf("speech reference transcript: %w", err)
	}
	return prepareAutoregressivePrompt(vocab, prompt, len(llama.Tokenize(vocab, prompt.ReferenceText, false, true)), input)
}
