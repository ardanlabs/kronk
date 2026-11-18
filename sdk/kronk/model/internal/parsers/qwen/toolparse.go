package qwen

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
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// parseQwenXML parses Qwen3-Coder style tool calls with XML-like tags.
// Format: <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
//
// The format has no escaping rule for its closing markers. Values containing
// those markers are therefore malformed and fail the entire parse rather than
// being reinterpreted as additional tool calls.
func parseQwenXML(content string) []parsers.ResponseToolCall {
	var toolCalls []parsers.ResponseToolCall
	raw := content

	// NOTE: We intentionally do NOT convert literal \n to actual newlines here.
	// The model uses real newlines to delimit parameters in the XML format.
	// Literal \n sequences inside parameter values (e.g., Go source code like
	// fmt.Printf("hello\n")) must be preserved as-is so that the content
	// written to files retains the correct escape sequences.

	content = strings.TrimLeft(content, " \t\n\r")
	if content == "" {
		return []parsers.ResponseToolCall{failedXMLToolCall(raw, errors.New("parse qwen XML: tool call is empty"))}
	}

	for content != "" {
		if !strings.HasPrefix(content, "<function=") {
			return []parsers.ResponseToolCall{failedXMLToolCall(raw,
				errors.New("parse qwen XML: unexpected content outside function"))}
		}

		funcEnd := strings.IndexByte(content, '>')
		if funcEnd == -1 {
			return []parsers.ResponseToolCall{failedXMLToolCall(raw,
				errors.New("parse qwen XML: function opener is unterminated"))}
		}

		name := strings.TrimSpace(content[len("<function="):funcEnd])
		if name == "" {
			return []parsers.ResponseToolCall{failedXMLToolCall(raw,
				errors.New("parse qwen XML: function name is empty"))}
		}

		args := make(map[string]any)
		content = content[funcEnd+1:]
		for {
			content = strings.TrimLeft(content, " \t\n\r")
			if strings.HasPrefix(content, "</function>") {
				content = content[len("</function>"):]
				break
			}

			if !strings.HasPrefix(content, "<parameter=") {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: unexpected content inside function %q", name))}
			}

			paramNameEnd := strings.IndexByte(content, '>')
			if paramNameEnd == -1 {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter opener in function %q is unterminated", name))}
			}

			paramName := strings.TrimSpace(content[len("<parameter="):paramNameEnd])
			if paramName == "" {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter name in function %q is empty", name))}
			}

			valueStart := paramNameEnd + 1
			paramCloseRel := strings.Index(content[valueStart:], "</parameter>")
			if paramCloseRel == -1 {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter %q in function %q is not closed", paramName, name))}
			}
			if funcCloseRel := strings.Index(content[valueStart:], "</function>"); funcCloseRel != -1 && funcCloseRel < paramCloseRel {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: function %q closes before parameter %q", name, paramName))}
			}
			paramClose := valueStart + paramCloseRel

			paramValue := content[valueStart:paramClose]
			paramValue = strings.TrimPrefix(paramValue, "\n")
			paramValue = strings.TrimSuffix(paramValue, "\n")
			if _, exists := args[paramName]; exists {
				return []parsers.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter %q in function %q is duplicated", paramName, name))}
			}
			args[paramName] = paramValue

			content = content[paramClose+len("</parameter>"):]
		}

		toolCalls = append(toolCalls, parsers.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: parsers.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		content = strings.TrimLeft(content, " \t\n\r")
	}

	return toolCalls
}

func failedXMLToolCall(raw string, err error) parsers.ResponseToolCall {
	return parsers.ResponseToolCall{
		ID:     newToolCallID(),
		Type:   "function",
		Status: 2,
		Raw:    raw,
		Error:  err.Error(),
	}
}

// normalizeXMLArguments converts direct-XML parameter text according to the
// matching function's declared schema. Values without an unambiguous schema
// type remain strings.
func normalizeXMLArguments(toolCalls []parsers.ResponseToolCall, tools []parsers.D) {
	for i := range toolCalls {
		properties := toolProperties(tools, toolCalls[i].Function.Name)
		if properties == nil {
			continue
		}

		for name, value := range toolCalls[i].Function.Arguments {
			raw, ok := value.(string)
			if !ok {
				continue
			}

			property, ok := properties[name].(parsers.D)
			if !ok {
				continue
			}

			schemaType, ok := declaredSchemaType(property["type"])
			if !ok || schemaType == "string" {
				continue
			}

			if schemaType == "boolean" {
				switch strings.ToLower(strings.TrimSpace(raw)) {
				case "true":
					toolCalls[i].Function.Arguments[name] = true
				case "false":
					toolCalls[i].Function.Arguments[name] = false
				}
				continue
			}

			parsed, ok := decodeJSONValue(raw)
			if !ok {
				continue
			}

			switch schemaType {
			case "object":
				if _, ok := parsed.(map[string]any); ok {
					toolCalls[i].Function.Arguments[name] = parsed
				}

			case "array":
				if _, ok := parsed.([]any); ok {
					toolCalls[i].Function.Arguments[name] = parsed
				}

			case "number":
				if _, ok := parsed.(json.Number); ok {
					toolCalls[i].Function.Arguments[name] = parsed
				}

			case "integer":
				if number, ok := parsed.(json.Number); ok && isJSONInteger(number) {
					toolCalls[i].Function.Arguments[name] = parsed
				}

			case "null":
				if parsed == nil {
					toolCalls[i].Function.Arguments[name] = nil
				}
			}
		}
	}
}

func toolProperties(tools []parsers.D, name string) parsers.D {
	var properties parsers.D

	for _, tool := range tools {
		if tool["type"] != "function" {
			continue
		}

		function, ok := tool["function"].(parsers.D)
		if !ok || function["name"] != name {
			continue
		}

		if properties != nil {
			return nil
		}

		parameters, ok := function["parameters"].(parsers.D)
		if !ok {
			return nil
		}

		properties, ok = parameters["properties"].(parsers.D)
		if !ok {
			return nil
		}
	}

	return properties
}

func declaredSchemaType(value any) (string, bool) {
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

func decodeJSONValue(raw string) (any, bool) {
	if !json.Valid([]byte(raw)) {
		return nil, false
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}

	return value, true
}

func isJSONInteger(number json.Number) bool {
	value, ok := new(big.Rat).SetString(number.String())
	return ok && value.IsInt()
}

// parseJSON parses tool calls in the OpenAI JSON envelope format used inside
// Qwen's <tool_call>…</tool_call> wrappers.
func parseJSON(ctx context.Context, log applog.Logger, content string) []parsers.ResponseToolCall {
	raw := content
	remaining := strings.TrimLeft(content, " \t\n\r")
	if remaining == "" {
		return []parsers.ResponseToolCall{failedJSONToolCall(raw, errors.New("parse qwen JSON: tool call is empty"))}
	}

	var toolCalls []parsers.ResponseToolCall
	for remaining != "" {
		if remaining[0] != '{' {
			return []parsers.ResponseToolCall{failedJSONToolCall(raw, errors.New("parse qwen JSON: unexpected content outside tool call object"))}
		}

		end := findJSONObjectEnd(remaining)
		if end < 0 {
			return []parsers.ResponseToolCall{failedJSONToolCall(raw, errors.New("parse qwen JSON: incomplete tool call object"))}
		}

		call := remaining[:end]
		function, err := decodeQwenFunction(call)
		if err != nil {
			function, err = repairQwenFunction(call, err)
		}
		if err != nil {
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed",
					"format", "json", "error", err, "json", call)
			}
			return []parsers.ResponseToolCall{failedJSONToolCall(raw, err)}
		}

		function.Name = strings.TrimPrefix(function.Name, ".")
		if function.Name == "" {
			return []parsers.ResponseToolCall{failedJSONToolCall(raw, errors.New("parse qwen JSON: tool call name is empty"))}
		}

		toolCalls = append(toolCalls, parsers.ResponseToolCall{
			ID:       newToolCallID(),
			Type:     "function",
			Function: function,
		})
		remaining = strings.TrimLeft(remaining[end:], " \t\n\r")
	}

	return toolCalls
}

