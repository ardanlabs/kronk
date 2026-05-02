package mistral

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseMistral parses Mistral/Devstral style tool calls.
// Format: [TOOL_CALLS]get_weather[ARGS]{"location":"NYC"}
//
// Direct port of the legacy parseMistralToolCall.
func parseMistral(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall

	remaining := content
	for {
		callStart := strings.Index(remaining, "[TOOL_CALLS]")
		if callStart == -1 {
			break
		}

		argsStart := strings.Index(remaining[callStart:], "[ARGS]")
		if argsStart == -1 {
			break
		}

		name := remaining[callStart+12 : callStart+argsStart]

		argsContent := remaining[callStart+argsStart+6:]

		endIdx := findJSONObjectEnd(argsContent)
		var argsJSON string
		switch endIdx == -1 {
		case true:
			argsJSON = argsContent
			remaining = ""
		case false:
			argsJSON = argsContent[:endIdx]
			remaining = argsContent[endIdx:]
		}

		var args map[string]any
		if err := jsonrepair.Unmarshal(argsJSON, &args); err != nil {
			if log != nil {
				log(ctx, "jsonrepair", "status", "unmarshal-failed",
					"format", "mistral", "error", err, "json", argsJSON)
			}
			args = make(map[string]any)
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
