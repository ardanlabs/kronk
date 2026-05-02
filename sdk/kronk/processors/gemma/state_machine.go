package gemma

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for Gemma.
//
// Recognized markers:
//
//   - <|channel> NAME … <channel|>  reasoning wrap (NAME token is swallowed)
//   - <tool_call> … </tool_call>    tool-call envelope (also <|tool_call>/<tool_call|>)
//   - <|tool_response>, <tool_response|> structural skips
type stateMachine struct {
	status model.Channel

	// Tool-call accumulation across tokens.
	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool

	// Swallow the channel name token (e.g. "thought") that follows <|channel>.
	awaitingChannel bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.toolCallDone = false
	sm.awaitingChannel = false
}

// Process classifies a single decoded token's content.
//
// Behavior is undefined if Process is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Process(content string) (model.Result, bool) {
	// Inside a tool-call envelope: accumulate until close.
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

	// After a tool call closes, only another opener avoids EOG.
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

	// Swallow the channel name token after <|channel>, then stream content
	// as reasoning until <channel|>.
	if sm.awaitingChannel {
		sm.awaitingChannel = false
		sm.status = model.ChannelReasoning
		return model.Result{}, false
	}

	switch content {
	case "<|channel>":
		sm.awaitingChannel = true
		return model.Result{}, false

	case "<channel|>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case "<tool_call>", "<|tool_call>":
		sm.status = model.ChannelTool
		sm.inToolCall = true
		sm.toolCallBuf.Reset()
		return model.Result{}, false

	case "<tool_call|>", "<|tool_response>", "<tool_response|>":
		// Structural markers outside tool-call accumulation; skip silently.
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}
