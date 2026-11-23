// Package parsers defines the internal contract shared by built-in model parsers.
// It is independent of the public model package; model owns boundary conversions.
package parsers

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// Channel identifies the destination for classified content.
type Channel uint8

const (
	// ChannelNone identifies structural markers that are not surfaced.
	ChannelNone Channel = iota
	// ChannelReasoning identifies reasoning content.
	ChannelReasoning
	// ChannelAnswer identifies answer content.
	ChannelAnswer
	// ChannelTool identifies accumulated tool-call content.
	ChannelTool
)

// String returns the channel name, defaulting to none for unknown values.
func (c Channel) String() string {
	switch c {
	case ChannelReasoning:
		return "reasoning"
	case ChannelAnswer:
		return "answer"
	case ChannelTool:
		return "tool"
	default:
		return "none"
	}
}

// Result is the per-token classification; structural markers may have no content.
type Result struct {
	Channel Channel
	Content string
}

// StateMachine classifies streaming content for a single slot. Reset must be
// called before reuse and before Classify after end-of-generation.
type StateMachine interface {
	Classify(content string) (r Result, eog bool)
	Reset()
}

// TokenStateMachine optionally distinguishes complete native CONTROL or
// USER_DEFINED tokens from ordinary decoded text. Classify remains available
// for text-only callers and synthetic protocol initialization.
type TokenStateMachine interface {
	ClassifyToken(content string, special bool) (r Result, eog bool)
}

// TokenVocabularyStateMachine optionally resolves framing markers against the
// loaded vocabulary before use. The classifier is only needed during setup.
type TokenVocabularyStateMachine interface {
	SetTokenVocabulary(isSpecial func(string) bool)
}

// VocabEOGConsumer optionally consumes native framing also classified as EOG.
type VocabEOGConsumer interface {
	ConsumeVocabEOG(content string)
}

// ToolAwareStateMachine optionally accepts the request's declared tools.
type ToolAwareStateMachine interface {
	SetTools(tools []D)
}

// StateMachineFlusher drains retained output at successful end-of-generation.
// Call Flush until it returns a zero Result; content must not be returned twice.
type StateMachineFlusher interface {
	Flush() Result
}

// ToolCallDeltaStreamer exposes tool-call activity and request identities.
type ToolCallDeltaStreamer interface {
	ToolCallDeltas() []ResponseToolCallDelta
	StartedToolCalls() []ResponseToolCallDelta
}

// Parser implements a model lineage's streaming and final tool-call parsing.
// Parser instances are shared; NewStateMachine returns fresh slot-owned state.
type Parser interface {
	Name() string
	NewStateMachine() StateMachine
	ToolCall(ctx context.Context, log applog.Logger, buf string) []ResponseToolCall
}

// ToolCallSchemaParser optionally interprets arguments using declared schemas.
type ToolCallSchemaParser interface {
	ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []D) []ResponseToolCall
}

// ParamsAdjuster optionally coerces request parameters for a lineage's template.
type ParamsAdjuster interface {
	AdjustParams(p Params) Params
}

// Params carries only the request parameter needed by built-in parsers.
type Params struct {
	ReasoningEffort string
}

const (
	// ReasoningEffortNone disables reasoning where supported.
	ReasoningEffortNone = "none"
	// ReasoningEffortMinimal requests minimal reasoning.
	ReasoningEffortMinimal = "minimal"
	// ReasoningEffortMedium requests medium reasoning.
	ReasoningEffortMedium = "medium"
	// ReasoningEffortHigh requests high reasoning.
	ReasoningEffortHigh = "high"
)

// Fingerprint carries the metadata inspected during parser selection.
type Fingerprint struct {
	ChatTemplate string
	Architecture string
	ModelName    string
}
