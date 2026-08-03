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
