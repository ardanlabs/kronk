package gemma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// parseGemma parses Gemma4-style tool calls.
// Format: call:get_weather{location:<|"|>New York City, NY<|"|>}
// Multiple calls may appear separated by newlines or back-to-back.
func parseGemma(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall
	raw := content

	remaining := strings.TrimSpace(content)
	if remaining == "" {
		return []model.ResponseToolCall{failedGemmaToolCall(raw, errors.New("parse gemma: tool call is empty"))}
	}

	for remaining != "" {
		if !strings.HasPrefix(remaining, "call:") {
			return []model.ResponseToolCall{failedGemmaToolCall(raw,
				errors.New("parse gemma: unexpected content outside call"))}
		}

		remaining = remaining[len("call:"):]

		braceIdx := strings.Index(remaining, "{")
		if braceIdx == -1 {
			return []model.ResponseToolCall{failedGemmaToolCall(raw,
				errors.New("parse gemma: call argument block is missing"))}
		}

		name := strings.TrimSpace(remaining[:braceIdx])
		if name == "" {
			return []model.ResponseToolCall{failedGemmaToolCall(raw,
				errors.New("parse gemma: call name is empty"))}
		}
		remaining = remaining[braceIdx:]

		braceEnd := findGemmaBraceEnd(remaining)
		if braceEnd == -1 {
			return []model.ResponseToolCall{failedGemmaToolCall(raw,
				fmt.Errorf("parse gemma: call %q argument block is not closed", name))}
		}
		argsRaw := remaining[1:braceEnd]
		remaining = remaining[braceEnd+1:]

		trimmed := strings.TrimSpace(argsRaw)
		jsonCandidate := trimmed
		if len(jsonCandidate) > 0 && jsonCandidate[0] != '{' {
			jsonCandidate = "{" + jsonCandidate + "}"
		}

		args, err := decodeRepairedGemmaObject(jsonCandidate)
		if err != nil {
			if errors.Is(err, errDuplicateGemmaKey) {
				return []model.ResponseToolCall{failedGemmaToolCall(raw,
					fmt.Errorf("parse gemma: call %q arguments: %w", name, err))}
			}
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed",
					"format", "gemma", "error", err, "json", jsonCandidate)
			}

			args, err = parseGemmaArgs(trimmed)
			if err != nil {
				return []model.ResponseToolCall{failedGemmaToolCall(raw,
					fmt.Errorf("parse gemma: call %q arguments: %w", name, err))}
			}
		}

		toolCalls = append(toolCalls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		remaining = strings.TrimSpace(remaining)
	}

	return toolCalls
}

var errDuplicateGemmaKey = errors.New("duplicate JSON key")

func failedGemmaToolCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:     newToolCallID(),
		Type:   "function",
		Status: 2,
		Raw:    raw,
		Error:  err.Error(),
	}
}

// findGemmaBraceEnd finds the closing brace that matches the opening brace
// at position 0, accounting for nested braces and Gemma's two quoting modes.
func findGemmaBraceEnd(s string) int {
	end := findGemmaDelimitedEnd(s)
	if end == -1 || s[0] != '{' {
		return -1
	}

	return end - 1
}

func findClosingGemmaQuote(s string) int {
	return strings.Index(s, "<|\"|>")
}

func findGemmaStructEnd(s string) int {
	return findGemmaDelimitedEnd(s)
}

func findClosingStandardQuote(s string) int {
	escaped := false
	for i := range len(s) {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == '"' {
			return i
		}
	}

	return -1
}

