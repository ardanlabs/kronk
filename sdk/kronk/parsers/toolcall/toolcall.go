// Package toolcall implements the marked JSON tool-call protocol.
//
// The protocol wraps an OpenAI-style JSON function envelope in explicit
// <tool_call>...</tool_call> markers. It is selected from the chat template,
// independent of the model's base architecture.
package toolcall

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "tool-call-json"

// Parser implements model.Parser for marked JSON tool calls.
type Parser struct{}

// New returns a Parser value when the chat template declares marked JSON
// tool calls.
func New(fp model.Fingerprint) (model.Parser, bool) {
	template := fp.ChatTemplate
	if !strings.Contains(template, "<tool_call>") || !strings.Contains(template, "</tool_call>") {
		return Parser{}, false
	}
	if !strings.Contains(template, `"name"`) || !strings.Contains(template, `"arguments"`) {
		return Parser{}, false
	}

	return Parser{}, true
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses the accumulated JSON tool-call envelopes.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseJSON(ctx, log, buf)
}

// StripToolCallMarkup removes complete and truncated marked-JSON tool-call
// envelopes while preserving unrelated content.
func (Parser) StripToolCallMarkup(buf string) string {
	if boundary := markedEvidenceIndex(buf, "<unexpected-content-after-tool>"); boundary >= 0 {
		before := buf[:boundary]
		after := buf[boundary+len("<unexpected-content-after-tool>"):]
		return emptyMarkedToolWhitespace(stripMarkedToolRound(before) + stripMarkedToolPrefix(after))
	}

	return emptyMarkedToolWhitespace(stripMarkedToolRound(buf))
}

func stripMarkedToolRound(buf string) string {
	for _, marker := range []string{"<invalid-tool-call-boundary>", "<missing-tool-call-close>"} {
		if at := markedEvidenceIndex(buf, marker); at >= 0 {
			suffix := buf[at+len(marker):]
			if strings.TrimSpace(suffix) == "" {
				return suffix
			}
			return ""
		}
	}
	if strings.TrimSpace(buf) == "<tool_call>" || strings.TrimSpace(buf) == "<empty-tool-call>" {
		return ""
	}
	return stripMarkedJSONToolCalls(buf)
}

func markedEvidenceIndex(content string, marker string) int {
	inString := false
	escaped := false
	for i := 0; i < len(content); i++ {
		if escaped {
			escaped = false
			continue
		}
		if inString && content[i] == '\\' {
			escaped = true
			continue
		}
		if content[i] == '"' {
			inString = !inString
			continue
		}
		if !inString && strings.HasPrefix(content[i:], marker) {
			return i
		}
	}
	return -1
}

func stripMarkedJSONToolCalls(buf string) string {
	var output strings.Builder
	for cursor := 0; cursor < len(buf); {
		start := strings.IndexByte(buf[cursor:], '{')
		markerAt, marker := nextToolCallEvidenceMarker(buf[cursor:])
		if markerAt >= 0 && (start < 0 || markerAt < start) {
			output.WriteString(buf[cursor : cursor+markerAt])
			cursor += markerAt + len(marker)
			continue
		}
		if start < 0 {
			output.WriteString(buf[cursor:])
			break
		}
		start += cursor
		output.WriteString(buf[cursor:start])

		end := findJSONObjectEnd(buf[start:])
		if end < 0 {
			remaining := buf[start:]
			if looksLikeToolCallEnvelope(remaining) || strings.Contains(remaining, "<missing-tool-call-close>") || strings.Contains(remaining, "<invalid-tool-call-boundary>") {
				break
			}
			output.WriteByte(buf[start])
			cursor = start + 1
			continue
		}
		end += start
		if _, err := decodeFunction(buf[start:end]); err != nil && !looksLikeToolCallEnvelope(buf[start:end]) {
			output.WriteString(buf[start:end])
			cursor = end
			continue
		}
		cursor = end
	}

	return output.String()
}

func emptyMarkedToolWhitespace(content string) string {
	if strings.Trim(content, " \t\r\n") == "" {
		return ""
	}
	return content
}

func stripMarkedToolPrefix(content string) string {
	for _, marker := range append(append([]string{}, openMarkers...), closeMarkers...) {
		for size := min(len(content), len(marker)-1); size > 0; size-- {
			if strings.HasSuffix(content, marker[:size]) {
				return content[:len(content)-size]
			}
		}
	}
	return content
}

func looksLikeToolCallEnvelope(content string) bool {
	return strings.Contains(content, `"name"`) && strings.Contains(content, `"arguments"`)
}

func nextToolCallEvidenceMarker(content string) (int, string) {
	markerAt := -1
	found := ""
	for _, marker := range []string{
		"<tool_call>",
		"<empty-tool-call>",
		"<invalid-tool-call-boundary>",
		"<missing-tool-call-close>",
		"<unexpected-content-after-tool>",
	} {
		if at := strings.Index(content, marker); at >= 0 && (markerAt == -1 || at < markerAt) {
			markerAt = at
			found = marker
		}
	}
	return markerAt, found
}
