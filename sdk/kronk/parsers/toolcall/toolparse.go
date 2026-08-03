package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// parseJSON parses a sequence of JSON tool-call objects.
// Format: {"name":"get_weather","arguments":{"location":"NYC"}}
func parseJSON(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	var toolCalls []model.ResponseToolCall
	if strings.TrimSpace(content) == "" {
		return []model.ResponseToolCall{failedToolCall(content, errors.New("tool call is empty"))}
	}

	remaining := content
	for len(remaining) > 0 {
		remaining = strings.TrimLeft(remaining, " \t\n\r")
		if len(remaining) == 0 {
			break
		}

		if remaining[0] != '{' {
			idx := strings.Index(remaining, "{")
			if idx == -1 {
				toolCalls = append(toolCalls, failedToolCall(remaining, errors.New("tool call does not contain a JSON object")))
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

		if err := unmarshalStandardFunction(call, &toolCall.Function); err != nil {
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

func failedToolCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:     newToolCallID(),
		Type:   "function",
		Status: 2,
		Error:  err.Error(),
		Raw:    raw,
	}
}

func unmarshalStandardFunction(raw string, function *model.ResponseToolCallFunction) error {
	repaired, err := jsonrepair.Repair(raw)
	if err != nil {
		return err
	}

	var wire struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(repaired), &wire); err != nil {
		return err
	}
	if wire.Name == "" {
		return errors.New("tool call name is empty")
	}
	if len(wire.Arguments) == 0 || string(wire.Arguments) == "null" {
		return errors.New("tool call arguments must be an object")
	}
	function.Name = wire.Name

	arguments, err := decodeStandardArguments(wire.Arguments)
	if err != nil {
		return err
	}

	function.Arguments = arguments

	return nil
}

func decodeStandardArguments(raw json.RawMessage) (model.ToolCallArguments, error) {
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, err
		}
		if encoded == "" {
			return nil, errors.New("tool call arguments must be an object")
		}
		raw = json.RawMessage(encoded)
	}

	if !json.Valid(raw) {
		return nil, errors.New("invalid tool arguments JSON")
	}

	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()

	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil {
		return nil, err
	}
	if arguments == nil {
		return nil, errors.New("tool call arguments must be an object")
	}

	return model.ToolCallArguments(arguments), nil
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
