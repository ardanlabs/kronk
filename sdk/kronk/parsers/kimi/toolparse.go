package kimi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"maps"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const parseErrorStatus = 2

func parseToolCalls(content string) []model.ResponseToolCall {
	var calls []model.ResponseToolCall

	for offset := 0; offset < len(content); {
		callAt := strings.Index(content[offset:], openMarker+"call")
		if callAt == -1 {
			break
		}
		callAt += offset

		openerEnd := strings.Index(content[callAt:], sepMarker)
		if openerEnd == -1 {
			calls = append(calls, failedToolCall(len(calls), "", nil, content[callAt:],
				errors.New("parse Kimi call: missing opener separator")))
			break
		}
		openerEnd += callAt

		closeAt := strings.Index(content[openerEnd+len(sepMarker):], closeMarker+"call"+sepMarker)
		if closeAt == -1 {
			calls = append(calls, failedToolCall(len(calls), "", nil, content[callAt:],
				errors.New("parse Kimi call: missing closing marker")))
			break
		}
		closeAt += openerEnd + len(sepMarker)
		callEnd := closeAt + len(closeMarker+"call"+sepMarker)

		raw := content[callAt:callEnd]
		opener := content[callAt+len(openMarker) : openerEnd]
		name, nameErr := attribute(opener, "tool")
		index := len(calls)
		indexText, indexErr := attribute(opener, "index")
		if indexErr == nil {
			xmlIndex, err := strconv.Atoi(indexText)
			switch {
			case err != nil:
				indexErr = fmt.Errorf("parse Kimi call index: %w", err)
			case xmlIndex < 1:
				indexErr = fmt.Errorf("parse Kimi call: invalid index %d", xmlIndex)
			default:
				index = xmlIndex - 1
			}
		}
		body := content[openerEnd+len(sepMarker) : closeAt]
		args, argsErr := parseArguments(body)
		parseErr := errors.Join(nameErr, indexErr, argsErr)

		if parseErr != nil {
			calls = append(calls, failedToolCall(index, name, args, raw, parseErr))
		} else {
			calls = append(calls, model.ResponseToolCall{
				ID:    "call_" + uuid.NewString(),
				Index: index,
				Type:  "function",
				Function: model.ResponseToolCallFunction{
					Name:      name,
					Arguments: args,
				},
			})
		}

		offset = callEnd
	}

	if len(calls) == 0 {
		return []model.ResponseToolCall{failedToolCall(0, "", nil, content,
			errors.New("parse Kimi tools: no call elements"))}
	}

	return calls
}

func parseArguments(body string) (model.ToolCallArguments, error) {
	args := make(model.ToolCallArguments)
	var errs []error

	for offset := 0; offset < len(body); {
		argumentAt := strings.Index(body[offset:], openMarker+"argument")
		if argumentAt == -1 {
			break
		}
		argumentAt += offset

		openerEnd := strings.Index(body[argumentAt:], sepMarker)
		if openerEnd == -1 {
			errs = append(errs, errors.New("parse Kimi argument: missing opener separator"))
			break
		}
		openerEnd += argumentAt

		closeAt := strings.Index(body[openerEnd+len(sepMarker):], closeMarker+"argument"+sepMarker)
		if closeAt == -1 {
			errs = append(errs, errors.New("parse Kimi argument: missing closing marker"))
			break
		}
		closeAt += openerEnd + len(sepMarker)

		opener := body[argumentAt+len(openMarker) : openerEnd]
		key, keyErr := attribute(opener, "key")
		valueType, typeErr := attribute(opener, "type")
		valueText := body[openerEnd+len(sepMarker) : closeAt]

		if keyErr != nil || typeErr != nil {
			errs = append(errs, errors.Join(keyErr, typeErr))
		} else if _, exists := args[key]; exists {
			errs = append(errs, fmt.Errorf("parse Kimi argument %q: duplicate key", key))
		} else {
			value, err := parseValue(valueType, valueText)
			if err != nil {
				errs = append(errs, fmt.Errorf("parse Kimi argument %q: %w", key, err))
			} else {
				args[key] = value
			}
		}

		offset = closeAt + len(closeMarker+"argument"+sepMarker)
	}

	jsonAt := strings.Index(body, openMarker+"json")
	if jsonAt != -1 {
		openerEnd := strings.Index(body[jsonAt:], sepMarker)
		if openerEnd == -1 {
			errs = append(errs, errors.New("parse Kimi json: missing opener separator"))
		} else {
			openerEnd += jsonAt
			closeAt := strings.Index(body[openerEnd+len(sepMarker):], closeMarker+"json"+sepMarker)
			if closeAt == -1 {
				errs = append(errs, errors.New("parse Kimi json: missing closing marker"))
			} else {
				closeAt += openerEnd + len(sepMarker)
				raw := body[openerEnd+len(sepMarker) : closeAt]
				var parsed map[string]any
				if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
					errs = append(errs, fmt.Errorf("parse Kimi json object: %w", err))
				} else {
					maps.Copy(args, parsed)
				}
			}
		}
	}

	return args, errors.Join(errs...)
}

func parseValue(valueType, text string) (any, error) {
	if valueType == "string" {
		return text, nil
	}

	if valueType == "null" {
		if strings.TrimSpace(text) != "null" && strings.TrimSpace(text) != "" {
			return nil, fmt.Errorf("invalid null %q", text)
		}
		return nil, nil
	}

	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &value); err != nil {
		return nil, fmt.Errorf("parse %s value: %w", valueType, err)
	}

	switch valueType {
	case "boolean":
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("expected boolean")
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return nil, fmt.Errorf("expected number")
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("expected object")
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return nil, fmt.Errorf("expected array")
		}
	default:
		return nil, fmt.Errorf("unsupported type %q", valueType)
	}

	return value, nil
}

func attribute(opener, key string) (string, error) {
	prefix := key + `="`
	start := strings.Index(opener, prefix)
	if start == -1 {
		return "", fmt.Errorf("parse Kimi tag: missing %s attribute", key)
	}
	start += len(prefix)

	end := strings.IndexByte(opener[start:], '"')
	if end == -1 {
		return "", fmt.Errorf("parse Kimi tag: unterminated %s attribute", key)
	}

	value := html.UnescapeString(opener[start : start+end])
	if value == "" {
		return "", fmt.Errorf("parse Kimi tag: empty %s attribute", key)
	}

	return value, nil
}

func failedToolCall(index int, name string, args model.ToolCallArguments, raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:    "call_" + uuid.NewString(),
		Index: index,
		Type:  "function",
		Function: model.ResponseToolCallFunction{
			Name:      name,
			Arguments: args,
		},
		Status: parseErrorStatus,
		Raw:    raw,
		Error:  err.Error(),
	}
}
