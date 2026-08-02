package qwen

import (
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
	wrappedTool  bool
	toolCallDone bool // After a complete call; whitespace and another opener avoid EOG.

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
	sm.wrappedTool = false
	sm.toolCallDone = false
	sm.pendingTagBuf.Reset()
	sm.inPendingTag = false
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	// Lookahead for split <function= openers.
	if sm.inPendingTag {
		sm.pendingTagBuf.WriteString(content)
		accumulated := sm.pendingTagBuf.String()

		if strings.HasPrefix(accumulated, "<function=") {
			sm.inPendingTag = false
			sm.pendingTagBuf.Reset()
			sm.startToolCall(false, accumulated)
			return model.Result{Channel: model.ChannelTool}, false
		}

		if !strings.HasPrefix("<function=", accumulated) {
			wasAfterToolCall := sm.toolCallDone
			sm.inPendingTag = false
			sm.toolCallDone = false
			sm.pendingTagBuf.Reset()
			if wasAfterToolCall {
				return model.Result{}, true
			}
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
			return sm.completeToolCall(), false

		default:
			if sm.wrappedTool {
				for _, marker := range []string{"</tool_call>", "<tool_call|>"} {
					trimmed := strings.TrimRight(content, " \t\r\n")
					if before, ok := strings.CutSuffix(trimmed, marker); ok {
						sm.toolCallBuf.WriteString(before)
						return sm.completeToolCall(), false
					}
				}
			}

			sm.toolCallBuf.WriteString(content)

			// Unwrapped direct calls close at </function>. Wrapped calls close
			// only at </tool_call>, after the complete inner XML is buffered.
			accumulated := sm.toolCallBuf.String()
			if !sm.wrappedTool && strings.HasSuffix(strings.TrimSpace(accumulated), "</function>") {
				return sm.completeToolCall(), false
			}

			return model.Result{}, false
		}
	}

	// After a tool call closes, allow whitespace and another opener so
	// consecutive calls are collected into one response.
	if sm.toolCallDone {
		content = strings.TrimLeft(content, " \t\r\n")
		switch content {
		case "<tool_call>", "<|tool_call>":
			sm.startToolCall(true, "")
			return model.Result{Channel: model.ChannelTool}, false
		default:
			if content == "" {
				return model.Result{}, false
			}
			if strings.HasPrefix(content, "<function=") {
				sm.startToolCall(false, content)
				return model.Result{Channel: model.ChannelTool}, false
			}
			if content == "<" || strings.HasPrefix(content, "<f") || strings.HasPrefix(content, "<function") {
				if strings.HasPrefix("<function=", content) {
					sm.inPendingTag = true
					sm.pendingTagBuf.Reset()
					sm.pendingTagBuf.WriteString(content)
					return model.Result{}, false
				}
			}
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
		sm.startToolCall(true, "")
		return model.Result{Channel: model.ChannelTool}, false

	default:
		// Direct <function= opener (single token or split-tag prefix).
		if content == "<" || strings.HasPrefix(content, "<f") || strings.HasPrefix(content, "<function") {
			if strings.HasPrefix(content, "<function=") {
				sm.startToolCall(false, content)
				return model.Result{Channel: model.ChannelTool}, false
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

func (sm *stateMachine) startToolCall(wrapped bool, content string) {
	sm.status = model.ChannelTool
	sm.inToolCall = true
	sm.wrappedTool = wrapped
	sm.toolCallDone = false
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
}

func (sm *stateMachine) completeToolCall() model.Result {
	content := strings.Trim(sm.toolCallBuf.String(), "\n")
	if content != "" {
		content += "\n"
	}

	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.wrappedTool = false
	sm.toolCallDone = true

	return model.Result{Channel: model.ChannelTool, Content: content}
}

// Flush drains a buffered tool call or unresolved direct-function prefix when
// generation ends before the state machine sees its closing delimiter.
func (sm *stateMachine) Flush() model.Result {
	if sm.inToolCall {
		return sm.completeToolCall()
	}

	if !sm.inPendingTag {
		return model.Result{}
	}

	result := model.Result{Channel: sm.status, Content: sm.pendingTagBuf.String()}
	sm.pendingTagBuf.Reset()
	sm.inPendingTag = false
	sm.toolCallDone = false

	return result
}
