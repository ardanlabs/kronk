package gpt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const messageMarker = "<|message|>"

// parseGPTToolCall parses a complete sequence of GPT-OSS tool-call frames.
func parseGPTToolCall(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	remaining := content
	var calls []model.ResponseToolCall

	for {
		remaining = trimASCIIWhitespace(remaining)
		if remaining == "" {
			if len(calls) == 0 {
				return failedToolCall(content, errors.New("no tool-call frames"))
			}
			return calls
		}

		if remaining[0] != '.' {
			return failedToolCall(content, errors.New("expected function-name prefix"))
		}
		remaining = remaining[1:]
		nameEnd := strings.IndexFunc(remaining, isASCIIWhitespace)
		if nameEnd <= 0 {
			return failedToolCall(content, errors.New("invalid function name"))
		}
		name := remaining[:nameEnd]
		if !safeFunctionName(name) {
			return failedToolCall(content, errors.New("invalid function name"))
		}

		remaining = trimASCIIWhitespace(remaining[nameEnd:])
		if !strings.HasPrefix(remaining, messageMarker) {
			return failedToolCall(content, errors.New("missing message marker"))
		}
		remaining = trimASCIIWhitespace(remaining[len(messageMarker):])
		end, err := strictJSONObjectEnd(remaining)
		if err != nil {
			return failedToolCall(content, err)
		}

		args, err := decodeJSONObject(remaining[:end])
		if err != nil {
			if log != nil {
				log(ctx, "json", "status", "unmarshal-failed", "error", err, "json", remaining[:end])
			}
			return failedToolCall(content, err)
		}
		calls = append(calls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
		remaining = remaining[end:]
	}
}

// parseJSONToolCall strictly parses a complete sequence of JSON tool-call objects.
func parseJSONToolCall(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	remaining := content
	var calls []model.ResponseToolCall
	for {
		remaining = trimASCIIWhitespace(remaining)
		if remaining == "" {
			if len(calls) == 0 {
				return failedToolCall(content, errors.New("no tool calls"))
			}
			return calls
		}

		end, err := strictJSONObjectEnd(remaining)
		if err != nil {
			return failedToolCall(content, err)
		}
		value, err := decodeUniqueJSON(remaining[:end])
		if err != nil {
			if log != nil {
				log(ctx, "json", "status", "unmarshal-failed", "error", err, "json", remaining[:end])
			}
			return failedToolCall(content, err)
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return failedToolCall(content, errors.New("tool call must be an object"))
		}
		name, ok := obj["name"].(string)
		if !ok || !safeFunctionName(strings.TrimPrefix(name, ".")) {
			return failedToolCall(content, errors.New("invalid function name"))
		}
		args, ok := obj["arguments"].(map[string]any)
		if !ok {
			return failedToolCall(content, errors.New("arguments must be an object"))
		}
		calls = append(calls, model.ResponseToolCall{ID: newToolCallID(), Type: "function", Function: model.ResponseToolCallFunction{Name: strings.TrimPrefix(name, "."), Arguments: args}})
		remaining = remaining[end:]
	}
}

func strictJSONObjectEnd(s string) (int, error) {
	if s == "" || s[0] != '{' {
		return 0, errors.New("expected JSON object")
	}
	depth, inString, escape := 0, false, false
	for i := range len(s) {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inString && c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
			if depth < 0 {
				return 0, errors.New("malformed JSON object")
			}
		}
	}
	return 0, errors.New("incomplete JSON object")
}

func decodeJSONObject(data string) (model.ToolCallArguments, error) {
	value, err := decodeUniqueJSON(data)
	if err != nil {
		return nil, err
	}
	args, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("arguments must be a JSON object")
	}
	return args, nil
}

func decodeUniqueJSON(data string) (any, error) {
	dec := json.NewDecoder(bytes.NewBufferString(data))
	dec.UseNumber()
	value, err := decodeUniqueValue(dec)
	if err != nil {
		return nil, err
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected token %v", tok)
		}
		return nil, err
	}
	return value, nil
}

func decodeUniqueValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		obj := make(map[string]any)
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			if _, exists := obj[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeUniqueValue(dec)
			if err != nil {
				return nil, err
			}
			obj[key] = value
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return obj, nil
	case '[':
		values := make([]any, 0)
		for dec.More() {
			value, err := decodeUniqueValue(dec)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return values, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

func safeFunctionName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, c := range []byte(name) {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func trimASCIIWhitespace(s string) string {
	return strings.Trim(s, " \t\r\n")
}

func isASCIIWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func failedToolCall(raw string, err error) []model.ResponseToolCall {
	return []model.ResponseToolCall{{ID: newToolCallID(), Type: "function", Status: 2, Raw: raw, Error: err.Error()}}
}

// findJSONObjectEnd returns the end of an object, or -1 when it is incomplete or invalid.
func findJSONObjectEnd(s string) int {
	end, err := strictJSONObjectEnd(s)
	if err != nil {
		return -1
	}
	return end
}

func newToolCallID() string {
	return "call_" + uuid.NewString()
}
