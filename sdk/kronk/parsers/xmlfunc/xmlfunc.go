// Package xmlfunc implements the Parser for models that emit tool calls
// using the <function=…><parameter=…> XML envelope. This format was
// popularized by Qwen-Coder and has since been adopted by other lineages
// (NVIDIA Nemotron, etc.), so the parser is named after the wire format
// rather than any single model family.
//
// Models in this lineage emit reasoning between <think>...</think> tags
// and tool calls in one of two formats:
//   - JSON envelope: <tool_call>{"name":"x","arguments":{…}}</tool_call>
//   - Direct XML:    <function=x>\n<parameter=k>\nv\n</parameter>\n</function>
//
// Some Qwen-Coder variants tokenize the direct-XML opener as separate
// tokens ("<", "function", "="), so the stateMachine carries a small
// lookahead buffer to detect the split <function=... pattern.
package xmlfunc

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "xmlfunc"

// Parser implements model.Parser for the <function=…> XML tool-call format.
type Parser struct{}

// New returns a Parser value if the fingerprint indicates a model that
// emits the <function=…><parameter=…> XML tool-call format, otherwise
// returns false. Detection is layered: the chat template's distinctive
// XML tool-call markers are the strongest signal (any model whose
// template contains <function= or <parameter= speaks this dialect),
// followed by the GGUF "general.architecture" prefix for known Qwen
// builds and the model-name substring as a last-resort legacy fallback.
func New(fp model.Fingerprint) (model.Parser, bool) {
	// 1. Chat template markers distinctive to the XML tool-call format.
	if containsXMLFuncMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	// 2. GGUF architecture prefix for known Qwen builds.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "qwen") {
		return Parser{}, true
	}

	// 3. Model name fallback.
	if strings.Contains(strings.ToLower(fp.ModelName), "qwen") {
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

// ToolCall parses the accumulated tool-call buffer. The buffer content
// varies by emission format (JSON envelope vs direct XML), so the parser
// inspects the leading bytes to choose between them.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	trimmed := strings.TrimLeft(buf, " \t\n\r")

	// Direct <function=…> XML format.
	if strings.HasPrefix(trimmed, "<function=") {
		return parseXMLFunc(ctx, log, buf)
	}

	// JSON envelope is the alternate tool-call format.
	return parseJSON(ctx, log, buf)
}

// containsXMLFuncMarkers reports whether a chat template carries the
// distinctive <function= / <parameter= openers used by the direct-XML
// tool-call format. These markers are unlikely to appear in any other
// lineage's template.
func containsXMLFuncMarkers(template string) bool {
	for _, marker := range []string{
		"<function=",
		"<parameter=",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}
