package qwen

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for Qwen models. It
// recognizes:
//
//   - <think>…</think>       reasoning wrap
//   - <tool_call>…</tool_call> JSON envelope (also <|tool_call>/<tool_call|>)
//   - <function=name>…</function> direct XML format (Qwen-Coder)
//
// The split-tag lookahead handles tokenizers that fragment "<function=" into
// "<", "f", "function", "=", etc.
type stateMachine struct {
	status model.Channel

	// Tool-call accumulation across tokens.
	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool // After </tool_call> or </function>; only another opener avoids EOG.

	// Lookahead buffer for split <function=… tokens.
	pendingTagBuf strings.Builder
	inPendingTag  bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.toolCallDone = false
	sm.pendingTagBuf.Reset()
	sm.inPendingTag = false
}

// Process classifies a single decoded token's content.
//
// Behavior is undefined if Process is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Process(content string) (model.Result, bool) {
	// Lookahead for split <function= openers.
	if sm.inPendingTag {
		sm.pendingTagBuf.WriteString(content)
		accumulated := sm.pendingTagBuf.String()

		if strings.HasPrefix(accumulated, "<function=") {
			sm.inPendingTag = false
			sm.pendingTagBuf.Reset()
			sm.status = model.ChannelTool
			sm.inToolCall = true
			sm.toolCallBuf.Reset()
			sm.toolCallBuf.WriteString(accumulated)
			return model.Result{}, false
		}

		if !strings.HasPrefix("<function=", accumulated) {
			sm.inPendingTag = false
			sm.pendingTagBuf.Reset()
			return model.Result{Channel: sm.status, Content: accumulated}, false
		}

		return model.Result{}, false
	}

	// Inside a tool-call buffer: accumulate until close, or detect implicit
	// </function> close for the direct-XML format.
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			// Repeated opener inside an open block — skip.
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

			// Implicit close for direct <function=…></function> format.
			accumulated := sm.toolCallBuf.String()
			if strings.HasSuffix(strings.TrimSpace(accumulated), "</function>") {
				toolContent := strings.Trim(accumulated, "\n")
				if toolContent != "" {
					toolContent = fmt.Sprintf("%s\n", toolContent)
				}
				sm.toolCallBuf.Reset()
				sm.inToolCall = false
				sm.toolCallDone = true
				return model.Result{Channel: model.ChannelTool, Content: toolContent}, false
			}

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

	// Normal token processing.
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
		// Direct <function= opener (single token or split-tag prefix).
		if content == "<" || strings.HasPrefix(content, "<f") || strings.HasPrefix(content, "<function") {
			if strings.HasPrefix(content, "<function=") {
				sm.status = model.ChannelTool
				sm.inToolCall = true
				sm.toolCallBuf.Reset()
				sm.toolCallBuf.WriteString(content)
				return model.Result{}, false
			}
			if strings.HasPrefix("<function=", content) {
				sm.inPendingTag = true
				sm.pendingTagBuf.Reset()
				sm.pendingTagBuf.WriteString(content)
				return model.Result{}, false
			}
		}

		return model.Result{Channel: sm.status, Content: content}, false
	}
}
