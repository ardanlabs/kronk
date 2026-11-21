// Package inkling implements Inkling's message-block response protocol.
// Reasoning and answers stream independently; native JSON tool frames are
// buffered and validated atomically before conversion to OpenAI tool calls.
package inkling

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

const (
	messageModel = "<|message_model|>"
	thinking     = "<|content_thinking|>"
	text         = "<|content_text|>"
	invoke       = "<|content_invoke_tool_json|>"
	endMessage   = "<|end_message|>"
	endSampling  = "<|content_model_end_sampling|>"
)

// Parser implements parsers.Parser for Inkling.
type Parser struct{}

// New returns a parser for Inkling 2 architectures or an Inkling template.
func New(fp parsers.Fingerprint) (parsers.Parser, bool) {
	if strings.EqualFold(fp.Architecture, "inkling2") || strings.EqualFold(fp.Architecture, "inkling2_loop") ||
		strings.Contains(fp.ChatTemplate, messageModel) &&
			strings.Contains(fp.ChatTemplate, text) &&
			strings.Contains(fp.ChatTemplate, endSampling) {
		return Parser{}, true
	}
	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return "inkling" }

// NewStateMachine returns independent, slot-owned response state.
func (Parser) NewStateMachine() parsers.StateMachine {
	sm := &stateMachine{}
	sm.Reset()
	return sm
}

// ToolCall validates native frames and converts args into function arguments.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []parsers.ResponseToolCall {
	return parseToolCalls(buf)
}
