package beam

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func parseHeader(header string) (string, model.Channel, error) {
	role, channel, ok := strings.Cut(header, "<|channel|>")
	if !ok {
		return "", model.ChannelAnswer, errors.New("beam: missing channel")
	}
	if name, ok := strings.CutPrefix(role, "assistant to="); ok {
		if name == "" || strings.ContainsAny(name, " \t\r\n<>\"") || strings.TrimSpace(channel) != "analysis <|constrain|>xml" {
			return "", model.ChannelTool, errors.New("beam: invalid tool header")
		}
		return name, model.ChannelTool, nil
	}
	if role != "assistant" {
		return "", model.ChannelNone, errors.New("beam: expected assistant")
	}
	channel, constraint, constrained := strings.Cut(channel, "<|constrain|>")
	switch strings.TrimSpace(channel) {
	case "analysis":
		if !constrained {
			return "", model.ChannelReasoning, nil
		}
	case "final", "final json":
		if !constrained || strings.TrimSpace(constraint) == "json" {
			return "", model.ChannelAnswer, nil
		}
	}
	return "", model.ChannelNone, errors.New("beam: invalid channel")
}

func parseToolCalls(raw string, tools []model.D) []model.ResponseToolCall {
	fail := func(err error) []model.ResponseToolCall {
		return []model.ResponseToolCall{{ID: "call_" + uuid.New().String(), Type: "function", Status: 2, Raw: raw, Error: err.Error()}}
	}
	remaining := strings.TrimSpace(raw)
	var calls []model.ResponseToolCall
	for remaining != "" {
		if !strings.HasPrefix(remaining, startMarker) {
			return fail(errors.New("beam: expected assistant tool frame"))
		}
		header, body, ok := strings.Cut(remaining[len(startMarker):], messageMarker)
		if !ok {
			return fail(errors.New("beam: missing tool body"))
		}
		name, channel, err := parseHeader(header)
		if err != nil || channel != model.ChannelTool {
			return fail(errors.New("beam: invalid tool header"))
		}
		end, marker := toolBoundary(body)
		if end < 0 {
			return fail(errors.New("beam: unterminated tool call"))
		}
		args := make(model.ToolCallArguments)
		// Retain the boundary while reading parameters so a missing final
		// parameter closer can terminate at its newline-delimited frame end.
		parameters := strings.TrimLeft(body[:end]+marker, " \t\r\n")
		for parameters != marker {
			key, value, tail, err := splitParameter(parameters)
			if err != nil {
				return fail(err)
			}
			if _, exists := args[key]; exists {
				return fail(fmt.Errorf("beam: duplicate parameter %q", key))
			}
			args[key], err = parameterValue(value, tools, name, key)
			if err != nil {
				return fail(err)
			}
			parameters = strings.TrimLeft(tail, " \t\r\n")
		}
		calls = append(calls, model.ResponseToolCall{
			ID: "call_" + uuid.New().String(), Type: "function",
			Function: model.ResponseToolCallFunction{Name: name, Arguments: args},
		})
		remaining = body[end:]
		if marker != startMarker {
			remaining = remaining[len(marker):]
		}
		remaining = strings.TrimSpace(remaining)
		if marker != startMarker && marker != "<|end|>" && remaining != "" {
			return fail(errors.New("beam: content after end of generation"))
		}
	}
	if len(calls) == 0 {
		return fail(errors.New("beam: no tool calls"))
	}
	return calls
}

// splitParameter removes only the wire-format newlines, not value whitespace.
// This is not general XML: bodies are raw strings, including '<' and '&'.
func splitParameter(s string) (string, string, string, error) {
	if !strings.HasPrefix(s, parameterOpen) {
		return "", "", "", errors.New("beam: expected parameter")
	}
	key, value, ok := strings.Cut(s[len(parameterOpen):], "\">\n")
	if !ok || key == "" || strings.ContainsAny(key, "\"\n<>") {
		return "", "", "", errors.New("beam: invalid parameter opener")
	}
	end := strings.Index(value, parameterEnd)
	delimiter := parameterEnd
	for _, candidate := range append([]string{parameterOpen}, frameMarkers...) {
		if at := strings.Index(value, "\n"+candidate); at >= 0 && (end < 0 || at < end) {
			end, delimiter = at, "\n"
		}
	}
	if end < 0 {
		return "", "", "", errors.New("beam: unterminated parameter")
	}
	return key, value[:end], value[end+len(delimiter):], nil
}

// parameterValue preserves strings verbatim. Nonstring schemas use JSON, and
// unions try a matching JSON value before falling back to a declared string.
func parameterValue(raw string, tools []model.D, name, key string) (any, error) {
	var schema model.D
	for _, tool := range tools {
		function, _ := tool["function"].(model.D)
		if function["name"] == name {
			parameters, _ := function["parameters"].(model.D)
			properties, _ := parameters["properties"].(model.D)
			schema, _ = properties[key].(model.D)
			break
		}
	}
	types := parameterTypes(schema)
	if len(types) == 0 || len(types) == 1 && types[0] == "string" {
		return raw, nil
	}
	var value any
	if json.Valid([]byte(raw)) {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("beam: decode parameter %q: %w", key, err)
		}
		for _, ty := range types {
			switch v := value.(type) {
			case map[string]any:
				if ty == "object" {
					return value, nil
				}
			case []any:
				if ty == "array" {
					return value, nil
				}
			case json.Number:
				if ty == "number" {
					return value, nil
				}
				if n, ok := new(big.Rat).SetString(v.String()); ty == "integer" && ok && n.IsInt() {
					return value, nil
				}
			case bool:
				if ty == "boolean" {
					return value, nil
				}
			case nil:
				if ty == "null" {
					return nil, nil
				}
			}
		}
	}
	if slices.Contains(types, "string") {
		return raw, nil
	}
	return nil, fmt.Errorf("beam: parameter %q does not match its declared type", key)
}

func parameterTypes(schema model.D) []string {
	var types []string
	switch value := schema["type"].(type) {
	case string:
		types = append(types, value)
	case []string:
		types = append(types, value...)
	case []any:
		for _, item := range value {
			if ty, ok := item.(string); ok {
				types = append(types, ty)
			}
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		switch variants := schema[key].(type) {
		case []model.D:
			for _, property := range variants {
				types = append(types, parameterTypes(property)...)
			}
		case []any:
			for _, variant := range variants {
				if property, ok := variant.(model.D); ok {
					types = append(types, parameterTypes(property)...)
				}
			}
		}
	}
	return types
}
