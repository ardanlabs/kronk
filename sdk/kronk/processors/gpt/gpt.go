// Package gpt implements the Processor for GPT-OSS models, which use the
// OpenAI Harmony chat-template markers (<|channel|>, <|message|>, <|return|>,
// <|call|>, <|end|>, <|start|>, <|constrain|>).
//
// Reasoning, completion, and tool-call routing all hinge on the same
// <|channel|> marker (e.g. "analysis" → reasoning, "final" → completion,
// "commentary to=functions.X" → tool call). Because the marker is shared,
// the stateMachine here cannot be assembled from independent reasoning and
// tool-call plugins; it must be a single state machine, which is why
// GPT-OSS is its own processor rather than a parser-only variant of standard.
package gpt

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Processor.Name.
const name = "gpt-oss"

// Processor implements model.Processor for GPT-OSS.
type Processor struct{}

// New returns a Processor value if the fingerprint indicates GPT-OSS, otherwise
// returns false. The legacy detection ("isGPTModel" in modelInfo) is replaced
// by inspecting the chat template and architecture metadata.
func New(fp model.Fingerprint) (model.Processor, bool) {
	// GPT-OSS chat templates contain the unique Harmony markers.
	if containsHarmonyMarkers(fp.ChatTemplate) {
		return Processor{}, true
	}

	return Processor{}, false
}

// Name returns the processor identifier.
func (Processor) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Processor) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelNone}
}

// ParseToolCall extracts JSON tool calls from the GPT-OSS Harmony format and
// parses each one. The format is "[.NAME <|message|>]JSON" repeated; this
// function recovers the pairs and turns each into a JSON object that the
// shared JSON parser can decode.
func (Processor) ParseToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseGPTToolCall(ctx, log, buf)
}

// containsHarmonyMarkers reports whether a chat template carries the
// distinctive GPT-OSS Harmony tokens. Any one is sufficient because no
// other processor uses these exact tokens.
func containsHarmonyMarkers(template string) bool {
	for _, marker := range []string{
		"<|channel|>",
		"<|message|>",
		"<|return|>",
		"<|call|>",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}
