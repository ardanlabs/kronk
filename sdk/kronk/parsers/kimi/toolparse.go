package kimi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const parseErrorStatus = 2

func parseTools(content string) []model.ResponseToolCall {
	body, err := toolsBody(content)
	if err != nil {
		return []model.ResponseToolCall{failedToolCall("", nil, content, err)}
	}

	var calls []model.ResponseToolCall
	for offset := skipSpace(body, 0); offset < len(body); offset = skipSpace(body, offset) {
		if !strings.HasPrefix(body[offset:], callOpen) {
			calls = append(calls, failedToolCall("", nil, body[offset:],
				errors.New("parse kimi tools: unexpected content outside call")))
			break
		}
		start := offset

		openerEnd := strings.Index(body[start:], sepToken)
		if openerEnd == -1 {
			calls = append(calls, failedToolCall("", nil, body[start:],
				errors.New("parse call: missing opener separator")))
			break
		}
		openerEnd += start + len(sepToken)

		closeAt := strings.Index(body[openerEnd:], callClose)
		if closeAt == -1 {
			calls = append(calls, failedToolCall("", nil, body[start:],
				errors.New("parse call: missing closing marker")))
			break
		}
		closeAt += openerEnd
		callEnd := closeAt + len(callClose)

		raw := body[start:callEnd]
		opener := body[start:openerEnd]
		toolName, nameErr := attribute(opener, "tool")
		indexText, indexErr := attribute(opener, "index")
		if indexErr == nil {
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 1 {
				indexErr = fmt.Errorf("parse call: index %q is not a positive integer", indexText)
			}
		}
		args, argsErr := parseArguments(body[openerEnd:closeAt])
		parseErr := errors.Join(nameErr, indexErr, argsErr)
		if parseErr != nil {
			calls = append(calls, failedToolCall(toolName, args, raw, parseErr))
		} else {
			calls = append(calls, model.ResponseToolCall{
				ID:   newToolCallID(),
				Type: "function",
				Function: model.ResponseToolCallFunction{
					Name:      toolName,
					Arguments: args,
				},
			})
		}

		offset = callEnd
	}

	if len(calls) == 0 {
		return []model.ResponseToolCall{failedToolCall("", nil, content,
			errors.New("parse kimi tools: no call elements"))}
	}

	return calls
}

func toolsBody(content string) (string, error) {
	openAt := strings.Index(content, toolsOpen)
	if openAt == -1 {
		return "", errors.New("parse kimi tools: missing tools opening marker")
	}

	bodyStart := openAt + len(toolsOpen)
	closeAt := strings.Index(content[bodyStart:], toolsClose)
	if closeAt == -1 {
		return "", errors.New("parse kimi tools: missing tools closing marker")
	}

	return content[bodyStart : bodyStart+closeAt], nil
}

func parseArguments(body string) (model.ToolCallArguments, error) {
	args := make(model.ToolCallArguments)
	var errs []error

	for offset := skipSpace(body, 0); offset < len(body); offset = skipSpace(body, offset) {
		if !strings.HasPrefix(body[offset:], argumentOpen) {
			errs = append(errs, errors.New("parse arguments: unexpected content outside argument"))
			break
		}
		start := offset

		openerEnd := strings.Index(body[start:], sepToken)
		if openerEnd == -1 {
			errs = append(errs, errors.New("parse argument: missing opener separator"))
			break
		}
		openerEnd += start + len(sepToken)

		closeAt := strings.Index(body[openerEnd:], argumentClose)
		if closeAt == -1 {
			errs = append(errs, errors.New("parse argument: missing closing marker"))
			break
		}
		closeAt += openerEnd

		opener := body[start:openerEnd]
		key, keyErr := attribute(opener, "key")
		valueType, typeErr := attribute(opener, "type")
		if keyErr != nil || typeErr != nil {
			errs = append(errs, errors.Join(keyErr, typeErr))
			offset = closeAt + len(argumentClose)
			continue
		}

		if _, exists := args[key]; exists {
			errs = append(errs, fmt.Errorf("parse argument %q: duplicate key", key))
			offset = closeAt + len(argumentClose)
			continue
		}

		valueText := body[openerEnd:closeAt]
		value, err := typedValue(valueType, valueText)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse argument %q: %w", key, err))
		} else {
			args[key] = value
		}

		offset = closeAt + len(argumentClose)
	}

	return args, errors.Join(errs...)
}

func typedValue(valueType, text string) (any, error) {
	if valueType == "string" {
		return text, nil
	}

	switch valueType {
	case "boolean", "null", "number", "object", "array":
		var value any
		decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode %s as JSON: %w", valueType, err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, fmt.Errorf("decode %s as JSON: trailing content", valueType)
		}
		if err := validateType(valueType, value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", valueType)
	}
}

func validateType(valueType string, value any) error {
	valid := false
	switch valueType {
	case "boolean":
		_, valid = value.(bool)
	case "null":
		valid = value == nil
	case "number":
		_, valid = value.(json.Number)
	case "object":
		_, valid = value.(map[string]any)
	case "array":
		_, valid = value.([]any)
	}
	if !valid {
		return fmt.Errorf("decoded JSON does not match declared type %q", valueType)
	}
	return nil
}

func attribute(opener, name string) (string, error) {
	prefix := " " + name + `="`
	start := strings.Index(opener, prefix)
	if start == -1 {
		return "", fmt.Errorf("parse attributes: missing %q", name)
	}
	start += len(prefix)

	end := strings.IndexByte(opener[start:], '"')
	if end == -1 {
		return "", fmt.Errorf("parse attributes: unterminated %q", name)
	}

	value := html.UnescapeString(opener[start : start+end])
	if value == "" {
		return "", fmt.Errorf("parse attributes: empty %q", name)
	}
	return value, nil
}

func skipSpace(content string, offset int) int {
	for offset < len(content) {
		switch content[offset] {
		case ' ', '\t', '\r', '\n':
			offset++
		default:
			return offset
		}
	}
	return offset
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

func newToolCallID() string { return "call_" + uuid.NewString() }
