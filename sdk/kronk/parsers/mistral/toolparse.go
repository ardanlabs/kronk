package mistral

import (
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

const (
	toolCallsMarker = "[TOOL_CALLS]"
	argsMarker      = "[ARGS]"
	parseError      = 2
)

// parseMistral parses the complete Mistral native tool-call stream. The
// grammar is deliberately all-or-nothing: a malformed sibling invalidates
// the complete stream.
func parseMistral(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	calls, err := parseCalls(content)
	if err == nil {
		return calls
	}

	if log != nil {
		log(ctx, "tool-call", "status", "unmarshal-failed", "format", "mistral", "error", err, "json", content)
	}

	return []model.ResponseToolCall{{
		ID:     newToolCallID(),
		Type:   "function",
		Status: parseError,
		Raw:    content,
		Error:  err.Error(),
	}}
}

func parseCalls(content string) ([]model.ResponseToolCall, error) {
	cursor := skipASCIIWhitespace(content, 0)
	var calls []model.ResponseToolCall

	for cursor < len(content) {
		if !strings.HasPrefix(content[cursor:], toolCallsMarker) {
			return nil, errors.New("tool call marker is missing")
		}
		cursor += len(toolCallsMarker)

		argsOffset := strings.Index(content[cursor:], argsMarker)
		if argsOffset < 0 {
			return nil, errors.New("tool call arguments marker is missing")
		}
		name := strings.Trim(content[cursor:cursor+argsOffset], " \t\r\n")
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("tool call name is empty")
		}
		cursor += argsOffset + len(argsMarker)
		cursor = skipASCIIWhitespace(content, cursor)
		if cursor == len(content) || content[cursor] != '{' {
			return nil, errors.New("tool call arguments must be a JSON object")
		}

		end := findJSONObjectEnd(content[cursor:])
		if end < 0 {
			return nil, errors.New("tool call arguments are incomplete")
		}
		raw := content[cursor : cursor+end]
		arguments, err := decodeArguments(raw)
		if err != nil {
			return nil, fmt.Errorf("decoding arguments for %q: %w", name, err)
		}
		calls = append(calls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: model.ToolCallArguments(arguments),
			},
		})
		cursor = skipASCIIWhitespace(content, cursor+end)
	}

	if len(calls) == 0 {
		return nil, errors.New("tool call stream is empty")
	}
	return calls, nil
}

func decodeArguments(raw string) (map[string]any, error) {
	value, err := decodeUniqueJSON(raw)
	if err != nil {
		return nil, err
	}

	arguments, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("arguments are not a JSON object")
	}
	return arguments, nil
}

func decodeUniqueJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueValue(decoder)
	if err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected JSON tail %v", token)
	}
	return value, nil
}

func decodeUniqueValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeUniqueValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil

	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeUniqueValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return array, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

func findJSONObjectEnd(content string) int {
	if len(content) == 0 || content[0] != '{' {
		return -1
	}

	depth := 0
	inString := false
	escaped := false
	for i := range len(content) {
		char := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
			if depth < 0 {
				return -1
			}
		}
	}
	return -1
}

func skipASCIIWhitespace(content string, cursor int) int {
	for cursor < len(content) {
		switch content[cursor] {
		case ' ', '\t', '\r', '\n':
			cursor++
		default:
			return cursor
		}
	}
	return cursor
}

func newToolCallID() string {
	return "call_" + uuid.NewString()
}
