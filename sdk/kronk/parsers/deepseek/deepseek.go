// Package deepseek implements the Parser for DeepSeek models that use the
// DSML tool-calling protocol.
package deepseek

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	name      = "deepseek"
	dsmlToken = "｜DSML｜"

	toolCallsOpen  = "<" + dsmlToken + "tool_calls>"
	toolCallsClose = "</" + dsmlToken + "tool_calls>"
	invokeOpen     = "<" + dsmlToken + "invoke"
	invokeClose    = "</" + dsmlToken + "invoke>"
	parameterOpen  = "<" + dsmlToken + "parameter"
	parameterClose = "</" + dsmlToken + "parameter>"
)

// Parser implements model.Parser for DeepSeek DSML output.
type Parser struct{}

// New returns a Parser value when the fingerprint identifies DeepSeek or its
// chat template constructs the canonical DSML tool-call protocol.
func New(fp model.Fingerprint) (model.Parser, bool) {
	if containsDSMLMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	if strings.HasPrefix(strings.ToLower(fp.Architecture), "deepseek") {
		return Parser{}, true
	}

	if strings.Contains(strings.ToLower(fp.ModelName), "deepseek") {
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

// ToolCall parses an accumulated DeepSeek DSML tool-call block.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	calls := parseDSML(buf)
	if log == nil {
		return calls
	}

	for i, call := range calls {
		if call.Status == 0 {
			continue
		}
		log(ctx, "parse-dsml", "status", "failed", "index", i,
			"func", call.Function.Name, "error", call.Error, "raw", call.Raw)
	}

	return calls
}

// StripToolCallMarkup removes complete and truncated DeepSeek DSML tool-call
// blocks while preserving all other content.
func (Parser) StripToolCallMarkup(buf string) string {
	var stripped strings.Builder
	var removed bool
	for {
		openAt := strings.Index(buf, toolCallsOpen)
		if openAt == -1 {
			stripped.WriteString(buf)
			break
		}

		stripped.WriteString(buf[:openAt])
		buf = buf[openAt+len(toolCallsOpen):]
		search := buf
		if nextOpen := strings.Index(search, toolCallsOpen); nextOpen >= 0 {
			search = search[:nextOpen]
		}
		closeAt := dsmlToolCallsClose(search)
		if closeAt == -1 {
			removed = true
			break
		}
		removed = true
		buf = buf[closeAt+len(toolCallsClose):]
	}

	result := stripped.String()
	if removed {
		result = stripDSMLMarkerPrefix(result)
	}
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return result
}

func dsmlToolCallsClose(content string) int {
	invokeDepth := 0
	parameterDepth := 0
	for cursor := 0; cursor < len(content); {
		at, marker := nextDSMLStructure(content[cursor:])
		if at == -1 {
			return -1
		}
		at += cursor
		switch marker {
		case invokeOpen:
			invokeDepth++
		case invokeClose:
			invokeDepth = max(invokeDepth-1, 0)
		case parameterOpen:
			parameterDepth++
		case parameterClose:
			parameterDepth = max(parameterDepth-1, 0)
		case toolCallsClose:
			if invokeDepth == 0 && parameterDepth == 0 {
				return at
			}
		}
		cursor = at + len(marker)
	}
	return -1
}

func nextDSMLStructure(content string) (int, string) {
	first := -1
	var marker string
	for _, candidate := range []string{invokeOpen, invokeClose, parameterOpen, parameterClose, toolCallsClose} {
		if at := strings.Index(content, candidate); at != -1 && (first == -1 || at < first) {
			first = at
			marker = candidate
		}
	}
	return first, marker
}

func stripDSMLMarkerPrefix(content string) string {
	for _, marker := range []string{toolCallsOpen, toolCallsClose, invokeOpen, invokeClose, parameterOpen, parameterClose} {
		for size := min(len(content), len(marker)-1); size > 0; size-- {
			if strings.HasSuffix(content, marker[:size]) {
				return content[:len(content)-size]
			}
		}
	}
	return content
}

// containsDSMLMarkers reports whether a template constructs or contains the
// canonical DeepSeek DSML tool-call markers. The token value is fixed by the
// model's vocabulary and is not treated as template-configurable.
func containsDSMLMarkers(template string) bool {
	literal := strings.Contains(template, toolCallsOpen) &&
		strings.Contains(template, invokeOpen) &&
		strings.Contains(template, parameterOpen)
	if literal {
		return true
	}

	assignment := "dsml_token = '" + dsmlToken + "'"
	return strings.Contains(template, assignment) &&
		strings.Contains(template, "dsml_token + 'tool_calls>") &&
		strings.Contains(template, `dsml_token + 'invoke name="`) &&
		strings.Contains(template, `dsml_token + 'parameter name="`)
}
