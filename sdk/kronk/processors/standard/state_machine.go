package standard

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for the standard
// catch-all processor.
//
// Recognized markers:
//   - <think>…</think>             reasoning wrap
//   - <tool_call>…</tool_call>     JSON tool-call envelope (also <|tool_call>/<tool_call|>)
type stateMachine struct {
	status model.Channel

	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.toolCallDone = false
}

// Process classifies a single decoded token's content.
//
// Behavior is undefined if Process is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Process(content string) (model.Result, bool) {
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			return model.Result{}, false

		case "</tool_call>", "<tool_call|>":
			toolContent := strings.Trim(sm.toolCallBuf.String(), "\n")
			if toolContent != "" {
				toolContent = fmt.Sprintf("%s\n", toolContent)
			}
			sm.toolCallBuf.Reset()
			sm.inToolCall = false
			sm.toolCallDone = true
			return model.Result{Channel: model.ChannelTool, Content: toolContent}, false

		default:
			sm.toolCallBuf.WriteString(content)
			return model.Result{}, false
		}
	}

	if sm.toolCallDone {
		switch content {
		case "<tool_call>", "<|tool_call>":
			sm.toolCallDone = false
			sm.inToolCall = true
			sm.toolCallBuf.Reset()
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
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}
