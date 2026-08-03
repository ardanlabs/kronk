package toolcall

import (
	"encoding/json"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for marked JSON tool
// calls.
//
// Recognized markers:
//   - <think>…</think>             reasoning wrap
//   - <tool_call>…</tool_call>     JSON tool-call envelope (also <|tool_call>/<tool_call|>)
type stateMachine struct {
	status model.Channel

	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool

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
			if strings.TrimSpace(content) == "" {
				return model.Result{}, false
			}
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
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta {
	return sm.startedCalls
}

func (sm *stateMachine) updateToolCallDeltas() {
	names := standardToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		delta := model.ResponseToolCallDelta{
			ID:    newToolCallID(),
			Index: len(sm.startedCalls),
			Type:  "function",
			Function: model.ResponseToolCallDeltaFunction{
				Name: name,
			},
		}
		sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
		sm.startedCalls = append(sm.startedCalls, delta)
	}
	sm.detectedCalls = len(names)
}

func standardToolCallNames(content string) []string {
	var names []string
	for offset := 0; offset < len(content); {
		start := strings.IndexByte(content[offset:], '{')
		if start == -1 {
			break
		}
		start += offset
		valueStart, ok := standardJSONFieldValueStart(content[start:], "name")
		if !ok || start+valueStart >= len(content) || content[start+valueStart] != '"' {
			break
		}
		valueStart += start
		valueEnd, ok := standardJSONStringEnd(content, valueStart)
		if !ok {
			break
		}
		var name string
		if json.Unmarshal([]byte(content[valueStart:valueEnd]), &name) == nil {
			name = strings.TrimPrefix(name, ".")
			if name != "" {
				names = append(names, name)
			}
		}
		if end := findJSONObjectEnd(content[start:]); end != -1 {
			offset = start + end
		} else {
			break
		}
	}
	return names
}

func standardJSONFieldValueStart(content string, field string) (int, bool) {
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
			end, ok := standardJSONStringEnd(content, i)
			if !ok {
				return 0, false
			}
			var key string
			if depth == 1 && json.Unmarshal([]byte(content[i:end]), &key) == nil {
				i = end
				for i < len(content) && strings.ContainsRune(" \t\r\n", rune(content[i])) {
					i++
				}
				if i < len(content) && content[i] == ':' {
					i++
					for i < len(content) && strings.ContainsRune(" \t\r\n", rune(content[i])) {
						i++
					}
					if key == field {
						return i, true
					}
				}
				continue
			}
			i = end
		default:
			i++
		}
	}
	return 0, false
}

func standardJSONStringEnd(content string, start int) (int, bool) {
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
		return model.Result{Channel: model.ChannelTool, Content: " "}
	}

	return model.Result{Channel: model.ChannelTool, Content: content + "\n"}
}
