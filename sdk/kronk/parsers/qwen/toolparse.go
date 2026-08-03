package qwen

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseQwenXML parses Qwen3-Coder style tool calls with XML-like tags.
// Format: <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
//
// Direct port of the legacy parseQwenToolCall.
func parseQwenXML(content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall

	// NOTE: We intentionally do NOT convert literal \n to actual newlines here.
	// The model uses real newlines to delimit parameters in the XML format.
	// Literal \n sequences inside parameter values (e.g., Go source code like
	// fmt.Printf("hello\n")) must be preserved as-is so that the content
	// written to files retains the correct escape sequences.

	for {
		funcStart := strings.Index(content, "<function=")
		if funcStart == -1 {
			break
		}

		funcEnd := strings.Index(content[funcStart:], ">")
		if funcEnd == -1 {
			break
		}

		name := strings.TrimSpace(content[funcStart+10 : funcStart+funcEnd])

		bodyStart := funcStart + funcEnd + 1
		closeFunc := strings.Index(content[bodyStart:], "</function>")
		if closeFunc == -1 {
			break
		}
		closeFunc += bodyStart

		funcBody := content[bodyStart:closeFunc]
		args := make(map[string]any)

		remaining := funcBody
		for {
			paramStart := strings.Index(remaining, "<parameter=")
			if paramStart == -1 {
				break
			}

			paramNameEnd := strings.Index(remaining[paramStart:], ">")
			if paramNameEnd == -1 {
				break
			}

			paramName := strings.TrimSpace(remaining[paramStart+11 : paramStart+paramNameEnd])

			valueStart := paramStart + paramNameEnd + 1
			paramCloseRel := strings.Index(remaining[valueStart:], "</parameter>")
			if paramCloseRel == -1 {
				break
			}
			paramClose := valueStart + paramCloseRel

			paramValue := remaining[valueStart:paramClose]
			paramValue = strings.TrimPrefix(paramValue, "\n")
			paramValue = strings.TrimSuffix(paramValue, "\n")
			args[paramName] = paramValue

			remaining = remaining[paramClose+12:]
		}

		toolCalls = append(toolCalls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		content = content[closeFunc+11:]
	}

	return toolCalls
}

// normalizeXMLArguments converts direct-XML parameter text according to the
// matching function's declared schema. Values without an unambiguous schema
// type remain strings.
func normalizeXMLArguments(toolCalls []model.ResponseToolCall, tools []model.D) {
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

			property, ok := properties[name].(model.D)
			if !ok {
				continue
			}

			schemaType, ok := declaredSchemaType(property["type"])
			if !ok || schemaType == "string" {
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

			case "boolean":
				if _, ok := parsed.(bool); ok {
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

func toolProperties(tools []model.D, name string) model.D {
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
func parseJSON(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall

	remaining := content
	for len(remaining) > 0 {
		remaining = strings.TrimLeft(remaining, " \t\n\r")
		if len(remaining) == 0 {
			break
		}

		if remaining[0] != '{' {
			idx := strings.Index(remaining, "{")
			if idx == -1 {
				break
			}
			remaining = remaining[idx:]
		}

		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			jsonEnd = len(remaining)
		}

		call := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		toolCall := model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
		}

		if err := jsonrepair.Unmarshal(call, &toolCall.Function); err != nil {
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed",
					"format", "json", "error", err, "json", call)
			}
			toolCall.Status = 2
			toolCall.Error = err.Error()
			toolCall.Raw = call
		}

		toolCall.Function.Name = strings.TrimPrefix(toolCall.Function.Name, ".")

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
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
	return "call_" + uuid.NewString()
}
