package llama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const parseErrorStatus = 2

func parseJSON(ctx context.Context, log applog.Logger, content string) []model.ResponseToolCall {
	toolCall := model.ResponseToolCall{ID: "call_" + uuid.NewString(), Type: "function"}
	if err := unmarshalFunction(content, &toolCall.Function); err != nil {
		if log != nil {
			log(ctx, "tool-call", "status", "unmarshal-failed", "format", "llama", "error", err, "json", content)
		}
		toolCall.Function = model.ResponseToolCallFunction{}
		toolCall.Status = parseErrorStatus
		toolCall.Error = err.Error()
		toolCall.Raw = content
	}

	return []model.ResponseToolCall{toolCall}
}

func unmarshalFunction(raw string, function *model.ResponseToolCallFunction) error {
	content := trimASCIIWhitespace(raw)
	if strings.HasPrefix(content, pythonTag) {
		content = trimASCIIWhitespace(content[len(pythonTag):])
		if content == "" {
			return errors.New("tool call after python tag is empty")
		}
	}
	if content == "" || content[0] != '{' {
		return errors.New("tool call must be one JSON object")
	}

	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	value, err := decodeUnique(decoder)
	if err != nil {
		return err
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("reading tool call tail: %w", err)
		}
		return fmt.Errorf("unexpected JSON value after tool call: %v", token)
	}

	envelope, ok := value.(map[string]any)
	if !ok {
		return errors.New("tool call must be one JSON object")
	}
	name, ok := envelope["name"].(string)
	if !ok || name == "" {
		return errors.New("tool call name is empty")
	}
	parameters, ok := envelope["parameters"].(map[string]any)
	if !ok {
		return errors.New("tool call parameters must be an object")
	}

	function.Name = name
	function.Arguments = model.ToolCallArguments(parameters)
	return nil
}

func decodeUnique(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decoding tool call: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decoding object key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeUnique(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("closing JSON object: %w", err)
		}
		return object, nil

	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeUnique(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("closing JSON array: %w", err)
		}
		return array, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

func trimASCIIWhitespace(content string) string {
	return strings.Trim(content, " \t\r\n")
}
