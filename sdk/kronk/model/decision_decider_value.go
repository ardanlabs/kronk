package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// decisionDeciderText follows llama-server's six-significant-digit JSON
// formatting, retaining ordered input except for decision2's sorted JSON.
func decisionDeciderText(value any, indent, sorted bool) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := marshalDecisionJSON(value)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	text, err := decisionDeciderJSON(decoder, sorted)
	if err != nil {
		return "", err
	}
	if indent {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return text, nil
}

func decisionDeciderJSON(decoder *json.Decoder, sorted bool) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	switch token := token.(type) {
	case json.Number:
		text := string(token)
		if strings.ContainsAny(text, ".eE") {
			number, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return "", err
			}
			text = strconv.FormatFloat(number, 'g', 6, 64)
		}
		return text, nil
	case json.Delim:
		var fields []struct{ key, text string }
		for decoder.More() {
			key := ""
			if token == '{' {
				name, err := decoder.Token()
				if err != nil {
					return "", err
				}
				key = name.(string)
			}
			text, err := decisionDeciderJSON(decoder, sorted)
			if err != nil {
				return "", err
			}
			fields = append(fields, struct{ key, text string }{key, text})
		}
		if _, err := decoder.Token(); err != nil {
			return "", err
		}
		if sorted && token == '{' {
			slices.SortFunc(fields, func(a, b struct{ key, text string }) int { return strings.Compare(a.key, b.key) })
		}
		parts := make([]string, len(fields))
		colon, comma := ": ", ", "
		if sorted {
			colon, comma = ":", ","
		}
		for i, field := range fields {
			parts[i] = field.text
			if token == '{' {
				key, err := decisionJSON(field.key)
				if err != nil {
					return "", err
				}
				parts[i] = key + colon + field.text
			}
		}
		if token == '{' {
			return "{" + strings.Join(parts, comma) + "}", nil
		}
		if token == '[' {
			return "[" + strings.Join(parts, comma) + "]", nil
		}
		return "", fmt.Errorf("unexpected JSON delimiter %q", token)
	default:
		return decisionJSON(token)
	}
}
