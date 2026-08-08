package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

const parseErrorStatus = 2

func parseDSML(content string) []model.ResponseToolCall {
	var calls []model.ResponseToolCall
	raw := content
	remaining := strings.TrimSpace(content)
	if remaining == "" {
		return []model.ResponseToolCall{failedToolCall("", nil, raw,
			errors.New("parse dsml: tool call is empty"))}
	}

	for remaining != "" {
		body, next, err := nextToolCallsBody(remaining, 0)
		if err != nil {
			return []model.ResponseToolCall{failedToolCall("", nil, raw, err)}
		}
		bodyCalls := parseDSMLBody(body)
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

func parseDSMLBody(body string) []model.ResponseToolCall {
	var calls []model.ResponseToolCall
	for offset := skipDSMLSpace(body, 0); offset < len(body); offset = skipDSMLSpace(body, offset) {
		if !hasDSMLElementPrefix(body[offset:], invokeOpen) {
			return []model.ResponseToolCall{failedToolCall("", nil, body[offset:],
				errors.New("parse dsml: unexpected content outside invoke"))}
		}
		invokeAt := offset

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
		attributes, attributeErr := parseElementAttributes(opener, invokeOpen, ">", "name")
		name := attributes["name"]
		args, argsErr := parseParameters(body[openerEnd+1 : closeAt])
		parseErr := errors.Join(attributeErr, argsErr)
		if parseErr != nil {
			return []model.ResponseToolCall{failedToolCall(name, args, raw, parseErr)}
		}
		calls = append(calls, model.ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: model.ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		offset = callEnd
	}

	if len(calls) == 0 {
		return []model.ResponseToolCall{failedToolCall("", nil, body,
			errors.New("parse dsml: no invoke elements"))}
	}

	return calls
}

func nextToolCallsBody(content string, offset int) (string, int, error) {
	if offset < 0 || offset > len(content) || !strings.HasPrefix(content[offset:], toolCallsOpen) {
		return "", offset, errors.New("parse dsml: missing tool_calls opening marker")
	}
	openAt := offset

	bodyStart := openAt + len(toolCallsOpen)
	closeAt := strings.Index(content[bodyStart:], toolCallsClose)
	if closeAt == -1 {
		return "", offset, errors.New("parse dsml: missing tool_calls closing marker")
	}
	closeAt += bodyStart

	return content[bodyStart:closeAt], closeAt + len(toolCallsClose), nil
}

func parseParameters(body string) (model.ToolCallArguments, error) {
	args := make(model.ToolCallArguments)

	for offset := skipDSMLSpace(body, 0); offset < len(body); offset = skipDSMLSpace(body, offset) {
		if !hasDSMLElementPrefix(body[offset:], parameterOpen) {
			return nil, errors.New("parse parameters: unexpected content outside parameter")
		}
		parameterAt := offset

		openerEnd := strings.IndexByte(body[parameterAt:], '>')
		if openerEnd == -1 {
			return nil, errors.New("parse parameter: missing opener terminator")
		}
		openerEnd += parameterAt

		closeAt := strings.Index(body[openerEnd+1:], parameterClose)
		if closeAt == -1 {
			return nil, errors.New("parse parameter: missing closing marker")
		}
		closeAt += openerEnd + 1

		opener := body[parameterAt : openerEnd+1]
		attributes, err := parseElementAttributes(opener, parameterOpen, ">", "name", "string")
		if err != nil {
			return nil, err
		}
		parameterName := attributes["name"]
		stringValue := attributes["string"]

		valueText := body[openerEnd+1 : closeAt]
		if _, exists := args[parameterName]; exists {
			return nil, fmt.Errorf("parse parameter %q: duplicate name", parameterName)
		}

		switch stringValue {
		case "true":
			args[parameterName] = valueText

		case "false":
			value, err := decodeUniqueDSMLJSON(strings.TrimSpace(valueText))
			if err != nil {
				return nil, fmt.Errorf("parse parameter %q as JSON: %w", parameterName, err)
			}
			args[parameterName] = value

		default:
			return nil, fmt.Errorf("parse parameter %q: string attribute %q is not true or false",
				parameterName, stringValue)
		}

		offset = closeAt + len(parameterClose)
	}

	return args, nil
}

func skipDSMLSpace(content string, offset int) int {
	for offset < len(content) && strings.ContainsRune(" \t\r\n", rune(content[offset])) {
		offset++
	}
	return offset
}

func decodeUniqueDSMLJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	value, err := decodeUniqueDSMLJSONValue(decoder)
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

func decodeUniqueDSMLJSONValue(decoder *json.Decoder) (any, error) {
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
			value, err := decodeUniqueDSMLJSONValue(decoder)
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
			value, err := decodeUniqueDSMLJSONValue(decoder)
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
		offset = skipDSMLSpace(opener, offset)
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
		attributes[name] = opener[offset:valueEnd]
		offset = valueEnd + 1
	}
	for _, name := range names {
		if _, ok := attributes[name]; !ok {
			return nil, fmt.Errorf("parse attributes: missing %q", name)
		}
	}
	return attributes, nil
}

func hasDSMLElementPrefix(content, marker string) bool {
	if !strings.HasPrefix(content, marker) || len(content) == len(marker) {
		return false
	}
	next := content[len(marker)]
	return next == '>' || strings.ContainsRune(" \t\r\n", rune(next))
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
