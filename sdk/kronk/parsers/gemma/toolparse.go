package gemma

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseGemma parses Gemma4-style tool calls.
// Format: call:get_weather{location:<|"|>New York City, NY<|"|>}
// Multiple calls may appear separated by newlines or back-to-back.
//
// Direct port of the legacy parseGemmaToolCall.
func parseGemma(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall

	remaining := content
	for {
		callIdx := strings.Index(remaining, "call:")
		if callIdx == -1 {
			break
		}

		remaining = remaining[callIdx+5:]

		braceIdx := strings.Index(remaining, "{")
		if braceIdx == -1 {
			break
		}

		name := strings.TrimSpace(remaining[:braceIdx])
		remaining = remaining[braceIdx:]

		braceEnd := findGemmaBraceEnd(remaining)

		var argsRaw string
		if braceEnd == -1 {
			argsRaw = remaining[1:]
			remaining = ""
		} else {
			argsRaw = remaining[1:braceEnd]
			remaining = remaining[braceEnd+1:]
		}

		var args map[string]any
		trimmed := strings.TrimSpace(argsRaw)

		jsonCandidate := trimmed
		if len(jsonCandidate) > 0 && jsonCandidate[0] != '{' {
			jsonCandidate = "{" + jsonCandidate + "}"
		}

		if err := unmarshalGemmaJSON(jsonCandidate, &args); err != nil {
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed",
					"format", "gemma", "error", err, "json", jsonCandidate)
			}

			inner := trimmed
			if len(inner) > 0 && inner[0] == '{' {
				inner = inner[1:]
				if idx := strings.LastIndex(inner, "}"); idx >= 0 {
					inner = inner[:idx]
				}
			}
			args = parseGemmaArgs(inner)
		}

		toolCalls = append(toolCalls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
	}

	return toolCalls
}

// findGemmaBraceEnd finds the closing brace that matches the opening brace
// at position 0, accounting for nested braces and Gemma's two quoting modes.
func findGemmaBraceEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		return -1
	}

	useJSONQuotes := !strings.Contains(s, "<|\"|>")

	depth := 0
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "<|\"|>") {
			i += len("<|\"|>")
			for i < len(s) {
				if strings.HasPrefix(s[i:], "<|\"|>") {
					i += len("<|\"|>")
					break
				}
				i++
			}
			continue
		}

		if useJSONQuotes && s[i] == '"' {
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}

		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}

	return -1
}

func findClosingGemmaQuote(s string) int {
	const token = "<|\"|>"
	searchFrom := 0

	for {
		idx := strings.Index(s[searchFrom:], token)
		if idx == -1 {
			return -1
		}

		pos := searchFrom + idx
		afterQuote := pos + len(token)

		if afterQuote >= len(s) {
			return pos
		}

		switch s[afterQuote] {
		case ',', '}', ']', '"':
			return pos
		}

		searchFrom = afterQuote
	}
}

func findGemmaStructEnd(s string) int {
	if len(s) == 0 {
		return -1
	}

	open := s[0]
	var close byte
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	default:
		return -1
	}

	depth := 0
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "<|\"|>") {
			i += len("<|\"|>")
			continue
		}

		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}

	return -1
}

func findClosingStandardQuote(s string) int {
	searchFrom := 0

	for {
		idx := strings.Index(s[searchFrom:], "\"")
		if idx == -1 {
			return -1
		}

		pos := searchFrom + idx

		if pos > 0 && s[pos-1] == '\\' {
			searchFrom = pos + 1
			continue
		}

		afterQuote := pos + 1

		if afterQuote >= len(s) {
			return pos
		}

		next := s[afterQuote]
		if next == ',' || next == '}' || next == ' ' || next == '\n' || next == '\r' || next == '\t' {
			return pos
		}

		searchFrom = afterQuote
	}
}

// parseGemmaArgs parses the key-value pairs inside a Gemma4 tool-call
// argument block. Values are delimited by <|"|> tokens (acting as quotes).
func parseGemmaArgs(raw string) map[string]any {
	args := make(map[string]any)

	remaining := raw
	for len(remaining) > 0 {
		colonIdx := strings.Index(remaining, ":")
		if colonIdx == -1 {
			break
		}

		key := strings.TrimLeft(remaining[:colonIdx], ", \t\n")
		key = strings.Trim(key, "\"")
		remaining = remaining[colonIdx+1:]

		if strings.HasPrefix(remaining, "<|\"|>") {
			remaining = remaining[len("<|\"|>"):]

			endQuote := findClosingGemmaQuote(remaining)
			if endQuote == -1 {
				args[key] = strings.TrimSpace(remaining)
				break
			}

			value := remaining[:endQuote]

			args[key] = value
			remaining = remaining[endQuote+len("<|\"|>"):]
			continue
		}

		if strings.HasPrefix(remaining, "\"") {
			remaining = remaining[1:]

			endQuote := findClosingStandardQuote(remaining)
			if endQuote == -1 {
				args[key] = strings.TrimSpace(remaining)
				break
			}

			args[key] = remaining[:endQuote]
			remaining = remaining[endQuote+1:]
			continue
		}

		if len(remaining) > 0 && (remaining[0] == '[' || remaining[0] == '{') {
			endIdx := findGemmaStructEnd(remaining)
			if endIdx == -1 {
				args[key] = strings.TrimSpace(remaining)
				break
			}

			raw := remaining[:endIdx]
			jsonVal := strings.ReplaceAll(raw, "<|\"|>", "\"")

			var parsed any
			if err := decodeGemmaJSONValue(jsonVal, &parsed); err == nil {
				args[key] = parsed
			} else {
				args[key] = raw
			}

			remaining = remaining[endIdx:]
			continue
		}

		endIdx := strings.IndexAny(remaining, ",}")
		var rawVal string
		if endIdx == -1 {
			rawVal = strings.TrimSpace(remaining)
		} else {
			rawVal = strings.TrimSpace(remaining[:endIdx])
		}

		args[key] = parseGemmaBareValue(rawVal)

		if endIdx == -1 {
			break
		}
		remaining = remaining[endIdx:]
	}

	return args
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

func unmarshalGemmaJSON(raw string, value any) error {
	repaired, err := jsonrepair.Repair(raw)
	if err != nil {
		return err
	}

	return decodeGemmaJSONValue(repaired, value)
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
	return "call_" + uuid.NewString()
}
