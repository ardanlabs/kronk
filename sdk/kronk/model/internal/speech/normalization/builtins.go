package normalization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/jinja"
)

type rule struct {
	name string
	pat  *pattern
}

func rulesFromValue(ctx context.Context, value jinja.Value) ([]rule, error) {
	var rules []rule
	add := func(name string, src jinja.Value) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !src.IsString() {
			return errors.New("tts_norm_scan: rule pattern must be a string")
		}
		pat, err := compile(src.AsString())
		if err != nil {
			return err
		}
		rules = append(rules, rule{name, pat})
		return nil
	}
	switch {
	case value.IsString():
		if err := add("match", value); err != nil {
			return nil, err
		}
	case value.IsDict():
		d := value.AsDict()
		for _, key := range d.OrderedKeys() {
			if err := add(key, d.Data[key]); err != nil {
				return nil, err
			}
		}
	case value.IsList():
		for _, v := range value.AsList().Items {
			if !v.IsList() || v.AsList().Len() != 2 || !v.AsList().Get(0).IsString() || !v.AsList().Get(1).IsString() {
				return nil, errors.New("tts_norm_scan: each rule must be a [name, pattern] pair of strings")
			}
			if err := add(v.AsList().Get(0).AsString(), v.AsList().Get(1)); err != nil {
				return nil, err
			}
		}
	default:
		return nil, errors.New("tts_norm_scan: rules must be a string, a dict or a list of [name, pattern] pairs")
	}
	return rules, nil
}

func normScan(ctx context.Context, text string, value jinja.Value) (jinja.Value, error) {
	rules, err := rulesFromValue(ctx, value)
	if err != nil {
		return jinja.None(), err
	}
	patterns := make([]*pattern, len(rules))
	for i, r := range rules {
		patterns[i] = r.pat
	}
	segments, err := scan(ctx, text, patterns)
	if err != nil {
		return jinja.None(), err
	}
	out := make([]jinja.Value, 0, len(segments))
	for _, s := range segments {
		if err := ctx.Err(); err != nil {
			return jinja.None(), err
		}
		groups := make([]jinja.Value, len(s.groups))
		for i, g := range s.groups {
			v := ""
			if g.start >= 0 {
				v = text[g.start:g.end]
			}
			groups[i] = jinja.NewString(v)
		}
		d := jinja.NewDict()
		name := jinja.None()
		if s.rule >= 0 {
			name = jinja.NewString(rules[s.rule].name)
		}
		d.AsDict().Set("rule", name)
		d.AsDict().Set("text", jinja.NewString(text[s.start:s.end]))
		d.AsDict().Set("groups", jinja.NewList(groups))
		out = append(out, d)
	}
	return jinja.NewList(out), nil
}

type replacement struct {
	literal string
	group   int
}

func parseReplacement(ctx context.Context, src string, pat *pattern) ([]replacement, error) {
	var pieces []replacement
	var lit strings.Builder
	flush := func() {
		if lit.Len() != 0 {
			pieces = append(pieces, replacement{literal: lit.String(), group: -1})
			lit.Reset()
		}
	}
	for i := 0; i < len(src); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if src[i] != '%' {
			lit.WriteByte(src[i])
			continue
		}
		i++
		if i >= len(src) {
			return nil, errors.New("invalid TTS norm replacement: dangling '%'")
		}
		c := src[i]
		switch {
		case c == '%':
			lit.WriteByte('%')
		case c >= '0' && c <= '9':
			g := int(c - '0')
			if g > pat.caps {
				return nil, fmt.Errorf("invalid TTS norm replacement: group %%%c does not exist", c)
			}
			flush()
			pieces = append(pieces, replacement{group: g})
		default:
			return nil, fmt.Errorf("invalid TTS norm replacement: unknown escape '%%%c'", c)
		}
	}
	flush()
	return pieces, nil
}

func normSub(ctx context.Context, text, src, repl string) (jinja.Value, error) {
	pat, err := compile(src)
	if err != nil {
		return jinja.None(), err
	}
	pieces, err := parseReplacement(ctx, repl, pat)
	if err != nil {
		return jinja.None(), err
	}
	segments, err := scan(ctx, text, []*pattern{pat})
	if err != nil {
		return jinja.None(), err
	}
	var out strings.Builder
	for _, s := range segments {
		if err := ctx.Err(); err != nil {
			return jinja.None(), err
		}
		if s.rule < 0 {
			out.WriteString(text[s.start:s.end])
			continue
		}
		for _, piece := range pieces {
			if err := ctx.Err(); err != nil {
				return jinja.None(), err
			}
			switch {
			case piece.group < 0:
				out.WriteString(piece.literal)
			case piece.group == 0:
				out.WriteString(text[s.start:s.end])
			default:
				g := s.groups[piece.group-1]
				if g.start >= 0 {
					out.WriteString(text[g.start:g.end])
				}
			}
		}
	}
	return jinja.NewString(out.String()), nil
}
