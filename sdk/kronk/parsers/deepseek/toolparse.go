package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const parseErrorStatus = 2

func parseDSML(content string) []model.ResponseToolCall {
	body, err := toolCallsBody(content)
	if err != nil {
		return []model.ResponseToolCall{failedToolCall("", nil, content, err)}
	}

	var calls []model.ResponseToolCall
	for offset := 0; offset < len(body); {
		invokeAt := strings.Index(body[offset:], invokeOpen)
		if invokeAt == -1 {
			break
		}
		invokeAt += offset

		openerEnd := strings.IndexByte(body[invokeAt:], '>')
		if openerEnd == -1 {
			raw := body[invokeAt:]
			calls = append(calls, failedToolCall("", nil, raw,
				errors.New("parse invoke: missing opener terminator")))
			break
		}
		openerEnd += invokeAt

		closeAt := strings.Index(body[openerEnd+1:], invokeClose)
		if closeAt == -1 {
			raw := body[invokeAt:]
			calls = append(calls, failedToolCall("", nil, raw,
				errors.New("parse invoke: missing closing marker")))
			break
		}
		closeAt += openerEnd + 1
		callEnd := closeAt + len(invokeClose)

		raw := body[invokeAt:callEnd]
		opener := body[invokeAt : openerEnd+1]
		name, nameErr := attribute(opener, "name")
		args, argsErr := parseParameters(body[openerEnd+1 : closeAt])
		parseErr := errors.Join(nameErr, argsErr)
		if parseErr != nil {
			calls = append(calls, failedToolCall(name, args, raw, parseErr))
		} else {
			calls = append(calls, model.ResponseToolCall{
				ID:   newToolCallID(),
				Type: "function",
				Function: model.ResponseToolCallFunction{
					Name:      name,
					Arguments: args,
				},
			})
		}

		offset = callEnd
	}

	if len(calls) == 0 {
		return []model.ResponseToolCall{failedToolCall("", nil, content,
			errors.New("parse dsml: no invoke elements"))}
	}

	return calls
}

func toolCallsBody(content string) (string, error) {
	openAt := strings.Index(content, toolCallsOpen)
	if openAt == -1 {
		return "", errors.New("parse dsml: missing tool_calls opening marker")
	}

	bodyStart := openAt + len(toolCallsOpen)
	closeAt := strings.Index(content[bodyStart:], toolCallsClose)
	if closeAt == -1 {
		return "", errors.New("parse dsml: missing tool_calls closing marker")
	}

	return content[bodyStart : bodyStart+closeAt], nil
}

func parseParameters(body string) (model.ToolCallArguments, error) {
	args := make(model.ToolCallArguments)
	var errs []error

	for offset := 0; offset < len(body); {
		parameterAt := strings.Index(body[offset:], parameterOpen)
		if parameterAt == -1 {
			break
		}
		parameterAt += offset

		openerEnd := strings.IndexByte(body[parameterAt:], '>')
		if openerEnd == -1 {
			errs = append(errs, errors.New("parse parameter: missing opener terminator"))
			break
		}
		openerEnd += parameterAt

		closeAt := strings.Index(body[openerEnd+1:], parameterClose)
		if closeAt == -1 {
			errs = append(errs, errors.New("parse parameter: missing closing marker"))
			break
		}
		closeAt += openerEnd + 1

		opener := body[parameterAt : openerEnd+1]
		parameterName, nameErr := attribute(opener, "name")
		stringValue, stringErr := attribute(opener, "string")
		if nameErr != nil || stringErr != nil {
			errs = append(errs, errors.Join(nameErr, stringErr))
			offset = closeAt + len(parameterClose)
			continue
		}

		valueText := body[openerEnd+1 : closeAt]
		if _, exists := args[parameterName]; exists {
			errs = append(errs, fmt.Errorf("parse parameter %q: duplicate name", parameterName))
			offset = closeAt + len(parameterClose)
			continue
		}

		switch stringValue {
		case "true":
			args[parameterName] = valueText

		case "false":
			var value any
			if err := json.Unmarshal([]byte(strings.TrimSpace(valueText)), &value); err != nil {
				errs = append(errs, fmt.Errorf("parse parameter %q as JSON: %w", parameterName, err))
			} else {
				args[parameterName] = value
			}

		default:
			errs = append(errs, fmt.Errorf("parse parameter %q: string attribute %q is not true or false",
				parameterName, stringValue))
		}

		offset = closeAt + len(parameterClose)
	}

	return args, errors.Join(errs...)
}

func attribute(opener, name string) (string, error) {
	prefix := name + `="`
	start := strings.Index(opener, prefix)
	if start == -1 {
		return "", fmt.Errorf("parse attributes: missing %q", name)
	}
	start += len(prefix)

	end := strings.IndexByte(opener[start:], '"')
	if end == -1 {
		return "", fmt.Errorf("parse attributes: unterminated %q", name)
	}

	value := opener[start : start+end]
	if value == "" {
		return "", fmt.Errorf("parse attributes: empty %q", name)
	}

	return value, nil
}

func failedToolCall(name string, args model.ToolCallArguments, raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{
		ID:   newToolCallID(),
		Type: "function",
		Function: model.ResponseToolCallFunction{
			Name:      name,
			Arguments: args,
		},
		Status: parseErrorStatus,
		Raw:    raw,
		Error:  err.Error(),
	}
}

func newToolCallID() string {
	return "call_" + uuid.NewString()
}