func findGemmaDelimitedEnd(s string) int {
	if len(s) == 0 || (s[0] != '{' && s[0] != '[') {
		return -1
	}

	close := byte('}')
	if s[0] == '[' {
		close = ']'
	}
	stack := []byte{close}
	for i := 1; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "<|\"|>"):
			quoteEnd := findClosingGemmaQuote(s[i+len("<|\"|>"):])
			if quoteEnd == -1 {
				return -1
			}
			i += len("<|\"|>") + quoteEnd + len("<|\"|>")

		case s[i] == '"':
			quoteEnd := findClosingStandardQuote(s[i+1:])
			if quoteEnd == -1 {
				return -1
			}
			i += quoteEnd + 2

		case s[i] == '{':
			stack = append(stack, '}')
			i++

		case s[i] == '[':
			stack = append(stack, ']')
			i++

		case s[i] == '}' || s[i] == ']':
			if len(stack) == 0 || s[i] != stack[len(stack)-1] {
				return -1
			}
			stack = stack[:len(stack)-1]
			i++
			if len(stack) == 0 {
				return i
			}

		default:
			i++
		}
	}

	return -1
}

// parseGemmaArgs parses the key-value pairs inside a Gemma4 tool-call
// argument block. Values are delimited by <|"|> tokens (acting as quotes).
func parseGemmaArgs(raw string) (map[string]any, error) {
	args := make(map[string]any)

	remaining := strings.TrimSpace(raw)
	for len(remaining) > 0 {
		colonIdx := strings.Index(remaining, ":")
		if colonIdx == -1 {
			return nil, errors.New("argument is missing a colon")
		}

		key := strings.Trim(strings.TrimSpace(remaining[:colonIdx]), "\"")
		if key == "" {
			return nil, errors.New("argument name is empty")
		}
		if _, exists := args[key]; exists {
			return nil, fmt.Errorf("argument %q is duplicated", key)
		}
		remaining = strings.TrimLeft(remaining[colonIdx+1:], " \t\n\r")

		var value any
		var rest string

		if strings.HasPrefix(remaining, "<|\"|>") {
			remaining = remaining[len("<|\"|>"):]

			endQuote := findClosingGemmaQuote(remaining)
			if endQuote == -1 {
				return nil, fmt.Errorf("argument %q has an unterminated gemma quote", key)
			}

			value = remaining[:endQuote]
			rest = remaining[endQuote+len("<|\"|>"):]
		} else if strings.HasPrefix(remaining, "\"") {
			remaining = remaining[1:]

			endQuote := findClosingStandardQuote(remaining)
			if endQuote == -1 {
				return nil, fmt.Errorf("argument %q has an unterminated quote", key)
			}

			value = remaining[:endQuote]
			rest = remaining[endQuote+1:]
		} else if len(remaining) > 0 && (remaining[0] == '[' || remaining[0] == '{') {
			endIdx := findGemmaStructEnd(remaining)
			if endIdx == -1 {
				return nil, fmt.Errorf("argument %q has an unterminated composite value", key)
			}

			rawValue := remaining[:endIdx]
			jsonVal := strings.ReplaceAll(rawValue, "<|\"|>", "\"")

			parsed, err := decodeRepairedGemmaJSON(jsonVal)
			if err != nil {
				return nil, fmt.Errorf("argument %q has an invalid composite value: %w", key, err)
			}
			value = parsed

			rest = remaining[endIdx:]
		} else {
			endIdx := strings.IndexAny(remaining, ",}")
			var rawValue string
			if endIdx == -1 {
				rawValue = strings.TrimSpace(remaining)
				rest = ""
			} else {
				rawValue = strings.TrimSpace(remaining[:endIdx])
				rest = remaining[endIdx:]
			}
			if rawValue == "" {
				return nil, fmt.Errorf("argument %q has an empty value", key)
			}
			value = parseGemmaBareValue(rawValue)
		}

		args[key] = value

		remaining = strings.TrimSpace(rest)
		if remaining == "" {
			break
		}
		if remaining[0] != ',' {
			return nil, fmt.Errorf("unexpected content after argument %q", key)
		}
		remaining = strings.TrimSpace(remaining[1:])
		if remaining == "" {
			return nil, errors.New("argument list has a trailing comma")
		}
	}

	return args, nil
}

// parseGemmaBareValue converts a bare (unquoted) value string to its native
// JSON type. Numbers use json.Number so their original value and precision are
// retained. Everything else is returned as a string.
func parseGemmaBareValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}

	var value any
	if err := decodeGemmaJSONValue(s, &value); err == nil {
		if number, ok := value.(json.Number); ok {
			return number
		}
	}

	return s
}

