package normalization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/jinja"
)

func decodeDictionary(ctx context.Context, doc string) (jinja.Value, error) {
	if !json.Valid([]byte(doc)) {
		return jinja.None(), errors.New("invalid JSON document")
	}
	decoder := json.NewDecoder(strings.NewReader(doc))
	decoder.UseNumber()
	return dictionaryValue(ctx, decoder)
}

// Decode directly into ordered Jinja dictionaries. Going through Go maps would
// reorder keys, changing template iteration and tie-breaking in LTS beam search.
func dictionaryValue(ctx context.Context, decoder *json.Decoder) (jinja.Value, error) {
	if err := ctx.Err(); err != nil {
		return jinja.None(), err
	}
	token, err := decoder.Token()
	if err != nil {
		return jinja.None(), err
	}
	delim, container := token.(json.Delim)
	if !container {
		return jinja.FromGoValue(token), nil
	}
	switch delim {
	case '{':
		out := jinja.NewDict()
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return jinja.None(), err
			}
			name, ok := key.(string)
			if !ok {
				return jinja.None(), errors.New("JSON object key must be a string")
			}
			value, err := dictionaryValue(ctx, decoder)
			if err != nil {
				return jinja.None(), err
			}
			out.AsDict().Set(name, value)
		}
		if _, err := decoder.Token(); err != nil {
			return jinja.None(), err
		}
		return out, nil
	case '[':
		var items []jinja.Value
		for decoder.More() {
			value, err := dictionaryValue(ctx, decoder)
			if err != nil {
				return jinja.None(), err
			}
			items = append(items, value)
		}
		if _, err := decoder.Token(); err != nil {
			return jinja.None(), err
		}
		return jinja.NewList(items), nil
	default:
		return jinja.None(), fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}
