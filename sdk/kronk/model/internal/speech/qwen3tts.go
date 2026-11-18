// Package speech implements speech conditioning and execution protocols.
// Scheduling and public request/response types belong to model.
package speech

import (
	"fmt"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Qwen3TTS owns Qwen's language tokens and transcript conditioning.
type Qwen3TTS struct{}

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

// Prepare applies validated conditioning on the native engine owner and returns
// additional transcript tokens for admission budgeting. Errors reject the request.
func (*Qwen3TTS) Prepare(vocab llama.Vocab, language, referenceText string, input *mtmd.HelperGenAudioInput) (int, error) {
	if language != "" {
		piece := "<|codec_language_" + language + "|>"
		ids := llama.Tokenize(vocab, piece, false, true)
		if len(ids) != 1 || llama.VocabGetText(vocab, ids[0]) != piece {
			return 0, fmt.Errorf("speech bundle does not support language %q", language)
		}
		if err := input.SetLang(language); err != nil {
			return 0, fmt.Errorf("speech language: %w", err)
		}
	}
	if err := input.SetSpeakerRefText(referenceText); err != nil {
		return 0, fmt.Errorf("speech reference transcript: %w", err)
	}
	return len(llama.Tokenize(vocab, referenceText, false, true)), nil
}
