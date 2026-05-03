package gpt

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseGPTToolCall parses GPT-OSS tool calls.
// Format: .FUNC_NAME <|message|>JSON_ARGS
// The JSON may span multiple lines, so we can't split by newlines.
// Instead, find each ".NAME <|message|>" prefix and extract the JSON that
// follows.
//
// Direct port of model.stateMachine.parseGPTToolCall, with the call to
// parseToolCall (sniffer) replaced by a direct call to the local JSON parser
// since the joined-JSON output always starts with {"name".
func parseGPTToolCall(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var jsonCalls []string
	remaining := content

	for {
		dotIdx := strings.Index(remaining, ".")
		if dotIdx == -1 {
			break
		}

		remaining = remaining[dotIdx:]

		msgIdx := strings.Index(remaining, "<|message|>")
		if msgIdx == -1 {
			break
		}

		// Extract function name (between dot and space before <|message|>).
		prefix := remaining[:msgIdx]
		parts := strings.SplitN(prefix, " ", 2)
		name := strings.TrimPrefix(parts[0], ".")

		// Move past <|message|> to get the JSON.
		jsonStart := msgIdx + 11
		remaining = remaining[jsonStart:]

		// Find the end of the JSON object by matching braces.
		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			jsonEnd = len(remaining)
		}

		args := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		jsonCall := `{"name":"` + name + `","arguments":` + args + `}`
		jsonCalls = append(jsonCalls, jsonCall)
	}

	return parseJSONToolCall(ctx, log, strings.Join(jsonCalls, "\n"))
}

// parseJSONToolCall parses a sequence of JSON tool-call objects.
// Format: {"name":"get_weather", "arguments":{"location":"NYC"}}
//
// Duplicated from processor/standard so the gpt processor stays self-contained.
func parseJSONToolCall(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
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

		// GPT models prefix function names with a dot (e.g. ".Kronk_web_search").
		// Strip it so clients can match the name to their registered tools.
		toolCall.Function.Name = strings.TrimPrefix(toolCall.Function.Name, ".")

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}

// findJSONObjectEnd finds the end of a JSON object starting at the beginning
// of s. Returns the index after the closing brace, or -1 if not found.
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
