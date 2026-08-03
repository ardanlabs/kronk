package mistral

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for Mistral and Devstral.
//
// Recognized markers:
//   - <think>…</think>           reasoning wrap (Magistral/Devstral)
//   - [THINK]…[/THINK]           reasoning wrap (Mistral Medium 3.5+)
//   - [TOOL_CALLS]               opens a streaming tool-call buffer
//
// Once [TOOL_CALLS] is emitted, every subsequent token (until EOG) is
// classified on the tool channel. The buffered payload is parsed at
// finish time via ToolCall.
type stateMachine struct {
	status         model.Channel
	inToolCall     bool
	toolCallBuf    strings.Builder
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	scanOffset     int
	awaitingArgEnd bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.inToolCall = false
	sm.toolCallBuf.Reset()
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.scanOffset = 0
	sm.awaitingArgEnd = false
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	// Once we are in tool mode, every token is tool-channel content. A
	// repeated [TOOL_CALLS] marker is silent (state already correct).
	if sm.inToolCall {
		sm.toolCallBuf.WriteString(content)
		sm.updateToolCallDeltas()
		return model.Result{}, false
	}

	switch content {
	case "<think>", "[THINK]":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>", "[/THINK]":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case "[TOOL_CALLS]":
		sm.status = model.ChannelTool
		sm.inToolCall = true
		sm.toolCallBuf.WriteString(content)
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}

func (sm *stateMachine) updateToolCallDeltas() {
	content := sm.toolCallBuf.String()
	for {
		if sm.awaitingArgEnd {
			argsEnd := findJSONObjectEnd(content[sm.scanOffset:])
			if argsEnd == -1 {
				return
			}
			sm.scanOffset += argsEnd
			sm.awaitingArgEnd = false
		}

		callOffset := strings.Index(content[sm.scanOffset:], "[TOOL_CALLS]")
		if callOffset == -1 {
			return
		}
		callStart := sm.scanOffset + callOffset
		nameStart := callStart + len("[TOOL_CALLS]")
		argsOffset := strings.Index(content[nameStart:], "[ARGS]")
		if argsOffset == -1 {
			return
		}

		name := strings.TrimSpace(content[nameStart : nameStart+argsOffset])
		if name != "" {
			delta := model.ResponseToolCallDelta{
				ID:       newToolCallID(),
				Index:    len(sm.startedCalls),
				Type:     "function",
				Function: model.ResponseToolCallDeltaFunction{Name: name},
			}
			sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
			sm.startedCalls = append(sm.startedCalls, delta)
		}
		sm.scanOffset = nameStart + argsOffset + len("[ARGS]")
		sm.awaitingArgEnd = true
	}
}

// Flush releases the complete buffered native tool-call stream.
func (sm *stateMachine) Flush() model.Result {
	if sm.toolCallBuf.Len() == 0 {
		return model.Result{}
	}

	content := sm.toolCallBuf.String()
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.scanOffset = 0
	sm.awaitingArgEnd = false
	return model.Result{Channel: model.ChannelTool, Content: content}
}

// ToolCallDeltas drains tool-call identity deltas produced by Classify.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns identities emitted during the current request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta {
	return sm.startedCalls
}
