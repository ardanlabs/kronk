package llama

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

func parseJSON(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	content = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), pythonTag))
	start := strings.IndexByte(content, '{')
	if start == -1 {
		err := errors.New("tool call does not contain a JSON object")
		return []model.ResponseToolCall{{
			ID:     "call_" + uuid.NewString(),
			Type:   "function",
			Status: 2,
			Error:  err.Error(),
			Raw:    content,
		}}
	}
	content = content[start:]
	end := findJSONObjectEnd(content)
	if end == -1 {
		end = len(content)
	}
	raw := content[:end]

	toolCall := model.ResponseToolCall{
		ID:   "call_" + uuid.NewString(),
		Type: "function",
	}
	if err := unmarshalFunction(raw, &toolCall.Function); err != nil {
		if log != nil {
			log(ctx, "jsonrepair", "status", "unmarshal-failed", "format", "llama", "error", err, "json", raw)
		}
		toolCall.Status = 2
		toolCall.Error = err.Error()
		toolCall.Raw = raw
	}

	return []model.ResponseToolCall{toolCall}
}

func unmarshalFunction(raw string, function *model.ResponseToolCallFunction) error {
	repaired, err := jsonrepair.Repair(raw)
	if err != nil {
		return err
	}

	var envelope struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(repaired), &envelope); err != nil {
		return err
	}
	if envelope.Name == "" {
		return errors.New("tool call name is empty")
	}
	if len(envelope.Parameters) == 0 || string(envelope.Parameters) == "null" || envelope.Parameters[0] != '{' {
		return errors.New("tool call parameters must be an object")
	}

	decoder := json.NewDecoder(strings.NewReader(string(envelope.Parameters)))
	decoder.UseNumber()
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil {
		return err
	}
	if arguments == nil {
		return errors.New("tool call parameters must be an object")
	}

	function.Name = envelope.Name
	function.Arguments = model.ToolCallArguments(arguments)
	return nil
}
