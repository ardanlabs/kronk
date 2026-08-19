package toolcall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// parseJSON parses a strictly whitespace-separated sequence of JSON tool calls.
func parseJSON(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	if strings.Trim(content, " \t\r\n") == "" {
		return []model.ResponseToolCall{failedToolCall(content, errors.New("tool call is empty"))}
	}

	remaining := strings.TrimLeft(content, " \t\r\n")
	var calls []model.ResponseToolCall
	for remaining != "" {
		if remaining[0] != '{' {
			return []model.ResponseToolCall{failedToolCall(content, errors.New("unexpected content outside tool call object"))}
		}
		end := findJSONObjectEnd(remaining)
		if end < 0 {
			return []model.ResponseToolCall{failedToolCall(content, errors.New("incomplete tool call object"))}
		}

		raw := remaining[:end]
		function, err := decodeFunction(raw)
		if err != nil {
			function, err = repairFunction(raw, err)
		}
		if err != nil {
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed", "format", "json", "error", err, "json", raw)
			}
			return []model.ResponseToolCall{failedToolCall(content, err)}
		}

		function.Name = strings.TrimPrefix(function.Name, ".")
		if function.Name == "" {
			return []model.ResponseToolCall{failedToolCall(content, errors.New("tool call name is empty"))}
		}
		calls = append(calls, model.ResponseToolCall{ID: newToolCallID(), Type: "function", Function: function})
		remaining = strings.TrimLeft(remaining[end:], " \t\r\n")
	}

	return calls
}

func failedToolCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{ID: newToolCallID(), Type: "function", Status: 2, Error: err.Error(), Raw: raw}
}

func repairFunction(raw string, original error) (model.ResponseToolCallFunction, error) {
	// Repair is only permitted for a physically complete object. In particular,
	// never synthesize missing framing at end of generation.
	if findJSONObjectEnd(raw) != len(raw) {
		return model.ResponseToolCallFunction{}, original
	}
	repaired, err := jsonrepair.Repair(raw)
	if err != nil || !onlyAddsEscapes(raw, repaired) {
		return model.ResponseToolCallFunction{}, original
	}
	return decodeFunction(repaired)
}

// onlyAddsEscapes permits the one repair this parser historically supported:
// inserting backslashes to preserve malformed quotes or escapes inside a JSON
// string. It rejects repairs that create names, values, delimiters, or fields.
func onlyAddsEscapes(raw, repaired string) bool {
	for rawPos, repairedPos := 0, 0; rawPos < len(raw) || repairedPos < len(repaired); {
		if rawPos < len(raw) && repairedPos < len(repaired) && raw[rawPos] == repaired[repairedPos] {
			rawPos++
			repairedPos++
			continue
		}
		if repairedPos < len(repaired) && repaired[repairedPos] == '\\' {
			repairedPos++
			continue
		}
		return false
	}
	return true
}

func decodeFunction(raw string) (model.ResponseToolCallFunction, error) {
	value, err := decodeUniqueJSON([]byte(raw))
	if err != nil {
		return model.ResponseToolCallFunction{}, err
	}
	envelope, ok := value.(map[string]any)
	if !ok {
		return model.ResponseToolCallFunction{}, errors.New("tool call must be an object")
	}
	name, ok := envelope["name"].(string)
	if !ok || name == "" {
		return model.ResponseToolCallFunction{}, errors.New("tool call name is empty")
	}
	argumentValue, ok := envelope["arguments"]
	if !ok {
		return model.ResponseToolCallFunction{}, errors.New("tool call arguments must be an object")
	}
	if encoded, ok := argumentValue.(string); ok {
		argumentValue, err = decodeUniqueJSON([]byte(encoded))
		if err != nil {
			return model.ResponseToolCallFunction{}, fmt.Errorf("invalid tool arguments JSON: %w", err)
		}
	}
	arguments, ok := argumentValue.(map[string]any)
	if !ok {
		return model.ResponseToolCallFunction{}, errors.New("tool call arguments must be an object")
	}

	return model.ResponseToolCallFunction{Name: name, Arguments: model.ToolCallArguments(arguments)}, nil
}

func decodeUniqueJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueValue(decoder)
	if err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	return value, nil
}

func decodeUniqueValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key := keyToken.(string)
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
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

// findJSONObjectEnd returns the byte immediately following a balanced object.
func findJSONObjectEnd(s string) int {
	if s == "" || s[0] != '{' {
		return -1
	}
	depth := 0
	inString := false
	escape := false
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
				return i + 1
			}
		}
	}
	return -1
}

func newToolCallID() string { return "call_" + uuid.New().String() }