func failedJSONToolCall(raw string, err error) parsers.ResponseToolCall {
	return parsers.ResponseToolCall{ID: newToolCallID(), Type: "function", Status: 2, Raw: raw, Error: err.Error()}
}

func repairQwenFunction(raw string, original error) (parsers.ResponseToolCallFunction, error) {
	repaired, err := jsonrepair.Repair(raw)
	if err != nil || !qwenRepairOnlyAddsEscapes(raw, repaired) {
		return parsers.ResponseToolCallFunction{}, original
	}

	return decodeQwenFunction(repaired)
}

func qwenRepairOnlyAddsEscapes(raw, repaired string) bool {
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

func decodeQwenFunction(raw string) (parsers.ResponseToolCallFunction, error) {
	value, err := decodeUniqueQwenJSON(raw)
	if err != nil {
		return parsers.ResponseToolCallFunction{}, err
	}

	envelope, ok := value.(map[string]any)
	if !ok {
		return parsers.ResponseToolCallFunction{}, errors.New("parse qwen JSON: tool call must be an object")
	}
	name, ok := envelope["name"].(string)
	if !ok || name == "" {
		return parsers.ResponseToolCallFunction{}, errors.New("parse qwen JSON: tool call name is empty")
	}

	argumentValue, ok := envelope["arguments"]
	if !ok {
		return parsers.ResponseToolCallFunction{}, errors.New("parse qwen JSON: tool call arguments must be an object")
	}
	if encoded, ok := argumentValue.(string); ok {
		argumentValue, err = decodeUniqueQwenJSON(encoded)
		if err != nil {
			return parsers.ResponseToolCallFunction{}, fmt.Errorf("parse qwen JSON: invalid tool arguments: %w", err)
		}
	}
	arguments, ok := argumentValue.(map[string]any)
	if !ok {
		return parsers.ResponseToolCallFunction{}, errors.New("parse qwen JSON: tool call arguments must be an object")
	}

	return parsers.ResponseToolCallFunction{Name: name, Arguments: parsers.ToolCallArguments(arguments)}, nil
}

func decodeUniqueQwenJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	value, err := decodeUniqueQwenJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("parse qwen JSON: unexpected data after JSON value")
		}
		return nil, err
	}

	return value, nil
}

func decodeUniqueQwenJSONValue(decoder *json.Decoder) (any, error) {
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
				return nil, errors.New("parse qwen JSON: object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("parse qwen JSON: duplicate key %q", key)
			}
			value, err := decodeUniqueQwenJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
			return nil, errors.New("parse qwen JSON: object is not closed")
		}
		return object, nil

	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeUniqueQwenJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			return nil, errors.New("parse qwen JSON: array is not closed")
		}
		return array, nil
	}

	return nil, fmt.Errorf("parse qwen JSON: unexpected delimiter %q", delim)
}

func findJSONObjectEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		idx := strings.Index(s, "{")
		if idx == -1 {
			return -1
		}
		s = s[idx:]
	}

	depth := 0
	inString := false
	escape := false

	for i, c := range s {
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
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

func newToolCallID() string {
	return "call_" + uuid.New().String()
}
