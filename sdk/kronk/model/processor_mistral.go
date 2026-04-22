package model

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
)

// parseMistralToolCall parses Mistral/Devstral style tool calls.
// Format: [TOOL_CALLS]get_weather[ARGS]{"location": "NYC"}
func (p *processor) parseMistralToolCall(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

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
			p.model.log(context.Background(), "jsonrepair", "status", "unmarshal-failed",
				"format", "mistral", "error", err, "json", argsJSON)
			args = make(map[string]any)
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
	}

	return toolCalls
}
