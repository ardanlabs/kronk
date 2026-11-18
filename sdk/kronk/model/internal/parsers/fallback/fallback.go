// Package fallback implements the catch-all Parser and shared helpers for the
// common <think>...</think> reasoning convention.
//
// The fallback parser is selected when no protocol-specific or model-specific
// parser claims a model. It does not infer tool calls from unmarked content.
package fallback

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// name is the canonical name returned by Parser.Name.
const name = "fallback"

// Parser implements parsers.Parser for otherwise unrecognized models.
type Parser struct{}

// New returns a Parser value. The fallback claims every model and must be
// registered last.
func New(_ parsers.Fingerprint) (parsers.Parser, bool) {
	return Parser{}, true
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() parsers.StateMachine {
	return &stateMachine{status: parsers.ChannelAnswer}
}

// ToolCall returns no calls because the fallback never infers tools from
// unmarked output.
func (Parser) ToolCall(context.Context, applog.Logger, string) []parsers.ResponseToolCall {
	return nil
}
