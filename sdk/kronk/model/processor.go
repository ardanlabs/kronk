package model

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// =============================================================================
// Processor Plugin Contract
//
// A processor plugin teaches the model server how a particular model lineage
// (Qwen, GPT-OSS, Mistral, Gemma, GLM, …) emits two kinds of structured
// content during streaming generation:
//
//  1. Reasoning vs answer vs tool-call content (per-token classification),
//     surfaced through the StateMachine interface.
//
//  2. Final tool-call payloads (parsed once at end-of-stream from the
//     accumulated tool-call buffer), surfaced through ParseToolCall.
//
// A single Processor implementation is selected at Model.Load() based on
// the chat template, architecture, and model name. The selected plugin
// lives on *Model for the lifetime of the model and is safe for concurrent
// use across slots — except for NewStateMachine, which returns a fresh,
// slot-owned state machine on every call.
//
// Plugins live under sdk/kronk/processors/<name>/ and are wired into the
// model package via RegisterProcessor at server bootstrap (no init()
// registration — every processor is explicitly enumerated at startup).
// =============================================================================

// Channel labels the semantic class of an emitted token, mapping 1:1 to the
// OpenAI chat/completions delta fields produced by the SSE writer.
type Channel uint8

const (
	// ChannelNone indicates a structural marker the caller should not
	// surface in either content or tool-call output. The token is still
	// counted toward the slot's output-token total.
	ChannelNone Channel = iota

	// ChannelReasoning content is accumulated into the response's
	// reasoning_content field and streamed as reasoning deltas.
	ChannelReasoning

	// ChannelAnswer content is accumulated into the response's content
	// field and streamed as content deltas.
	ChannelAnswer

	// ChannelTool content is accumulated into the slot's tool-call buffer
	// and parsed once via ParseToolCall when generation finishes.
	ChannelTool
)

// Result is the per-token outcome returned by StateMachine.Process.
//
// Content may be empty when the token is a structural marker that has been
// fully consumed by the state machine (e.g. <think>, <tool_call>). When
// Content is non-empty, it is routed to the appropriate accumulator based on
// Channel.
type Result struct {
	Channel Channel
	Content string
}

// StateMachine is the per-request, per-slot streaming state machine. One
// instance is created per slot via Processor.NewStateMachine and reused
// across requests on that slot via Reset.
//
// Behavior is undefined if Process is called after a previous call returned
// eog=true. Callers must invoke Reset before reusing the state machine.
type StateMachine interface {
	// Process classifies a single decoded token's content and returns the
	// Result plus whether the model has signaled end-of-generation.
	Process(content string) (r Result, eog bool)

	// Reset returns the state machine to its initial state for reuse.
	Reset()
}

// Processor is the plugin interface implemented by each model lineage.
// Implementations live in sdk/kronk/processors/<name>/ and are registered
// at startup via RegisterProcessor.
type Processor interface {
	// Name returns the processor identifier (e.g. "standard", "gpt-oss").
	// Used for logging and as the override key in model configs.
	Name() string

	// NewStateMachine returns a fresh per-slot state machine. Callers must
	// not share StateMachine instances across slots.
	NewStateMachine() StateMachine

	// ParseToolCall parses the accumulated tool-call buffer into structured
	// tool calls. Called once when generation finishes, never on the hot
	// per-token path. The logger is used for repair/parse failures; tests
	// may pass a no-op logger.
	ParseToolCall(ctx context.Context, log applog.Logger, buf string) []ResponseToolCall
}

// Fingerprint carries the model metadata that processor selection logic
// inspects at Model.Load time.
type Fingerprint struct {
	ChatTemplate string // raw jinja chat template
	Architecture string // gguf "general.architecture" (e.g. "llama", "qwen2")
	ModelName    string // gguf "general.name" (e.g. "Qwen3-Coder-30B-A3B")
}
