package lfm

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

type valueParser struct {
	text string
	pos  int
}

func parseToolCalls(content string) []model.ResponseToolCall {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []model.ResponseToolCall{failedCall(content, errors.New("parse lfm tools: empty payload"))}
	}
	if strings.HasPrefix(trimmed, toolOpen) {
		var calls []model.ResponseToolCall
		for {
			if !strings.HasPrefix(trimmed, toolOpen) {
				calls = append(calls, failedCall(trimmed, errors.New("parse lfm tools: unexpected content outside tool marker")))
				break
			}
			after := trimmed[len(toolOpen):]
			body, rest, ok := strings.Cut(after, toolClose)
			if !ok {
				calls = append(calls, failedCall(after, errors.New("parse lfm tools: missing closing marker")))
				break
			}
			calls = append(calls, parseToolCalls(body)...)
			trimmed = strings.TrimSpace(rest)
			if trimmed == "" {
				break
			}
		}
		return calls
	}
	if payloads := splitPayloads(trimmed); len(payloads) > 1 {
		var calls []model.ResponseToolCall
		for _, payload := range payloads {
			calls = append(calls, parseToolCalls(payload)...)
		}
		return calls
	}
	if trimmed[0] == '{' || jsonArrayEnvelope(trimmed) {
		return parseJSONCalls(trimmed)
	}
	return parsePythonCalls(trimmed)
}

func splitPayloads(content string) []string {
	var payloads []string
	start := 0
	depth := 0
	for pos := 0; pos < len(content); pos++ {
		switch content[pos] {
		case '\'', '"':
			pos = skipString(content, pos) - 1
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
			if depth == 0 {
				end := pos + 1
				payloads = append(payloads, strings.TrimSpace(content[start:end]))
				start = skipWhitespace(content, end)
				pos = start - 1
			}
		}
	}
	if start < len(content) || depth != 0 {
		return nil
	}
	return payloads
}

func jsonArrayEnvelope(content string) bool {
	if content[0] != '[' {
		return false
	}
	pos := skipWhitespace(content, 1)
	return pos < len(content) && content[pos] == '{'
}

func parseJSONCalls(content string) []model.ResponseToolCall {
	var value any
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return []model.ResponseToolCall{failedCall(content, fmt.Errorf("parse lfm JSON: %w", err))}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return []model.ResponseToolCall{failedCall(content, errors.New("parse lfm JSON: trailing content"))}
	}
	return jsonValueCalls(value, content)
}

func jsonValueCalls(value any, raw string) []model.ResponseToolCall {
	items := []any{value}
	if array, ok := value.([]any); ok {
		items = array
	}
	var calls []model.ResponseToolCall
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			calls = append(calls, failedCall(raw, errors.New("parse lfm JSON: call must be an object")))
			continue
		}
		name, nameOK := object["name"].(string)
		args, argsOK := object["arguments"].(map[string]any)
		if !nameOK || name == "" || !argsOK {
			calls = append(calls, failedCall(raw, errors.New("parse lfm JSON: call requires string name and object arguments")))
			continue
		}
		calls = append(calls, successfulCall(name, args))
	}
	return calls
}

func parsePythonCalls(content string) []model.ResponseToolCall {
	p := valueParser{text: content}
	p.skipSpace()
	bracketed := p.consume('[')
	var calls []model.ResponseToolCall
	for {
		p.skipSpace()
		if bracketed && p.consume(']') {
			break
		}
		start := p.pos
		name, args, err := p.call()
		if err != nil {
			calls = append(calls, failedCall(content[start:], err))
			return calls
		}
		calls = append(calls, successfulCall(name, args))
		p.skipSpace()
		if p.consume(',') {
			continue
		}
		if bracketed && p.consume(']') {
			break
		}
		if bracketed && p.pos == len(p.text) {
			calls = append(calls, failedCall(content, errors.New("parse lfm Python calls: missing closing bracket")))
			break
		}
		if p.pos != len(p.text) {
			calls = append(calls, failedCall(content[p.pos:], errors.New("parse lfm Python calls: unexpected trailing content")))
		}
		break
	}
	p.skipSpace()
	if p.pos != len(p.text) && (len(calls) == 0 || calls[len(calls)-1].Status != parseErrorStatus) {
		calls = append(calls, failedCall(content[p.pos:], errors.New("parse lfm Python calls: trailing content")))
	}
	if len(calls) == 0 {
		return []model.ResponseToolCall{failedCall(content, errors.New("parse lfm Python calls: no calls"))}
	}
	return calls
}

