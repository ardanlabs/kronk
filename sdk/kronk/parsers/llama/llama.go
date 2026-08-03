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
