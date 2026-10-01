package glm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// parseGLM parses GLM-style tool calls with <arg_key>/<arg_value> tags.
// Format: get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
func parseGLM(content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall
	raw := content
	remaining := strings.TrimSpace(content)

	for remaining != "" {
		argKeyIdx := strings.Index(remaining, "<arg_key>")
		if argKeyIdx == -1 {
			return []model.ResponseToolCall{failedGLMToolCall(raw,
				errors.New("parse glm: call has no argument key"))}
		}

		name := strings.TrimSpace(remaining[:argKeyIdx])
		if name == "" {
			return []model.ResponseToolCall{failedGLMToolCall(raw,
				errors.New("parse glm: function name is empty"))}
		}
		args := make(map[string]any)

		remaining = remaining[argKeyIdx:]
		for {
			if !strings.HasPrefix(remaining, "<arg_key>") {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: unexpected content in function %q", name))}
			}

			keyEnd := strings.Index(remaining[len("<arg_key>"):], "</arg_key>")
			if keyEnd == -1 {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: argument key in function %q is not closed", name))}
			}
			keyEnd += len("<arg_key>")

			key := remaining[len("<arg_key>"):keyEnd]
			if key == "" {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: argument key in function %q is empty", name))}
			}
			if _, exists := args[key]; exists {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: argument %q in function %q is duplicated", key, name))}
			}
			remaining = remaining[keyEnd+len("</arg_key>"):]
			remaining = strings.TrimLeft(remaining, " \t\r\n")

			if !strings.HasPrefix(remaining, "<arg_value>") {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: argument %q in function %q has no value", key, name))}
			}
			remaining = remaining[len("<arg_value>"):]

			valEnd := strings.Index(remaining, "</arg_value>")
			if valEnd == -1 {
				return []model.ResponseToolCall{failedGLMToolCall(raw,
					fmt.Errorf("parse glm: argument %q in function %q value is not closed", key, name))}
			}

			value := remaining[:valEnd]
			args[key] = value

			remaining = remaining[valEnd+12:]
			remaining = strings.TrimLeft(remaining, " \t\r\n")
			if !strings.HasPrefix(remaining, "<arg_key>") {
				break
			}
		}

		toolCalls = append(toolCalls, newGLMToolCall(name, args))
	}

	if len(toolCalls) == 0 {
		return []model.ResponseToolCall{failedGLMToolCall(raw,
			errors.New("parse glm: no tool calls"))}
	}

	return toolCalls
}

func newGLMToolCall(name string, args model.ToolCallArguments) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:   newToolCallID(),
		Type: "function",
		Function: model.ResponseToolCallFunction{
			Name:      name,
			Arguments: args,
		},
	}
}

func failedGLMToolCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:     newToolCallID(),
		Type:   "function",
		Status: 2,
		Raw:    raw,
		Error:  err.Error(),
	}
}

func normalizeGLMArguments(toolCalls []model.ResponseToolCall, tools []model.D) {
	for i := range toolCalls {
		properties := glmToolProperties(tools, toolCalls[i].Function.Name)
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
			schemaType, ok := glmSchemaType(property["type"])
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

			parsed, ok := decodeGLMJSONValue(raw)
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
				if number, ok := parsed.(json.Number); ok && glmJSONInteger(number) {
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

func glmToolProperties(tools []model.D, name string) model.D {
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

func glmSchemaType(value any) (string, bool) {
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

func decodeGLMJSONValue(raw string) (any, bool) {
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

func glmJSONInteger(number json.Number) bool {
	value, ok := new(big.Rat).SetString(number.String())
	return ok && value.IsInt()
}

func newToolCallID() string {
	return "call_" + uuid.New().String()
}
