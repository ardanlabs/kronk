// Package mistral implements the Processor for Mistral and Devstral
// models, which emit reasoning between <think>...</think> tags and tool
// calls in the streaming [TOOL_CALLS]name[ARGS]{...} format.
//
// Unlike the JSON-envelope processors (qwen, standard), Mistral does not
// surround tool calls with explicit close tags — the model emits tokens
// until end-of-generation, and the buffered [TOOL_CALLS]/[ARGS] payload is
// parsed at finish time.
package mistral

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "mistral"

// Processor implements model.Processor for Mistral and Devstral.
type Processor struct{}

// New returns a Processor value if the fingerprint indicates Mistral or
// Devstral, otherwise returns false. Detection is layered: GGUF
// "general.architecture" prefix (e.g. "mistral") is the strongest signal,
// the chat template's distinctive Mistral tool-call markers ([TOOL_CALLS],
// [ARGS]) is the next, and the model name substring is a last-resort
// legacy fallback.
func New(fp model.Fingerprint) (model.Processor, bool) {
	// 1. GGUF architecture prefix.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "mistral") {
		return Processor{}, true
	}

	// 2. Chat template markers distinctive to Mistral tool calls.
	if containsMistralMarkers(fp.ChatTemplate) {
		return Processor{}, true
	}

	// 3. Model name fallback.
	mn := strings.ToLower(fp.ModelName)
	if strings.Contains(mn, "mistral") || strings.Contains(mn, "devstral") {
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

// ParseToolCall parses Mistral's [TOOL_CALLS]name[ARGS]{...} buffer into
// structured tool calls.
func (Processor) ParseToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseMistral(ctx, log, buf)
}

// containsMistralMarkers reports whether a chat template carries
// distinctive Mistral tool-call tokens. [TOOL_CALLS] and [ARGS] are
// specific to Mistral's streaming tool-call format and unlikely to
// appear in any other lineage's template.
func containsMistralMarkers(template string) bool {
	for _, marker := range []string{
		"[TOOL_CALLS]",
		"[ARGS]",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}
