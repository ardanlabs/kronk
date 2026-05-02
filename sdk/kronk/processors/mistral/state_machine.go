package mistral

import (
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for Mistral and Devstral.
//
// Recognized markers:
//   - <think>…</think>           reasoning wrap
//   - [TOOL_CALLS]               opens a streaming tool-call buffer
//
// Once [TOOL_CALLS] is emitted, every subsequent token (until EOG) is
// classified on the tool channel. The buffered payload is parsed at
// finish time via ParseToolCall.
type stateMachine struct {
	status     model.Channel
	inToolCall bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.inToolCall = false
}

// Process classifies a single decoded token's content.
//
// Behavior is undefined if Process is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Process(content string) (model.Result, bool) {
	// Once we are in tool mode, every token is tool-channel content. A
	// repeated [TOOL_CALLS] marker is silent (state already correct).
	if sm.inToolCall {
		if content == "[TOOL_CALLS]" {
			return model.Result{}, false
		}
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case "[TOOL_CALLS]":
		sm.status = model.ChannelTool
		sm.inToolCall = true
		return model.Result{Channel: model.ChannelTool, Content: "[TOOL_CALLS]"}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}
