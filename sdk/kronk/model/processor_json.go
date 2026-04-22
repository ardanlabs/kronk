package model

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
)

// parseJSONToolCall parses tool calls in standard JSON format.
// Format: {"name":"get_weather", "arguments":{"location":"NYC"}}
func (p *processor) parseJSONToolCall(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	remaining := content
	for len(remaining) > 0 {
		// Skip leading whitespace and newlines.
		remaining = strings.TrimLeft(remaining, " \t\n\r")
		if len(remaining) == 0 {
			break
		}

		// Find the start of a JSON object.
		if remaining[0] != '{' {
			// Skip non-JSON content until we find '{' or run out.
			idx := strings.Index(remaining, "{")
			if idx == -1 {
				break
			}
			remaining = remaining[idx:]
		}

		// Find the end of this JSON object.
		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			// Malformed JSON - try to parse what's left.
			jsonEnd = len(remaining)
		}

		call := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		toolCall := ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
		}

		if err := jsonrepair.Unmarshal(call, &toolCall.Function); err != nil {
			p.model.log(context.Background(), "jsonrepair", "status", "unmarshal-failed",
				"format", "json", "error", err, "json", call)
			toolCall.Status = 2
			toolCall.Error = err.Error()
			toolCall.Raw = call
		}

		// GPT models prefix function names with a dot (e.g. ".Kronk_web_search").
		// Strip it so clients can match the name to their registered tools.
		toolCall.Function.Name = strings.TrimPrefix(toolCall.Function.Name, ".")

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}

// findJSONObjectEnd finds the end of a JSON object starting at the beginning of s.
// Returns the index after the closing brace, or -1 if not found.
func findJSONObjectEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		// Try to find the start of JSON object.
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
