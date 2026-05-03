// Package qwen implements the Processor for Qwen and Qwen-Coder models.
//
// Qwen models emit reasoning between <think>...</think> tags and tool calls
// in one of two formats:
//   - JSON envelope: <tool_call>{"name":"x","arguments":{…}}</tool_call>
//   - Direct XML:    <function=x>\n<parameter=k>\nv\n</parameter>\n</function>
//
// Some Qwen-Coder variants tokenize the direct-XML opener as separate tokens
// ("<", "function", "="), so the stateMachine carries a small lookahead buffer
// to detect the split <function=... pattern.
package qwen

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "qwen"

// Processor implements model.Processor for Qwen.
type Processor struct{}

// New returns a Processor value if the fingerprint indicates a Qwen model,
// otherwise returns false. Detection is layered: GGUF
// "general.architecture" prefix (e.g. "qwen2", "qwen3", "qwen35moe") is
// the strongest signal, the chat template's distinctive Qwen tool-call
// markers (<function=, <parameter=) is the next, and the model name
// substring is a last-resort legacy fallback.
func New(fp model.Fingerprint) (model.Processor, bool) {
	// 1. GGUF architecture prefix.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "qwen") {
		return Processor{}, true
	}

	// 2. Chat template markers distinctive to Qwen tool calls.
	if containsQwenMarkers(fp.ChatTemplate) {
		return Processor{}, true
	}

	// 3. Model name fallback.
	if strings.Contains(strings.ToLower(fp.ModelName), "qwen") {
		return Processor{}, true
	}

	return Processor{}, false
}

// Name returns the processor identifier.
func (Processor) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Processor) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ParseToolCall parses Qwen's accumulated tool-call buffer. The buffer
// content varies by emission format (JSON envelope vs direct XML), so the
// parser inspects the leading bytes to choose between them.
func (Processor) ParseToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	trimmed := strings.TrimLeft(buf, " \t\n\r")

	// Direct <function=…> XML format.
	if strings.HasPrefix(trimmed, "<function=") {
		return parseQwenXML(buf)
	}

	// JSON envelope is the default Qwen tool-call format.
	return parseJSON(ctx, log, buf)
}

// containsQwenMarkers reports whether a chat template carries distinctive
// Qwen tool-call tokens. The <function= and <parameter= openers are
// specific to Qwen's direct-XML tool-call format and unlikely to appear
// in any other lineage's template.
func containsQwenMarkers(template string) bool {
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
