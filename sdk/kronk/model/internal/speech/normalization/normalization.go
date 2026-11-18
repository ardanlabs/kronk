// Package normalization renders model-provided TTS templates with the llama.cpp
// b12503 normalization pattern and dictionary builtins. It owns no native state.
//
// The pattern implementation is derived from llama.cpp commit
// 4577932fab595eaa8b27ad71734d5de1f3c07d8c: scripts/chat_tts_norm.py,
// common/chat-tts-norm.cpp and docs/tts-norm-patterns.md. See LICENSE.
package normalization

import (
	"context"
	"errors"
	"fmt"

	"github.com/ardanlabs/jinja"
)

// Renderer executes an unchanged model template. Concurrent renders are isolated;
// the caller's dictionary lookup must also support concurrent use.
type Renderer struct {
	template *jinja.Template
	names    map[string]struct{}
	lookup   func(name, key string) (string, bool)
}

// New compiles a model's tokenizer.chat_template.tts and registers its dictionary
// names. Lookup returns a single JSON document without materializing dictionaries.
func New(template string, names []string, lookup func(name, key string) (string, bool)) (*Renderer, error) {
	if lookup == nil && len(names) != 0 {
		return nil, errors.New("normalization: dictionary lookup is required")
	}
	known := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return nil, errors.New("normalization: empty dictionary name")
		}
		if _, exists := known[name]; exists {
			return nil, fmt.Errorf("normalization: duplicate dictionary name %q", name)
		}
		known[name] = struct{}{}
	}
	t, err := jinja.Compile(template)
	if err != nil {
		return nil, fmt.Errorf("normalization: compiling template: %w", err)
	}
	r := Renderer{template: t, names: known, lookup: lookup}
	return &r, nil
}

// Render normalizes text using the model template and the requested language and
// voice. An optional G2P callback supplies unknown-word pronunciations; nil
// reports no pronunciation so the template can use its spelling fallback.
// Cancellation is checked before and after rendering and within builtins;
// Jinja itself does not expose cancellation of template-only work.
func (r *Renderer) Render(ctx context.Context, text, language, voice string, g2p func(string) (string, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	values := map[string]jinja.Value{
		"messages": jinja.FromGoValue([]map[string]string{{"role": "user", "content": text}}),
		"language": jinja.NewString(language),
		"voice":    jinja.NewString(voice),
	}
	values["tts_g2p"] = jinja.NewCallable("tts_g2p", func(args []jinja.Value, kwargs map[string]jinja.Value) (jinja.Value, error) {
		if err := arguments(ctx, "tts_g2p", args, kwargs, 1); err != nil {
			return jinja.None(), err
		}
		if !args[0].IsString() {
			return jinja.None(), errors.New("tts_g2p: word must be a string")
		}
		if g2p == nil {
			return jinja.NewString(""), nil
		}
		phones, err := g2p(args[0].AsString())
		if err != nil {
			return jinja.None(), fmt.Errorf("tts_g2p: %w", err)
		}
		return jinja.NewString(phones), nil
	})
	values["tts_norm_scan"] = jinja.NewCallable("tts_norm_scan", func(args []jinja.Value, kwargs map[string]jinja.Value) (jinja.Value, error) {
		if err := arguments(ctx, "tts_norm_scan", args, kwargs, 2); err != nil {
			return jinja.None(), err
		}
		if !args[0].IsString() {
			return jinja.None(), errors.New("tts_norm_scan: text must be a string")
		}
		return normScan(ctx, args[0].AsString(), args[1])
	})
	values["tts_norm_sub"] = jinja.NewCallable("tts_norm_sub", func(args []jinja.Value, kwargs map[string]jinja.Value) (jinja.Value, error) {
		if err := arguments(ctx, "tts_norm_sub", args, kwargs, 3); err != nil {
			return jinja.None(), err
		}
		for _, arg := range args {
			if !arg.IsString() {
				return jinja.None(), errors.New("tts_norm_sub: arguments must be strings")
			}
		}
		return normSub(ctx, args[0].AsString(), args[1].AsString(), args[2].AsString())
	})
	values["tts_dict"] = jinja.NewCallable("tts_dict", func(args []jinja.Value, kwargs map[string]jinja.Value) (jinja.Value, error) {
		if err := arguments(ctx, "tts_dict", args, kwargs, 2); err != nil {
			return jinja.None(), err
		}
		if !args[0].IsString() || !args[1].IsString() {
			return jinja.None(), errors.New("tts_dict: name and key must be strings")
		}
		name := args[0].AsString()
		if _, ok := r.names[name]; !ok {
			return jinja.None(), fmt.Errorf("tts_dict: unknown dictionary %q", name)
		}
		doc, ok := r.lookup(name, args[1].AsString())
		if err := ctx.Err(); err != nil {
			return jinja.None(), err
		}
		if !ok {
			return jinja.None(), nil
		}
		value, err := decodeDictionary(ctx, doc)
		if err != nil {
			return jinja.None(), fmt.Errorf("tts_dict: decoding dictionary %q: %w", name, err)
		}
		if err := ctx.Err(); err != nil {
			return jinja.None(), err
		}
		return value, nil
	})
	out, err := r.template.RenderValues(values)
	if errCtx := ctx.Err(); errCtx != nil {
		return "", errCtx
	}
	if err != nil {
		return "", fmt.Errorf("normalization: rendering template: %w", err)
	}
	return out, nil
}

func arguments(ctx context.Context, name string, args []jinja.Value, kwargs map[string]jinja.Value, count int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) != count || len(kwargs) != 0 {
		return fmt.Errorf("%s: expected %d positional arguments", name, count)
	}
	return nil
}
