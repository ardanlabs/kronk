// Package llama implements Meta Llama's JSON tool-call protocol.
//
// Custom calls use a {"name":...,"parameters":...} envelope without a
// required marker. Some models also prefix calls with <|python_tag|>.
package llama

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "llama"

// Parser implements model.Parser for Meta Llama tool calls.
type Parser struct{}

// New returns a Parser value when the chat template declares Llama's
// name-and-parameters JSON tool-call envelope.
func New(fp model.Fingerprint) (model.Parser, bool) {
	template := fp.ChatTemplate
	if strings.Contains(template, `<|python_tag|>`) {
		return Parser{}, true
	}
	if strings.Contains(template, `Respond in the format {"name": function name, "parameters":`) {
		return Parser{}, true
	}

	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses accumulated Llama JSON tool-call envelopes.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseJSON(ctx, log, buf)
}

// StripToolCallMarkup removes complete and truncated Llama tool-call
// envelopes while preserving unrelated content.
func (Parser) StripToolCallMarkup(buf string) string {
	var output strings.Builder
	var removed bool
	for cursor := 0; cursor < len(buf); {
		start, marked := nextLlamaToolStart(buf, cursor)
		if start < 0 {
			output.WriteString(buf[cursor:])
			break
		}
		output.WriteString(buf[cursor:start])

		objectStart := start
		if marked {
			objectStart += len(pythonTag)
			objectStart = skipLlamaWhitespace(buf, objectStart)
		}
		end := findLlamaObjectEnd(buf[objectStart:])
		if end < 0 {
			if marked || looksLikeLlamaEnvelope(buf[objectStart:]) {
				removed = true
				break
			}
			output.WriteString(buf[start : start+1])
			cursor = start + 1
			continue
		}
		end += objectStart
		if marked {
			removed = true
			cursor = end
			continue
		}
		if _, ok := envelopeName(buf[objectStart:end]); !ok {
			output.WriteString(buf[start : start+1])
			cursor = start + 1
			continue
		}
		removed = true
		cursor = end
	}

	content := output.String()
	if removed {
		content = stripLlamaMarkerPrefix(content)
	}
	return emptyIfWhitespace(content)
}

func stripLlamaMarkerPrefix(content string) string {
	for size := min(len(content), len(pythonTag)-1); size > 0; size-- {
		if strings.HasSuffix(content, pythonTag[:size]) {
			return content[:len(content)-size]
		}
	}
	return content
}

func nextLlamaToolStart(buf string, cursor int) (int, bool) {
	marker := strings.Index(buf[cursor:], pythonTag)
	object := strings.IndexByte(buf[cursor:], '{')
	if marker >= 0 && (object < 0 || marker <= object) {
		return cursor + marker, true
	}
	if object >= 0 {
		return cursor + object, false
	}
	return -1, false
}

func looksLikeLlamaEnvelope(content string) bool {
	return strings.HasPrefix(strings.TrimLeft(content, " \t\r\n"), "{") &&
		strings.Contains(content, `"name"`) && strings.Contains(content, `"parameters"`)
}

func findLlamaObjectEnd(content string) int {
	depth := 0
	inString := false
	escaped := false
	for i := range len(content) {
		char := content[i]
		if escaped {
			escaped = false
			continue
		}
		if inString && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch char {
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

func skipLlamaWhitespace(content string, cursor int) int {
	for cursor < len(content) && strings.ContainsRune(" \t\r\n", rune(content[cursor])) {
		cursor++
	}
	return cursor
}

func emptyIfWhitespace(content string) string {
	if strings.Trim(content, " \t\r\n") == "" {
		return ""
	}
	return content
}
