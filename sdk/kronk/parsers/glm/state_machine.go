package glm

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for GLM.
//
// Recognized markers:
//   - <think>…</think>             reasoning wrap
//   - <tool_call>…</tool_call>     tool-call envelope (also <|tool_call>/<tool_call|>)
type stateMachine struct {
	status model.Channel

	toolCallBuf    strings.Builder
	inToolCall     bool
	toolCallDone   bool
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	detectedCalls  int
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.toolCallDone = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			return model.Result{}, false

		case "</tool_call>", "<tool_call|>":
			result := sm.flushToolCall()
			sm.toolCallDone = true
			return result, false

		default:
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			return model.Result{}, false
		}
	}

	if sm.toolCallDone {
		switch content {
		case "<tool_call>", "<|tool_call>":
			sm.toolCallDone = false
			sm.inToolCall = true
			sm.toolCallBuf.Reset()
			sm.detectedCalls = 0
			return model.Result{}, false
		default:
			sm.toolCallDone = false
			return model.Result{}, true
		}
	}

	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case "<tool_call>", "<|tool_call>":
		sm.status = model.ChannelTool
		sm.inToolCall = true
		sm.toolCallBuf.Reset()
		sm.detectedCalls = 0
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}

// ToolCallDeltas drains OpenAI-compatible tool-call activity deltas.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities emitted for this request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta { return sm.startedCalls }

func (sm *stateMachine) updateToolCallDeltas() {
	names := glmToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		delta := model.ResponseToolCallDelta{ID: newToolCallID(), Index: len(sm.startedCalls), Type: "function", Function: model.ResponseToolCallDeltaFunction{Name: name}}
		sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
		sm.startedCalls = append(sm.startedCalls, delta)
	}
	sm.detectedCalls = len(names)
}

func glmToolCallNames(content string) []string {
	var names []string
	for line := range strings.SplitSeq(content, "\n") {
		if before, _, ok := strings.Cut(line, "<arg_key>"); ok {
			if name := strings.TrimSpace(before); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// Flush drains tool-call content held while waiting for a closing marker.
func (sm *stateMachine) Flush() model.Result {
	if !sm.inToolCall {
		return model.Result{}
	}

	return sm.flushToolCall()
}

func (sm *stateMachine) flushToolCall() model.Result {
	content := strings.Trim(sm.toolCallBuf.String(), "\n")
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	if content == "" {
		return model.Result{}
	}

	return model.Result{Channel: model.ChannelTool, Content: content + "\n"}
}
