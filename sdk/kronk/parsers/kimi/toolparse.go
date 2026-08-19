package kimi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const parseErrorStatus = 2

func parseTools(content string) []model.ResponseToolCall {
	var calls []model.ResponseToolCall
	raw := content
	remaining := strings.TrimSpace(content)
	if remaining == "" {
		return []model.ResponseToolCall{failedToolCall("", nil, raw,
			errors.New("parse kimi tools: tool call is empty"))}
	}

	for remaining != "" {
		body, next, err := nextToolsBody(remaining, 0)
		if err != nil {
			return []model.ResponseToolCall{failedToolCall("", nil, raw, err)}
		}
		bodyCalls := parseToolsBody(body)
		for _, call := range bodyCalls {
			if call.Status != 0 {
				return []model.ResponseToolCall{failedToolCall("", nil, raw,
					errors.New(call.Error))}
			}
		}
		calls = append(calls, bodyCalls...)
		remaining = strings.TrimSpace(remaining[next:])
	}

	return calls
}

func parseToolsBody(body string) []model.ResponseToolCall {
	var calls []model.ResponseToolCall
	for offset := skipSpace(body, 0); offset < len(body); offset = skipSpace(body, offset) {
		if !hasKimiElementPrefix(body[offset:], callOpen) {
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
		attributes, attributeErr := parseElementAttributes(opener, callOpen, sepToken, "tool", "index")
		toolName := attributes["tool"]
		indexText := attributes["index"]
		indexErr := attributeErr
		if indexErr == nil {
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 1 {
				indexErr = fmt.Errorf("parse call: index %q is not a positive integer", indexText)
			}
		}
		args, argsErr := parseArguments(body[openerEnd:closeAt])
		parseErr := errors.Join(indexErr, argsErr)
		if parseErr != nil {
			return []model.ResponseToolCall{failedToolCall(toolName, args, raw, parseErr)}
		}
		calls = append(calls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      toolName,
				Arguments: args,
			},
		})

		offset = callEnd
	}

	if len(calls) == 0 {
		return []model.ResponseToolCall{failedToolCall("", nil, body,
			errors.New("parse kimi tools: no call elements"))}
	}

	return calls
}

func nextToolsBody(content string, offset int) (string, int, error) {
	if offset < 0 || offset > len(content) || !strings.HasPrefix(content[offset:], toolsOpen) {
		return "", offset, errors.New("parse kimi tools: missing tools opening marker")
	}
	openAt := offset

	bodyStart := openAt + len(toolsOpen)
	closeAt := strings.Index(content[bodyStart:], toolsClose)
	if closeAt == -1 {
		return "", offset, errors.New("parse kimi tools: missing tools closing marker")
	}
	closeAt += bodyStart

	return content[bodyStart:closeAt], closeAt + len(toolsClose), nil
}

func parseArguments(body string) (model.ToolCallArguments, error) {
	args := make(model.ToolCallArguments)
	var errs []error

	for offset := skipSpace(body, 0); offset < len(body); offset = skipSpace(body, offset) {
		if !hasKimiElementPrefix(body[offset:], argumentOpen) {
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
		attributes, err := parseElementAttributes(opener, argumentOpen, sepToken, "key", "type")
		if err != nil {
			errs = append(errs, err)
			offset = closeAt + len(argumentClose)
			continue
		}
		key := attributes["key"]
		valueType := attributes["type"]

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
		value, err := decodeUniqueKimiJSON(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("decode %s as JSON: %w", valueType, err)
		}
		if err := validateType(valueType, value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", valueType)
	}
}

func decodeUniqueKimiJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueKimiJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("unexpected data after JSON value")
		}
		return nil, err
	}
	return value, nil
}

func decodeUniqueKimiJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
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
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeUniqueKimiJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
			return nil, errors.New("JSON object is not closed")
		}
		return object, nil

	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeUniqueKimiJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			return nil, errors.New("JSON array is not closed")
		}
		return array, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
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

func parseElementAttributes(opener, marker, terminator string, names ...string) (map[string]string, error) {
	if !strings.HasPrefix(opener, marker) {
		return nil, errors.New("parse attributes: invalid element marker")
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	attributes := make(map[string]string, len(names))
	for offset := len(marker); ; {
		spaceAt := offset
		offset = skipSpace(opener, offset)
		if strings.HasPrefix(opener[offset:], terminator) && offset+len(terminator) == len(opener) {
			break
		}
		if offset == spaceAt {
			return nil, errors.New("parse attributes: expected structural whitespace")
		}
		nameEnd := strings.IndexByte(opener[offset:], '=')
		if nameEnd <= 0 {
			return nil, errors.New("parse attributes: invalid attribute name")
		}
		nameEnd += offset
		name := opener[offset:nameEnd]
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("parse attributes: unexpected %q", name)
		}
		if _, exists := attributes[name]; exists {
			return nil, fmt.Errorf("parse attributes: duplicate %q", name)
		}
		offset = nameEnd + 1
		if offset >= len(opener) || opener[offset] != '"' {
			return nil, fmt.Errorf("parse attributes: %q value is not quoted", name)
		}
		offset++
		valueEnd := strings.IndexByte(opener[offset:], '"')
		if valueEnd == -1 {
			return nil, fmt.Errorf("parse attributes: unterminated %q", name)
		}
		valueEnd += offset
		if valueEnd == offset {
			return nil, fmt.Errorf("parse attributes: empty %q", name)
		}
		attributes[name] = html.UnescapeString(opener[offset:valueEnd])
		offset = valueEnd + 1
	}
	for _, name := range names {
		if _, ok := attributes[name]; !ok {
			return nil, fmt.Errorf("parse attributes: missing %q", name)
		}
	}
	return attributes, nil
}

func hasKimiElementPrefix(content, marker string) bool {
	if !strings.HasPrefix(content, marker) {
		return false
	}
	remainder := content[len(marker):]
	return strings.HasPrefix(remainder, sepToken) ||
		(len(remainder) > 0 && strings.ContainsRune(" \t\r\n", rune(remainder[0])))
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

func newToolCallID() string { return "call_" + uuid.New().String() }
