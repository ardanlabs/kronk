package qwen

import (
	"encoding/json"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
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
	status parsers.Channel

	// Vocabulary policy survives Reset; request state never does.
	specialMarkers map[string]bool

	// Tool-call accumulation across tokens.
	toolCallBuf  strings.Builder
	inToolCall   bool
	wrappedTool  bool
	toolCallDone bool // After a complete call; whitespace and another opener avoid EOG.

	// Retain only a possible delimiter prefix and up to two separator newlines.
	// A decoded piece can cross channels; queue its results in emission order.
	pending string
	results []parsers.Result

	// OpenAI-compatible activity deltas for tool-call starts.
	toolCallDeltas []parsers.ResponseToolCallDelta
	startedCalls   []parsers.ResponseToolCallDelta
	deltaCallID    string
	deltaCallIndex int
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = parsers.ChannelAnswer
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
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	sm.consume(content, false, false)
	return sm.nextResult(), false
}

// SetTokenVocabulary resolves framing once per slot. Qwen XML tags may be
// ordinary text, whereas LFM3 also gives those tags dedicated tokens.
func (sm *stateMachine) SetTokenVocabulary(isSpecial func(string) bool) {
	sm.specialMarkers = make(map[string]bool)
	for _, marker := range []string{"<think>", "</think>", "<tool_call>", "</tool_call>", "<|tool_call>", "<tool_call|>", "<function=", "</function>"} {
		sm.specialMarkers[marker] = isSpecial(marker)
	}
}

// ClassifyToken recognizes dedicated framing only as a complete special token.
// Delimiters absent from the special vocabulary retain text-based matching.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	sm.consume(content, true, special)
	return sm.nextResult(), false
}

func (sm *stateMachine) consume(content string, tokenAware, special bool) {
	tokenStart, tokenLen := len(sm.pending), len(content)
	markerAt := func(marker string, offset int) bool {
		return !tokenAware || !sm.specialMarkers[marker] ||
			(special && offset == tokenStart && len(marker) == tokenLen)
	}
	content = sm.pending + content
	sm.pending = ""
	for content != "" {
		if sm.inToolCall {
			// Repeated standalone opener inside an open block — skip.
			if (content == "<tool_call>" || content == "<|tool_call>") && markerAt(content, 0) {
				return
			}
			// Only newly appended bytes and the overlap can form a closer.
			previousLen := sm.toolCallBuf.Len()
			start := max(0, previousLen-len("</tool_call>")+1)
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			accumulated := sm.toolCallBuf.String()
			markers := []string{"</function>"}
			if sm.wrappedTool {
				markers = []string{"</tool_call>", "<tool_call|>"}
			}
			end, closer := len(accumulated), ""
			for _, marker := range markers {
				for offset := start; offset < end; {
					i := strings.Index(accumulated[offset:], marker)
					if i < 0 {
						break
					}
					offset += i
					if offset < end && markerAt(marker, offset-previousLen) {
						end, closer = offset, marker
						break
					}
					offset++
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
			tokenStart -= end + len(closer) - previousLen
			content = accumulated[end+len(closer):]
			continue
		}

		markers := []string{"<tool_call>", "<|tool_call>", "<function="}
		if sm.status != parsers.ChannelTool {
			markers = append(markers, "<think>", "</think>")
		}
		if sm.toolCallDone {
			trimmed := strings.TrimLeft(content, " \t\r\n")
			tokenStart -= len(content) - len(trimmed)
			content = trimmed
		}
		matched := false
		for i := 0; i < len(content); i++ {
			for _, marker := range markers {
				if !markerAt(marker, i) {
					continue
				}
				if strings.HasPrefix(content[i:], marker) {
					before := content[:i]
					// Consume only the template's exact newline separators,
					// not arbitrary trailing whitespace or extra blank lines.
					if marker == "</think>" && sm.status == parsers.ChannelReasoning {
						before = strings.TrimSuffix(before, "\n")
					} else if sm.status == parsers.ChannelAnswer && marker != "<think>" && marker != "</think>" {
						before = strings.TrimSuffix(strings.TrimSuffix(before, "\n"), "\n")
					}
					sm.emit(before)
					tokenStart -= i + len(marker)
					content = content[i+len(marker):]
					switch marker {
					case "<think>":
						sm.status = parsers.ChannelReasoning
					case "</think>":
						sm.status = parsers.ChannelAnswer
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
			if sm.status == parsers.ChannelTool {
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
		sm.results = append(sm.results, parsers.Result{Channel: sm.status, Content: content})
	}
}

func (sm *stateMachine) nextResult() parsers.Result {
	if len(sm.results) == 0 {
		return parsers.Result{}
	}
	result := sm.results[0]
	sm.results[0] = parsers.Result{}
	sm.results = sm.results[1:]
	return result
}

func (sm *stateMachine) startToolCall(wrapped bool, content string) {
	sm.status = parsers.ChannelTool
	sm.inToolCall = true
	sm.wrappedTool = wrapped
	sm.toolCallDone = false
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
	sm.deltaCallID = ""
	sm.updateToolCallDeltas()
	sm.results = append(sm.results, parsers.Result{Channel: parsers.ChannelTool})
}

func (sm *stateMachine) completeToolCall() parsers.Result {
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

	return parsers.Result{Channel: parsers.ChannelTool, Content: content}
}

// ToolCallDeltas drains OpenAI-compatible tool-call deltas produced by the
// most recent Classify call.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns the tool-call identities emitted during the current
// request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta {
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
	delta := parsers.ResponseToolCallDelta{
		ID:    sm.deltaCallID,
		Index: sm.deltaCallIndex,
		Type:  "function",
		Function: parsers.ResponseToolCallDeltaFunction{
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
func (sm *stateMachine) Flush() parsers.Result {
	if len(sm.results) != 0 {
		return sm.nextResult()
	}
	if sm.pending != "" {
		result := parsers.Result{Channel: sm.status, Content: sm.pending}
		sm.pending = ""
		return result
	}
	if sm.inToolCall {
		return sm.completeToolCall()
	}

	return parsers.Result{}
}
