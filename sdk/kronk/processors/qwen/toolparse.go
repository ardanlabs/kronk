package qwen

import (
	"context"
	"encoding/json"
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

			paramValue := strings.TrimSpace(remaining[valueStart:paramClose])

			switch {
			case len(paramValue) == 0:
				args[paramName] = paramValue

			case paramValue[0] == '{', paramValue[0] == '[', paramValue[0] == '"':
				args[paramName] = paramValue

				var parsed any
				if err := json.Unmarshal([]byte(paramValue), &parsed); err == nil {
					args[paramName] = parsed
				}

			default:
				var parsed any
				if err := json.Unmarshal([]byte(paramValue), &parsed); err == nil {
					args[paramName] = parsed
				} else {
					args[paramName] = paramValue
				}
			}

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
