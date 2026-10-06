// Package nemotron implements the Parser for NVIDIA Nemotron models.
//
// Nemotron 3 Super uses <think> reasoning markers and the Qwen3-Coder direct
// XML tool-call protocol, so parsing delegates to the established Qwen parser.
package nemotron

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/qwen"
)

const name = "nemotron"

// Parser implements model.Parser for NVIDIA Nemotron models.
type Parser struct{}

// New returns a Parser when the fingerprint identifies a Nemotron model.
func New(fp model.Fingerprint) (model.Parser, bool) {
	architecture := strings.ToLower(fp.Architecture)
	modelName := strings.ToLower(fp.ModelName)
	if architecture == "nemotron_h_moe" || strings.Contains(modelName, "nemotron-3-super") {
		return Parser{}, true
	}

	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh Qwen-compatible state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return qwen.Parser{}.NewStateMachine()
}

// ToolCall parses Nemotron's Qwen3-Coder-compatible XML tool calls.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return qwen.Parser{}.ToolCall(ctx, log, buf)
}

// ToolCallWithSchema parses Nemotron tool calls and restores argument types
// from the request's declared tool schema.
func (Parser) ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []model.D) []model.ResponseToolCall {
	return qwen.Parser{}.ToolCallWithSchema(ctx, log, buf, tools)
}
