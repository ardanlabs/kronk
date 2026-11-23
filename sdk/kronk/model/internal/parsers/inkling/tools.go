package inkling

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

func parseToolCalls(raw string) []parsers.ResponseToolCall {
	fail := func(err error) []parsers.ResponseToolCall {
		return []parsers.ResponseToolCall{{ID: "call_" + uuid.New().String(), Type: "function", Status: 2, Raw: raw, Error: err.Error()}}
	}
	remaining := strings.TrimSpace(raw)
	var calls []parsers.ResponseToolCall
	for remaining != "" {
		frame, ok := strings.CutPrefix(remaining, messageModel)
		if !ok {
			return fail(errors.New("inkling: expected model tool frame"))
		}
		name, body, ok := strings.Cut(frame, invoke)
		if !ok || strings.ContainsAny(name, " \t\r\n<>\"") {
			return fail(errors.New("inkling: invalid tool header"))
		}
		name, args, tail, err := decodeCall(name, body)
		if err != nil {
			return fail(err)
		}
		remaining, ok = strings.CutPrefix(strings.TrimLeft(tail, " \t\r\n"), endMessage)
		if !ok {
			return fail(errors.New("inkling: missing tool message terminator"))
		}
		calls = append(calls, parsers.ResponseToolCall{
			ID: "call_" + uuid.New().String(), Index: len(calls), Type: "function",
			Function: parsers.ResponseToolCallFunction{Name: name, Arguments: args},
		})
		remaining = strings.TrimSpace(remaining)
		if tail, ok := strings.CutPrefix(remaining, endSampling); ok {
			if strings.TrimSpace(tail) != "" {
				return fail(errors.New("inkling: content after end of generation"))
			}
			remaining = ""
		}
	}
	if len(calls) == 0 {
		return fail(errors.New("inkling: no tool calls"))
	}
	return calls
}

// decodeCall consumes exactly one JSON envelope. Decoding before inspecting
// framing preserves literal protocol markers and escaped quotes in arguments.
// Duplicate envelope fields are rejected rather than silently taking the last.
// A supplied header name must match; otherwise the JSON name supplies it.
func decodeCall(headerName, body string) (string, parsers.ToolCallArguments, string, error) {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	open, err := dec.Token()
	if err != nil || open != json.Delim('{') {
		return "", nil, "", errors.New("inkling: expected tool JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return "", nil, "", fmt.Errorf("inkling: reading tool field: %w", err)
		}
		key, ok := token.(string)
		if !ok || (key != "name" && key != "args") {
			return "", nil, "", errors.New("inkling: unexpected tool field")
		}
		if _, exists := fields[key]; exists {
			return "", nil, "", fmt.Errorf("inkling: duplicate tool field %q", key)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return "", nil, "", fmt.Errorf("inkling: decoding tool field %q: %w", key, err)
		}
		fields[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return "", nil, "", fmt.Errorf("inkling: closing tool JSON: %w", err)
	}
	var jsonName string
	if err := json.Unmarshal(fields["name"], &jsonName); err != nil || jsonName == "" || strings.ContainsAny(jsonName, " \t\r\n<>\"") {
		return "", nil, "", errors.New("inkling: invalid tool name")
	}
	if headerName != "" && jsonName != headerName {
		return "", nil, "", errors.New("inkling: tool name must match header")
	}
	argsJSON := fields["args"]
	if len(argsJSON) == 0 || argsJSON[0] != '{' {
		return "", nil, "", errors.New("inkling: args must be an object")
	}
	var args parsers.ToolCallArguments
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", nil, "", fmt.Errorf("inkling: decoding args: %w", err)
	}
	return jsonName, args, body[dec.InputOffset():], nil
}
