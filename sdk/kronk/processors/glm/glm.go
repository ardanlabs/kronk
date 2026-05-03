// Package glm implements the Processor for GLM models.
//
// GLM uses <think>...</think> for reasoning and a <tool_call>NAME<arg_key>K</arg_key><arg_value>V</arg_value>...</tool_call>
// envelope for tool calls. The stateMachine is a slimmed standard envelope;
// the parser walks the <arg_key>/<arg_value> tag pairs to build the
// arguments map.
package glm

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "glm"

// Processor implements model.Processor for GLM.
type Processor struct{}

// New returns a Processor value if the fingerprint indicates GLM, otherwise
// returns false. Detection is layered: GGUF "general.architecture" prefix
// (e.g. "glm", "chatglm") is the strongest signal, the chat template's
// distinctive GLM markers is the next, and the model name substring is a
// last-resort legacy fallback.
func New(fp model.Fingerprint) (model.Processor, bool) {
	// 1. GGUF architecture prefix.
	arch := strings.ToLower(fp.Architecture)
	if strings.HasPrefix(arch, "glm") || strings.HasPrefix(arch, "chatglm") {
		return Processor{}, true
	}

	// 2. Chat template markers distinctive to GLM tool calls.
	if containsGLMMarkers(fp.ChatTemplate) {
		return Processor{}, true
	}

	// 3. Model name fallback.
	if strings.Contains(strings.ToLower(fp.ModelName), "glm") {
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

// ParseToolCall parses GLM's accumulated tool-call buffer.
func (Processor) ParseToolCall(_ context.Context, _ applog.Logger, buf string) []model.ResponseToolCall {
	return parseGLM(buf)
}

// containsGLMMarkers reports whether a chat template carries distinctive
// GLM tool-call tokens. The <arg_key>/<arg_value> pair is unique to GLM's
// tool-call format and unlikely to appear in any other lineage's template.
func containsGLMMarkers(template string) bool {
	for _, marker := range []string{
		"<arg_key>",
		"<arg_value>",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}
