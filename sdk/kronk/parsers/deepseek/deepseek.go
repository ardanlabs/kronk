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
