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
	toolCallBuf  strings.Builder
	inToolCall   bool
	wrappedTool  bool
	toolCallDone bool // After a complete call; whitespace and another opener avoid EOG.

	// Retain only a possible delimiter prefix and up to two separator newlines.
	// A decoded piece can cross channels; queue its results in emission order.
	pending string
	results []model.Result

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
	sm.pending = ""
	sm.results = nil
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
	sm.consume(content)
	return sm.nextResult(), false
}

func (sm *stateMachine) consume(content string) {
	content = sm.pending + content
	sm.pending = ""
	for content != "" {
		if sm.inToolCall {
			// Repeated standalone opener inside an open block — skip.
			if content == "<tool_call>" || content == "<|tool_call>" {
				return
			}
			// Only newly appended bytes and the overlap can form a closer.
			start := max(0, sm.toolCallBuf.Len()-len("</tool_call>")+1)
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			accumulated := sm.toolCallBuf.String()
			markers := []string{"</function>"}
			if sm.wrappedTool {
				markers = []string{"</tool_call>", "<tool_call|>"}
			}
			end, closer := len(accumulated), ""
			for _, marker := range markers {
				if i := strings.Index(accumulated[start:], marker); i >= 0 && start+i < end {
					end, closer = start+i, marker
				}
			}
			if closer == "" {
				return
			}
			payloadEnd := end
			if !sm.wrappedTool {
				payloadEnd += len(closer)
			}
			sm.toolCallBuf.Reset()
			sm.toolCallBuf.WriteString(accumulated[:payloadEnd])
			sm.results = append(sm.results, sm.completeToolCall())
			content = accumulated[end+len(closer):]
			continue
		}

		markers := []string{"<tool_call>", "<|tool_call>", "<function="}
		if sm.status != model.ChannelTool {
			markers = append(markers, "<think>", "</think>")
		}
		if sm.toolCallDone {
			content = strings.TrimLeft(content, " \t\r\n")
		}
		matched := false
		for i := 0; i < len(content); i++ {
			for _, marker := range markers {
				if strings.HasPrefix(content[i:], marker) {
					before := content[:i]
					// Consume only the template's exact newline separators,
					// not arbitrary trailing whitespace or extra blank lines.
					if marker == "</think>" && sm.status == model.ChannelReasoning {
						before = strings.TrimSuffix(before, "\n")
					} else if sm.status == model.ChannelAnswer && marker != "<think>" && marker != "</think>" {
						before = strings.TrimSuffix(strings.TrimSuffix(before, "\n"), "\n")
					}
					sm.emit(before)
					content = content[i+len(marker):]
					switch marker {
					case "<think>":
						sm.status = model.ChannelReasoning
					case "</think>":
						sm.status = model.ChannelAnswer
					case "<function=":
						sm.startToolCall(false, marker)
					default:
						sm.startToolCall(true, "")
					}
					matched = true
					break
				}
				if strings.HasPrefix(marker, content[i:]) {
					sm.retain(content, i)
					return
				}
			}
			if matched {
				break
			}
			// Once tooling starts, unexpected continuation belongs to the
			// tool parser, not answer text. Never discard a failed prefix.
			if sm.status == model.ChannelTool {
				sm.emit(content)
				sm.toolCallDone = false
				return
			}
		}
		if !matched {
			sm.retain(content, len(content))
			return
		}
	}
}

func (sm *stateMachine) retain(content string, end int) {
	// A newline cannot be emitted until the following delimiter is known.
	for range 2 {
		if end > 0 && content[end-1] == '\n' {
			end--
		}
	}
	sm.emit(content[:end])
	sm.pending = content[end:]
}

func (sm *stateMachine) emit(content string) {
	if content != "" {
		sm.results = append(sm.results, model.Result{Channel: sm.status, Content: content})
	}
}

func (sm *stateMachine) nextResult() model.Result {
	if len(sm.results) == 0 {
		return model.Result{}
	}
	result := sm.results[0]
	sm.results[0] = model.Result{}
	sm.results = sm.results[1:]
	return result
}

func (sm *stateMachine) startToolCall(wrapped bool, content string) {
	sm.status = model.ChannelTool
	sm.inToolCall = true
	sm.wrappedTool = wrapped
	sm.toolCallDone = false
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
	sm.deltaCallID = ""
	sm.updateToolCallDeltas()
	sm.results = append(sm.results, model.Result{Channel: model.ChannelTool})
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

// Flush drains queued channel transitions, then any unresolved delimiter prefix
// or tool call. At final EOF a partial delimiter is ordinary channel content.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.results) != 0 {
		return sm.nextResult()
	}
	if sm.pending != "" {
		result := model.Result{Channel: sm.status, Content: sm.pending}
		sm.pending = ""
		return result
	}
	if sm.inToolCall {
		return sm.completeToolCall()
	}

	return model.Result{}
}
