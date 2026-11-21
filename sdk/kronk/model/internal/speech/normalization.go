package speech

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speech/normalization"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// newTTSNormalizer borrows dictionary storage without loading the lexicons
// into Go maps. The engine serializes lookups and frees helpers before mdl.
func newTTSNormalizer(mdl llama.Model) (*normalization.Renderer, error) {
	template := llama.ModelChatTemplate(mdl, "tts")
	if template == "" {
		return nil, errors.New("missing TTS normalization template")
	}
	n, err := llama.ModelMetaArrN(mdl, "tokenizer.tts_dicts")
	if err != nil {
		return nil, err
	}
	if n < 1 {
		return nil, errors.New("missing TTS dictionaries")
	}
	names := make([]string, 0, n)
	sizes := make(map[string]int32, n)
	for i := range n {
		name, ok, err := llama.ModelMetaArrStr(mdl, "tokenizer.tts_dicts", i)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("missing TTS dictionary name %d", i)
		}
		prefix := "tokenizer.tts_dict." + name
		keys, err := llama.ModelMetaArrN(mdl, prefix+".keys")
		if err != nil {
			return nil, err
		}
		values, err := llama.ModelMetaArrN(mdl, prefix+".values")
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
		low, high := int32(0), sizes[name]
		for low < high {
			mid := low + (high-low)/2
			candidate, ok, err := llama.ModelMetaArrStr(mdl, prefix+".keys", mid)
			if err != nil || !ok {
				return "", false
			}
			switch {
			case candidate < key:
				low = mid + 1
			case candidate > key:
				high = mid
			default:
				value, ok, err := llama.ModelMetaArrStr(mdl, prefix+".values", mid)
				return value, ok && err == nil
			}
		}
		return "", false
	})
	if err != nil {
		return nil, fmt.Errorf("TTS normalization: %w", err)
	}
	return renderer, nil
}
