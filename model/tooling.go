package model

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

func parseGPTToolCall(content string) []ResponseToolCall {
	// .get_weather <|constrain|>json<|message|>{"location":"NYC"}
	// .get_weather <|constrain|>json<|message|>{"location":"NYC"}

	var jsonCalls []string

	for call := range strings.SplitSeq(content, "\n") {
		if call == "" {
			continue
		}

		// Extract tool name (remove leading dot)
		parts := strings.SplitN(call, " ", 2)
		name := strings.TrimPrefix(parts[0], ".")

		// Extract arguments JSON after <|message|>
		var args string
		if idx := strings.Index(call, "<|message|>"); idx != -1 {
			args = call[idx+11:]
		}

		// Build JSON: {"name":"get_weather","arguments":{"location":"NYC"}}
		jsonCall := `{"name":"` + name + `","arguments":` + args + `}`
		jsonCalls = append(jsonCalls, jsonCall)
	}

	return parseToolCall(strings.Join(jsonCalls, "\n"))
}

func parseToolCall(content string) []ResponseToolCall {
	// {"name":"get_weather", "arguments":{"location":"NYC"})
	// {"name":"get_weather", "arguments":{"location":"NYC"})

	var toolCalls []ResponseToolCall

	for call := range strings.SplitSeq(content, "\n") {
		toolCall := ResponseToolCall{
			ID:  uuid.NewString(),
			Raw: call,
		}

		switch {
		case len(call) == 0:
			toolCall.Status = 1
			toolCall.Error = "response missing"

		default:
			if err := json.Unmarshal([]byte(call), &toolCall); err != nil {
				toolCall.Status = 2
				toolCall.Error = err.Error()
			}
		}

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}
