package model

import (
	"encoding/json"
	"strings"
)

// parseQwenToolCall parses Qwen3-Coder style tool calls with XML-like tags.
// Format: <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
func parseQwenToolCall(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

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

			// Try to parse the value as JSON so that arrays and objects
			// are stored as proper Go types ([]any, map[string]any) instead
			// of raw strings. Fall back to the plain string for scalars.
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
				// Try JSON parse for scalars: true, false, null, numbers.
				var parsed any
				if err := json.Unmarshal([]byte(paramValue), &parsed); err == nil {
					args[paramName] = parsed
				} else {
					args[paramName] = paramValue
				}
			}

			remaining = remaining[paramClose+12:]
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		content = content[closeFunc+11:]
	}

	return toolCalls
}
