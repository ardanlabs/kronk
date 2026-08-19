package qwen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// parseQwenXML parses Qwen3-Coder style tool calls with XML-like tags.
// Format: <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
//
// The format has no escaping rule for its closing markers. Values containing
// those markers are therefore malformed and fail the entire parse rather than
// being reinterpreted as additional tool calls.
func parseQwenXML(content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall
	raw := content

	// NOTE: We intentionally do NOT convert literal \n to actual newlines here.
	// The model uses real newlines to delimit parameters in the XML format.
	// Literal \n sequences inside parameter values (e.g., Go source code like
	// fmt.Printf("hello\n")) must be preserved as-is so that the content
	// written to files retains the correct escape sequences.

	content = strings.TrimLeft(content, " \t\n\r")
	if content == "" {
		return []model.ResponseToolCall{failedXMLToolCall(raw, errors.New("parse qwen XML: tool call is empty"))}
	}

	for content != "" {
		if !strings.HasPrefix(content, "<function=") {
			return []model.ResponseToolCall{failedXMLToolCall(raw,
				errors.New("parse qwen XML: unexpected content outside function"))}
		}

		funcEnd := strings.IndexByte(content, '>')
		if funcEnd == -1 {
			return []model.ResponseToolCall{failedXMLToolCall(raw,
				errors.New("parse qwen XML: function opener is unterminated"))}
		}

		name := strings.TrimSpace(content[len("<function="):funcEnd])
		if name == "" {
			return []model.ResponseToolCall{failedXMLToolCall(raw,
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
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: unexpected content inside function %q", name))}
			}

			paramNameEnd := strings.IndexByte(content, '>')
			if paramNameEnd == -1 {
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter opener in function %q is unterminated", name))}
			}

			paramName := strings.TrimSpace(content[len("<parameter="):paramNameEnd])
			if paramName == "" {
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter name in function %q is empty", name))}
			}

			valueStart := paramNameEnd + 1
			paramCloseRel := strings.Index(content[valueStart:], "</parameter>")
			if paramCloseRel == -1 {
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter %q in function %q is not closed", paramName, name))}
			}
			if funcCloseRel := strings.Index(content[valueStart:], "</function>"); funcCloseRel != -1 && funcCloseRel < paramCloseRel {
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: function %q closes before parameter %q", name, paramName))}
			}
			paramClose := valueStart + paramCloseRel

			paramValue := content[valueStart:paramClose]
			paramValue = strings.TrimPrefix(paramValue, "\n")
			paramValue = strings.TrimSuffix(paramValue, "\n")
			if _, exists := args[paramName]; exists {
				return []model.ResponseToolCall{failedXMLToolCall(raw,
					fmt.Errorf("parse qwen XML: parameter %q in function %q is duplicated", paramName, name))}
			}
			args[paramName] = paramValue

			content = content[paramClose+len("</parameter>"):]
		}

		toolCalls = append(toolCalls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		content = strings.TrimLeft(content, " \t\n\r")
	}

	return toolCalls
}

func failedXMLToolCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
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
	return "call_" + uuid.New().String()
}