func decodeRepairedGemmaObject(raw string) (map[string]any, error) {
	value, err := decodeRepairedGemmaJSON(raw)
	if err != nil {
		return nil, err
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("gemma arguments must be an object")
	}

	return object, nil
}

func decodeRepairedGemmaJSON(raw string) (any, error) {
	repaired, err := jsonrepair.Repair(raw)
	if err != nil {
		return nil, err
	}

	value, err := decodeUniqueGemmaJSON(repaired)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func decodeUniqueGemmaJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	value, err := decodeUniqueGemmaJSONValue(decoder)
	if err != nil {
		return nil, err
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("unexpected data after JSON value")
		}
		return nil, err
	}

	return value, nil
}

func decodeUniqueGemmaJSONValue(decoder *json.Decoder) (any, error) {
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
				return nil, fmt.Errorf("%w %q", errDuplicateGemmaKey, key)
			}

			value, err := decodeUniqueGemmaJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
			return nil, errors.New("JSON object is not closed")
		}
		return object, nil

	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeUniqueGemmaJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			return nil, errors.New("JSON array is not closed")
		}
		return array, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

func decodeGemmaJSONValue(raw string, value any) error {
	if !json.Valid([]byte(raw)) {
		return errors.New("invalid JSON value")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	return decoder.Decode(value)
}

func normalizeGemmaArguments(toolCalls []model.ResponseToolCall, tools []model.D) {
	for i := range toolCalls {
		properties := gemmaToolProperties(tools, toolCalls[i].Function.Name)
		if properties == nil {
			continue
		}

		for name, value := range toolCalls[i].Function.Arguments {
			property, ok := properties[name].(model.D)
			if !ok {
				continue
			}

			schemaType, ok := gemmaSchemaType(property["type"])
			if !ok {
				continue
			}

			if schemaType == "string" {
				switch value.(type) {
				case bool, json.Number, nil:
					if encoded, err := json.Marshal(value); err == nil {
						toolCalls[i].Function.Arguments[name] = string(encoded)
					}
				}
				continue
			}

			if _, ok := value.(string); ok {
				continue
			}

			switch schemaType {
			case "object":
				if _, ok := value.(map[string]any); ok {
					toolCalls[i].Function.Arguments[name] = value
				}

			case "array":
				if _, ok := value.([]any); ok {
					toolCalls[i].Function.Arguments[name] = value
				}

			case "number":
				if _, ok := value.(json.Number); ok {
					toolCalls[i].Function.Arguments[name] = value
				}

			case "integer":
				if number, ok := value.(json.Number); ok && gemmaJSONInteger(number) {
					toolCalls[i].Function.Arguments[name] = value
				}

			case "boolean":
				if _, ok := value.(bool); ok {
					toolCalls[i].Function.Arguments[name] = value
				}

			case "null":
				if value == nil {
					toolCalls[i].Function.Arguments[name] = nil
				}
			}
		}
	}
}

func gemmaToolProperties(tools []model.D, name string) model.D {
	var properties model.D

	for _, tool := range tools {
		if tool["type"] != "function" {
			continue
		}

		function, ok := tool["function"].(model.D)
		if !ok || function["name"] != name {
			continue
		}

		if properties != nil {
			return nil
		}

		parameters, ok := function["parameters"].(model.D)
		if !ok {
			return nil
		}

		properties, ok = parameters["properties"].(model.D)
		if !ok {
			return nil
		}
	}

	return properties
}

func gemmaSchemaType(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return value, true

	case []any:
		if len(value) == 1 {
			schemaType, ok := value[0].(string)
			return schemaType, ok
		}

	case []string:
		if len(value) == 1 {
			return value[0], true
		}
	}

	return "", false
}

func gemmaJSONInteger(number json.Number) bool {
	value, ok := new(big.Rat).SetString(number.String())
	return ok && value.IsInt()
}

func newToolCallID() string {
	return "call_" + uuid.New().String()
}
