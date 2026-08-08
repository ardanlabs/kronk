package qwen

import (
	"encoding/json"
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
	toolCallBuf        strings.Builder
	inToolCall         bool
	wrappedTool        bool
	toolCallDone       bool // After a complete call; whitespace and another opener avoid EOG.
	directToolCallDone bool // The completed buffer used direct XML rather than a JSON envelope.

	// Lookahead buffer for split <function=… tokens.
	pendingTagBuf strings.Builder
	inPendingTag  bool

	// OpenAI-compatible activity deltas for tool-call starts.
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	deltaCallID    string
	deltaCallIndex int
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.wrappedTool = false
	sm.toolCallDone = false
	sm.directToolCallDone = false
	sm.pendingTagBuf.Reset()
	sm.inPendingTag = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.deltaCallID = ""
	sm.deltaCallIndex = 0
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
			wasAfterDirectToolCall := sm.directToolCallDone
			sm.inPendingTag = false
			sm.toolCallDone = false
			sm.directToolCallDone = false
			sm.pendingTagBuf.Reset()
			if wasAfterToolCall {
				if wasAfterDirectToolCall {
					return model.Result{Channel: model.ChannelTool, Content: accumulated}, false
				}
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
						sm.updateToolCallDeltas()
						return sm.completeToolCall(), false
					}
				}
			}

			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()

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
			if sm.directToolCallDone {
				// Preserve every unexpected continuation after direct XML for the
				// final parser. If it were discarded here, token boundaries could
				// turn malformed nested delimiter text into valid-looking calls.
				sm.toolCallDone = false
				sm.directToolCallDone = false
				return model.Result{Channel: model.ChannelTool, Content: content}, false
			}
			sm.toolCallDone = false
			sm.directToolCallDone = false
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
	sm.directToolCallDone = false
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
	sm.deltaCallID = ""
	sm.updateToolCallDeltas()
}

func (sm *stateMachine) completeToolCall() model.Result {
	content := strings.Trim(sm.toolCallBuf.String(), "\n")
	direct := strings.HasPrefix(strings.TrimSpace(content), "<function=")
	if content != "" {
		content += "\n"
	}

	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.wrappedTool = false
	sm.toolCallDone = true
	sm.directToolCallDone = direct
	if sm.deltaCallID != "" {
		sm.deltaCallIndex++
	}

	return model.Result{Channel: model.ChannelTool, Content: content}
}

// ToolCallDeltas drains OpenAI-compatible tool-call deltas produced by the
// most recent Classify call.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns the tool-call identities emitted during the current
// request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta {
	return sm.startedCalls
}

func (sm *stateMachine) updateToolCallDeltas() {
	if sm.deltaCallID != "" {
		return
	}

	name, ok := toolCallName(sm.toolCallBuf.String())
	if !ok {
		return
	}

	sm.deltaCallID = newToolCallID()
	delta := model.ResponseToolCallDelta{
		ID:    sm.deltaCallID,
		Index: sm.deltaCallIndex,
		Type:  "function",
		Function: model.ResponseToolCallDeltaFunction{
			Name: name,
		},
	}
	sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
	sm.startedCalls = append(sm.startedCalls, delta)
}

func toolCallName(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "<function=") {
		nameEnd := strings.IndexByte(content, '>')
		if nameEnd == -1 {
			return "", false
		}

		name := strings.TrimSpace(content[len("<function="):nameEnd])
		return name, name != ""
	}
	if content == "" || content[0] != '{' {
		return "", false
	}

	nameStart, ok := jsonFieldValueStart(content, "name")
	if !ok || nameStart >= len(content) || content[nameStart] != '"' {
		return "", false
	}

	nameEnd, ok := jsonStringEnd(content, nameStart)
	if !ok {
		return "", false
	}

	var name string
	if err := json.Unmarshal([]byte(content[nameStart:nameEnd]), &name); err != nil {
		return "", false
	}
	name = strings.TrimPrefix(name, ".")

	return name, name != ""
}

func jsonFieldValueStart(content string, field string) (int, bool) {
	depth := 0
	for i := 0; i < len(content); {
		switch content[i] {
		case '{':
			depth++
			i++
		case '}':
			depth--
			i++
		case '"':
			end, ok := jsonStringEnd(content, i)
			if !ok {
				return 0, false
			}
			if depth != 1 {
				i = end
				continue
			}

			var key string
			if err := json.Unmarshal([]byte(content[i:end]), &key); err != nil {
				return 0, false
			}
			i = end
			for i < len(content) && strings.ContainsRune(" \t\r\n", rune(content[i])) {
				i++
			}
			if i >= len(content) || content[i] != ':' {
				continue
			}
			i++
			for i < len(content) && strings.ContainsRune(" \t\r\n", rune(content[i])) {
				i++
			}
			if key == field {
				return i, true
			}
		default:
			i++
		}
	}

	return 0, false
}

func jsonStringEnd(content string, start int) (int, bool) {
	escape := false
	for i := start + 1; i < len(content); i++ {
		if escape {
			escape = false
			continue
		}
		if content[i] == '\\' {
			escape = true
			continue
		}
		if content[i] == '"' {
			return i + 1, true
		}
	}

	return 0, false
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
	sm.directToolCallDone = false

	return result
}
