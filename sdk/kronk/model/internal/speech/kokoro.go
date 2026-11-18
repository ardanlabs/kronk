package speech

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speech/normalization"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// KokoroContextWindow is the padded encoder-segment capacity of Kokoro-82M.
const KokoroContextWindow = 512

// Kokoro owns English text normalization and phoneme-character admission.
// The renderer uses the loaded model's TTS template and dictionaries.
// Requests contain one segment of at most 510 mapped phoneme tokens, with
// two BOS pads. The noncausal encoder has no persistent KV cache or frame
// budget; resolved MaxFrames must remain at its default 512. Only English
// a*/b* voices are accepted. Empty language follows the voice's US/GB dialect;
// en/en-us selects US and en-gb selects GB. Recordings are not supported.
type Kokoro struct {
	renderer *normalization.Renderer
	tokens   map[rune]bool
}

// NewKokoro validates Kokoro's fixed encoder/frame policy and borrows its TTS
// dictionaries through the supplied native metadata accessors. The model must
// outlive the adapter and every execution created from it.
func NewKokoro(mdl llama.Model, maxFrames int, count func(llama.Model, string) (int, error), value func(llama.Model, string, int32) (string, bool)) (*Kokoro, error) {
	if maxFrames != 512 {
		return nil, fmt.Errorf("%w: Kokoro is non-autoregressive and does not support MaxFrames overrides", ErrInvalidConfig)
	}
	if llama.ModelNCtxTrain(mdl) != KokoroContextWindow {
		return nil, fmt.Errorf("Kokoro requires a %d-position encoder", KokoroContextWindow)
	}
	renderer, err := newKokoroNormalizer(mdl, count, value)
	if err != nil {
		return nil, err
	}
	vocab := llama.ModelGetVocab(mdl)
	k := Kokoro{renderer: renderer, tokens: make(map[rune]bool)}
	for token := range llama.VocabNTokens(vocab) {
		id := llama.Token(token)
		if llama.VocabGetAttr(vocab, id)&llama.TokenAttrNormal == 0 {
			continue
		}
		text := llama.VocabGetText(vocab, id)
		if utf8.RuneCountInString(text) == 1 {
			r, _ := utf8.DecodeRuneInString(text)
			k.tokens[r] = true
		}
	}
	return &k, nil
}

// NewExecution creates Kokoro's whole-segment encoder/vocoder protocol.
func (*Kokoro) NewExecution(_ context.Context, _ applog.Logger, cfg ExecutionConfig) (Execution, error) {
	return NewNonAutoregressive(cfg.Context, cfg.Decoder)
}

// Validate accepts English dialect selection and rejects reference transcripts.
// Non-English text-to-phoneme conversion is not provided by the model template.
func (*Kokoro) Validate(language, referenceText string) (string, error) {
	if referenceText != "" {
		return language, errors.New("Kokoro does not support reference recordings or transcripts")
	}
	language = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(language)), "_", "-")
	switch language {
	case "", "en", "en-us", "en-gb":
		return language, nil
	case "english":
		return "en", nil
	default:
		return language, fmt.Errorf("unsupported Kokoro speech language %q: use en-us or en-gb", language)
	}
}

// Prepare renders English into the model's phonemes and admits one bounded
// segment. Both BOS padding tokens count toward the 512-position capacity.
func (k *Kokoro) Prepare(ctx context.Context, _ llama.Vocab, prompt Prompt, input *mtmd.HelperGenAudioInput) (int, error) {
	if !strings.HasPrefix(prompt.Voice, "a") && !strings.HasPrefix(prompt.Voice, "b") {
		return 0, fmt.Errorf("Kokoro requires an English voice (a* or b*), got %q", prompt.Voice)
	}
	phonemes, err := k.renderer.Render(ctx, prompt.Text, prompt.Language, prompt.Voice)
	if err != nil {
		return 0, fmt.Errorf("Kokoro text normalization: %w", err)
	}
	var count int
	for _, r := range phonemes {
		if k.tokens[r] {
			count++
		}
	}
	if count == 0 || count > KokoroContextWindow-2 {
		return 0, fmt.Errorf("Kokoro requires 1 through %d phoneme tokens after normalization, got %d", KokoroContextWindow-2, count)
	}
	input.SetPrompt(phonemes)
	return count, nil
}

// newKokoroNormalizer borrows dictionary storage without loading the lexicons
// into Go maps. The engine serializes lookups and frees helpers before mdl.
func newKokoroNormalizer(mdl llama.Model, count func(llama.Model, string) (int, error), value func(llama.Model, string, int32) (string, bool)) (*normalization.Renderer, error) {
	template := llama.ModelChatTemplate(mdl, "tts")
	if template == "" {
		return nil, errors.New("missing TTS normalization template")
	}
	n, err := count(mdl, "tokenizer.tts_dicts")
	if err != nil {
		return nil, err
	}
	if n < 1 {
		return nil, errors.New("missing TTS dictionaries")
	}
	names := make([]string, 0, n)
	sizes := make(map[string]int, n)
	for i := range n {
		name, ok := value(mdl, "tokenizer.tts_dicts", int32(i))
		if !ok {
			return nil, fmt.Errorf("missing TTS dictionary name %d", i)
		}
		prefix := "tokenizer.tts_dict." + name
		keys, err := count(mdl, prefix+".keys")
		if err != nil {
			return nil, err
		}
		values, err := count(mdl, prefix+".values")
		if err != nil {
			return nil, err
		}
		if keys < 0 || keys != values {
			return nil, fmt.Errorf("mismatched TTS dictionary %q", name)
		}
		names = append(names, name)
		sizes[name] = keys
	}
	renderer, err := normalization.New(template, names, func(name, key string) (string, bool) {
		prefix := "tokenizer.tts_dict." + name
		low, high := 0, sizes[name]
		for low < high {
			mid := low + (high-low)/2
			candidate, ok := value(mdl, prefix+".keys", int32(mid))
			if !ok {
				return "", false
			}
			switch {
			case candidate < key:
				low = mid + 1
			case candidate > key:
				high = mid
			default:
				return value(mdl, prefix+".values", int32(mid))
			}
		}
		return "", false
	})
	if err != nil {
		return nil, fmt.Errorf("TTS normalization: %w", err)
	}
	return renderer, nil
}
