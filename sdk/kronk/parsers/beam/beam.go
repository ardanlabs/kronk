// Package beam implements Reflection Beam's Harmony framing and XML tool calls.
// It separates analysis from final answers and converts rfl:parameter bodies
// into OpenAI tool arguments using the declared tool schema. Strings remain
// raw text, not XML-decoded text. Tool calls are buffered until final validation.
//
// Beam always reasons. Its template accepts minimal/low/medium/high/xhigh/max;
// omitted reasoning effort keeps the template's default of max. Clients should
// preserve reasoning_content in assistant history for subsequent tool rounds.
package beam

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Parser implements model.Parser for Beam.
type Parser struct{}

// New returns a parser for Beam architecture or an XML-Harmony chat template.
func New(fp model.Fingerprint) (model.Parser, bool) {
	if strings.EqualFold(fp.Architecture, "beam") ||
		strings.Contains(fp.ChatTemplate, "<|channel|>") &&
			strings.Contains(fp.ChatTemplate, "<|constrain|>xml") &&
			strings.Contains(fp.ChatTemplate, "rfl:parameter") {
		return Parser{}, true
	}
	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return "beam" }

// NewStateMachine returns an independent Harmony XML stream classifier.
func (Parser) NewStateMachine() model.StateMachine {
	sm := &stateMachine{}
	sm.Reset()
	return sm
}

// ToolCall parses accumulated XML-Harmony frames, preserving untyped values as strings.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseToolCalls(buf, nil)
}

// ToolCallWithSchema recovers parameter types using the request's tool declarations.
func (Parser) ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []model.D) []model.ResponseToolCall {
	return parseToolCalls(buf, tools)
}

// AdjustParams maps disabled reasoning to Beam's smallest supported budget.
func (Parser) AdjustParams(params model.Params) model.Params {
	if params.ReasoningEffort == model.ReasoningEffortNone {
		params.ReasoningEffort = model.ReasoningEffortMinimal
	}
	return params
}
