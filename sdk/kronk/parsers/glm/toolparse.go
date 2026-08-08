package glm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseGLM parses GLM-style tool calls with <arg_key>/<arg_value> tags.
// Format: get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
func parseGLM(content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall
	raw := content

	for call := range strings.SplitSeq(content, "\n") {
		call = strings.TrimSpace(call)
		if call == "" {
			continue
		}

		argKeyIdx := strings.Index(call, "<arg_key>")
		if argKeyIdx == -1 {
			return []model.ResponseToolCall{failedGLMToolCall(raw,
				errors.New("parse glm: call has no argument key"))}
		}

		name := strings.TrimSpace(call[:argKeyIdx])
		if name == "" {
			return []model.ResponseToolCall{failedGLMToolCall(raw,
				errors.New("parse glm: function name is empty"))}
		}
		args := make(map[string]any)

		remaining := call[argKeyIdx:]
		for remaining != "" {
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
			remaining = strings.TrimLeft(remaining, " \t\r")

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
			remaining = strings.TrimLeft(remaining, " \t\r")
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

func newToolCallID() string {
	return "call_" + uuid.NewString()
}
