// Package fallback implements the catch-all Parser and shared helpers for the
// common <think>...</think> reasoning convention.
//
// The fallback parser is selected when no protocol-specific or model-specific
// parser claims a model. It does not infer tool calls from unmarked content.
package fallback

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "fallback"

// Parser implements model.Parser for otherwise unrecognized models.
type Parser struct{}

// New returns a Parser value. The fallback claims every model and must be
// registered last.
func New(_ model.Fingerprint) (model.Parser, bool) {
	return Parser{}, true
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall returns no calls because the fallback never infers tools from
// unmarked output.
func (Parser) ToolCall(context.Context, applog.Logger, string) []model.ResponseToolCall {
	return nil
}

// StripToolCallMarkup preserves output because the fallback parser does not
// recognize model-native tool-call markup.
func (Parser) StripToolCallMarkup(buf string) string {
	return buf
}
