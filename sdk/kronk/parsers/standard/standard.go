// Package standard implements the catch-all Parser for models that
// emit the most common conventions: <think>...</think> reasoning wraps and
// the OpenAI-style JSON tool-call envelope inside <tool_call>...</tool_call>.
//
// The standard parser is selected when no more-specific parser (gpt, qwen,
// mistral, gemma, glm, …) claims a model. It must be registered last in the
// parser registry so the more specific parsers get first chance to claim.
package standard

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "standard"

// Parser implements model.Parser for the standard catch-all lineage.
type Parser struct{}

// New returns a Parser value. Standard claims any model — it is the
// fallback and must be registered last.
func New(_ model.Fingerprint) (model.Parser, bool) {
	return Parser{}, true
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses the accumulated tool-call buffer as a sequence of
// JSON tool-call objects.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseJSON(ctx, log, buf)
}