func (p *valueParser) call() (string, model.ToolCallArguments, error) {
	name := p.identifier()
	if name == "" {
		return "", nil, errors.New("parse lfm call: missing function name")
	}
	p.skipSpace()
	if !p.consume('(') {
		return name, nil, errors.New("parse lfm call: missing opening parenthesis")
	}
	args := make(model.ToolCallArguments)
	for {
		p.skipSpace()
		if p.consume(')') {
			return name, args, nil
		}
		key := p.identifier()
		p.skipSpace()
		if key == "" || !p.consume('=') {
			return name, args, errors.New("parse lfm call: expected named argument")
		}
		p.skipSpace()
		value, err := p.value()
		if err != nil {
			return name, args, fmt.Errorf("parse argument %q: %w", key, err)
		}
		if _, exists := args[key]; exists {
			return name, args, fmt.Errorf("parse argument %q: duplicate name", key)
		}
		args[key] = value
		p.skipSpace()
		if p.consume(',') {
			continue
		}
		if !p.consume(')') {
			return name, args, errors.New("parse lfm call: expected comma or closing parenthesis")
		}
		return name, args, nil
	}
}

func (p *valueParser) value() (any, error) {
	if p.pos >= len(p.text) {
		return nil, errors.New("missing value")
	}
	switch p.text[p.pos] {
	case '\'', '"':
		return p.quoted()
	case '[':
		return p.array()
	case '{':
		return p.object()
	}
	start := p.pos
	for p.pos < len(p.text) && !strings.ContainsRune(",)]} \t\r\n", rune(p.text[p.pos])) {
		p.pos++
	}
	word := p.text[start:p.pos]
	switch word {
	case "True", "true":
		return true, nil
	case "False", "false":
		return false, nil
	case "None", "null":
		return nil, nil
	}
	if number, ok := jsonNumber(word); ok {
		return number, nil
	}
	return nil, fmt.Errorf("unsupported bare value %q", word)
}

func (p *valueParser) quoted() (string, error) {
	if p.pos >= len(p.text) || p.text[p.pos] != '\'' && p.text[p.pos] != '"' {
		return "", errors.New("expected quoted string")
	}
	quote := p.text[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.text) {
		ch := p.text[p.pos]
		p.pos++
		if ch == quote {
			return b.String(), nil
		}
		if ch != '\\' {
			b.WriteByte(ch)
			continue
		}
		if p.pos >= len(p.text) {
			return "", errors.New("unterminated escape")
		}
		escaped := p.text[p.pos]
		p.pos++
		switch escaped {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '\'', '"':
			b.WriteByte(escaped)
		default:
			b.WriteByte('\\')
			b.WriteByte(escaped)
		}
	}
	return "", errors.New("unterminated quoted string")
}

func jsonNumber(word string) (json.Number, bool) {
	decoder := json.NewDecoder(strings.NewReader(word))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}
	number, ok := value.(json.Number)
	return number, ok
}

func (p *valueParser) array() ([]any, error) {
	p.pos++
	var values []any
	for {
		p.skipSpace()
		if p.consume(']') {
			return values, nil
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		p.skipSpace()
		if p.consume(',') {
			continue
		}
		if !p.consume(']') {
			return nil, errors.New("array missing closing bracket")
		}
		return values, nil
	}
}

func (p *valueParser) object() (map[string]any, error) {
	p.pos++
	values := make(map[string]any)
	for {
		p.skipSpace()
		if p.consume('}') {
			return values, nil
		}
		key, err := p.quoted()
		if err != nil {
			return nil, errors.New("object key must be quoted")
		}
		p.skipSpace()
		if !p.consume(':') {
			return nil, errors.New("object key missing colon")
		}
		p.skipSpace()
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		values[key] = value
		p.skipSpace()
		if p.consume(',') {
			continue
		}
		if !p.consume('}') {
			return nil, errors.New("object missing closing brace")
		}
		return values, nil
	}
}

func (p *valueParser) identifier() string {
	if p.pos >= len(p.text) || !isIdentStart(p.text[p.pos]) {
		return ""
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.text) && isIdent(p.text[p.pos]) {
		p.pos++
	}
	return p.text[start:p.pos]
}

func (p *valueParser) skipSpace() { p.pos = skipWhitespace(p.text, p.pos) }
func (p *valueParser) consume(ch byte) bool {
	if p.pos < len(p.text) && p.text[p.pos] == ch {
		p.pos++
		return true
	}
	return false
}

func skipWhitespace(text string, pos int) int {
	for pos < len(text) && strings.ContainsRune(" \t\r\n", rune(text[pos])) {
		pos++
	}
	return pos
}
func isIdentStart(ch byte) bool { return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' }
func isIdent(ch byte) bool      { return isIdentStart(ch) || ch >= '0' && ch <= '9' }

func successfulCall(name string, args model.ToolCallArguments) model.ResponseToolCall {
	return model.ResponseToolCall{ID: "call_" + uuid.NewString(), Type: "function", Function: model.ResponseToolCallFunction{Name: name, Arguments: args}}
}
func failedCall(raw string, err error) model.ResponseToolCall {
	return model.ResponseToolCall{ID: "call_" + uuid.NewString(), Type: "function", Status: parseErrorStatus, Raw: raw, Error: err.Error()}
}
