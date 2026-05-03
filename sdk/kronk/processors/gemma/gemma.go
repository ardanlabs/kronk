// Package gemma implements the Processor for Google's Gemma model lineage.
//
// Detection is layered: the GGUF "general.architecture" prefix (e.g.
// "gemma2", "gemma3", "gemma4") is the strongest signal, the chat
// template's distinctive Gemma markers (e.g. <start_of_turn>) is the
// next, and the model name substring is a last-resort legacy fallback.
package gemma

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "gemma"

// Processor implements model.Processor for the Gemma lineage.
type Processor struct{}

// New returns a Processor value if the fingerprint indicates a Gemma model,
// otherwise returns false.
func New(fp model.Fingerprint) (model.Processor, bool) {
	// 1. GGUF architecture prefix: set by the model author and the most
	//    reliable signal of model lineage.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "gemma") {
		return Processor{}, true
	}

	// 2. Chat template markers: identify the prompt format the model was
	//    fine-tuned to emit, which determines reasoning and tool-call
	//    parsing.
	if containsGemmaMarkers(fp.ChatTemplate) {
		return Processor{}, true
	}

	// 3. Model name fallback: legacy GGUFs without rich metadata.
	if strings.Contains(strings.ToLower(fp.ModelName), "gemma") {
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

// ParseToolCall parses Gemma's accumulated tool-call buffer.
func (Processor) ParseToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseGemma(ctx, log, buf)
}

// containsGemmaMarkers reports whether a chat template carries distinctive
// Gemma tokens. Any one is sufficient because no other supported lineage
// uses these exact tokens.
func containsGemmaMarkers(template string) bool {
	for _, marker := range []string{
		"<start_of_turn>",
		"<end_of_turn>",
		"<|channel>",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}
