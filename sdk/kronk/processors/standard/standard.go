// Package standard implements the catch-all Processor for models that
// emit the most common conventions: <think>...</think> reasoning wraps and
// the OpenAI-style JSON tool-call envelope inside <tool_call>...</tool_call>.
//
// The standard processor is selected when no more-specific processor (gpt, qwen,
// mistral, gemma, glm, …) claims a model. It must be registered last in the
// processor registry so the more specific processors get first chance to claim.
package standard

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "standard"

// Processor implements model.Processor for the standard catch-all lineage.
type Processor struct{}

// New returns a Processor value. Standard claims any model — it is the
// fallback and must be registered last.
func New(_ model.Fingerprint) (model.Processor, bool) {
	return Processor{}, true
}

// Name returns the processor identifier.
func (Processor) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Processor) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ParseToolCall parses the accumulated tool-call buffer as a sequence of
// JSON tool-call objects.
func (Processor) ParseToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseJSON(ctx, log, buf)
}
