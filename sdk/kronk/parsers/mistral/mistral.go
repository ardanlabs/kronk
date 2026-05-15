// Package mistral implements the Parser for Mistral and Devstral
// models, which emit reasoning between <think>...</think> or
// [THINK]...[/THINK] tags and tool calls in the streaming
// [TOOL_CALLS]name[ARGS]{...} format.
//
// Unlike the JSON-envelope parsers (qwen, standard), Mistral does not
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

// name is the canonical name returned by Parser.Name.
const name = "mistral"

// strictReasoningEffortMarker is the validation expression emitted by
// Mistral Medium 3.5+ chat templates that restrict reasoning_effort to
// "none" or "high". Matching the expression is more robust than matching
// the human-readable exception message: it survives wording changes to the
// raise_exception(...) text. Detected at parser construction so
// AdjustParams can coerce any other value (e.g. the global default
// "medium") into a valid one.
const strictReasoningEffortMarker = `reasoning_effort not in ['none', 'high']`

// Parser implements model.Parser for Mistral and Devstral.
type Parser struct {
	// strictReasoningEffort is true when the model's chat template only
	// accepts reasoning_effort values "none" or "high". When set,
	// AdjustParams coerces any other value to "high".
	strictReasoningEffort bool
}

// New returns a Parser value if the fingerprint indicates Mistral or
// Devstral, otherwise returns false. Detection is layered: GGUF
// "general.architecture" prefix (e.g. "mistral") is the strongest signal,
// the chat template's distinctive Mistral tool-call markers ([TOOL_CALLS],
// [ARGS]) is the next, and the model name substring is a last-resort
// legacy fallback.
func New(fp model.Fingerprint) (model.Parser, bool) {
	matched := false

	switch {
	// 1. GGUF architecture prefix.
	case strings.HasPrefix(strings.ToLower(fp.Architecture), "mistral"):
		matched = true

	// 2. Chat template markers distinctive to Mistral tool calls.
	case containsMistralMarkers(fp.ChatTemplate):
		matched = true

	// 3. Model name fallback.
	default:
		mn := strings.ToLower(fp.ModelName)
		if strings.Contains(mn, "mistral") || strings.Contains(mn, "devstral") {
			matched = true
		}
	}

	if !matched {
		return Parser{}, false
	}

	return Parser{
		strictReasoningEffort: strings.Contains(fp.ChatTemplate, strictReasoningEffortMarker),
	}, true
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses Mistral's [TOOL_CALLS]name[ARGS]{...} buffer into
// structured tool calls.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseMistral(ctx, log, buf)
}

// AdjustParams coerces request Params into values the model's chat template
// will accept. For templates that restrict reasoning_effort to "none" or
// "high" (Mistral Medium 3.5+), any other value is coerced to "high" so the
// model performs reasoning rather than silently disabling it.
func (p Parser) AdjustParams(params model.Params) model.Params {
	if p.strictReasoningEffort {
		switch params.ReasoningEffort {
		case model.ReasoningEffortNone, model.ReasoningEffortHigh:
			// Already a valid value.
		default:
			params.ReasoningEffort = model.ReasoningEffortHigh
		}
	}

	return params
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
