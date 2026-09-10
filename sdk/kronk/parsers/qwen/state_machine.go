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
//   - ```json fenced envelope  a tool call emitted as fenced text
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

	// Reply-leading markdown code fence. Some Qwen models (notably
	// Qwen2.5-Coder at larger prompt sizes) emit a valid tool-call envelope
	// inside a ``` fence as visible text instead of the marked format. When
	// tools are declared, the fence is held until its body resolves as one
	// of those calls or as ordinary content.
	fenceBuf    strings.Builder
	fenceActive bool
	fenceBody   bool
	emitted     bool

	// Declared tool names from the request, used to tell a fenced tool-call
	// envelope apart from ordinary fenced JSON.
	toolNames map[string]struct{}

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
	sm.fenceBuf.Reset()
	sm.fenceActive = false
	sm.fenceBody = false
	sm.emitted = false
	sm.toolNames = nil
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
	// A reply-leading markdown code fence being held for classification.
	if sm.fenceActive {
		return sm.classifyFenced(content)
	}

	// The fence hold engages only when tools are declared and the reply's
	// very first token opens a fence — anything else (prose, whitespace, a
	// marked call) must stream through the normal paths untouched.
	if len(sm.toolNames) > 0 && !sm.emitted && !sm.inPendingTag && !sm.inToolCall && !sm.toolCallDone && sm.status == model.ChannelAnswer &&
		content != "" && (strings.HasPrefix(content, fenceOpen) || strings.HasPrefix(fenceOpen, content)) {
		sm.fenceActive = true
		return sm.classifyFenced(content)
	}
	sm.emitted = true

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

const fenceOpen = "```"

// classifyFenced resolves reply-leading fenced content. The opener line is
// held until its newline arrives; the body is held until a closing fence, an
// early bail-out, or end of generation (Flush). A body that is a JSON
// tool-call envelope is delivered through the same completion path as a
// marked call; anything else is released verbatim as answer content, fences
// included.
func (sm *stateMachine) classifyFenced(content string) (model.Result, bool) {
	sm.fenceBuf.WriteString(content)
	candidate := sm.fenceBuf.String()

	if !sm.fenceBody {
		trimmed := strings.TrimLeft(candidate, " \t\r\n")
		if trimmed != "" && !strings.HasPrefix(trimmed, fenceOpen) && !strings.HasPrefix(fenceOpen, trimmed) {
			return sm.fenceBail(candidate)
		}
		line, _, found := strings.Cut(trimmed, "\n")
		if !found {
			return model.Result{}, false
		}
		if tag := strings.TrimPrefix(line, fenceOpen); tag != "" && !isLanguageTag(tag) {
			return sm.fenceBail(candidate)
		}
		sm.fenceBody = true
	}

	body := sm.fenceBodyText(candidate)
	inner, _, found := strings.Cut(body, "\n"+fenceOpen)
	if !found {
		// A body that cannot be an envelope is released immediately so
		// ordinary fenced content still streams rather than buffering to
		// end of generation.
		if b := strings.TrimLeft(body, " \t\r\n"); b != "" && !strings.HasPrefix(b, "{") {
			return sm.fenceBail(candidate)
		}
		return model.Result{}, false
	}
	trimmedInner := strings.TrimSpace(inner)
	if name, ok := envelopeName(trimmedInner); !ok || !sm.declaresTool(name) {
		return sm.fenceBail(candidate)
	}

	sm.fenceActive = false
	sm.fenceBody = false
	sm.fenceBuf.Reset()
	sm.startToolCall(true, trimmedInner)
	return sm.completeToolCall(), false
}

// fenceBodyText returns the fenced body following the opener line.
func (sm *stateMachine) fenceBodyText(candidate string) string {
	open := strings.Index(candidate, fenceOpen)
	if nl := strings.Index(candidate[open:], "\n"); nl >= 0 {
		return candidate[open+nl+1:]
	}
	return ""
}

// fenceBail releases a fence candidate as ordinary answer content, fences
// included, when it did not resolve to a tool-call envelope.
func (sm *stateMachine) fenceBail(candidate string) (model.Result, bool) {
	sm.fenceActive = false
	sm.fenceBody = false
	sm.fenceBuf.Reset()
	sm.emitted = true
	return model.Result{Channel: model.ChannelAnswer, Content: candidate}, false
}

// envelopeName parses content as a JSON tool-call envelope and returns its
// declared function name.
func envelopeName(content string) (string, bool) {
	if !strings.HasPrefix(content, "{") {
		return "", false
	}
	var envelope struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(content), &envelope) != nil || envelope.Name == "" {
		return "", false
	}
	return envelope.Name, true
}

// declaresTool reports whether name matches a tool declared in the request.
func (sm *stateMachine) declaresTool(name string) bool {
	_, declared := sm.toolNames[name]
	return declared
}

// SetTools supplies the request's declared tools, telling a fenced JSON
// envelope apart from ordinary fenced JSON by its function name.
func (sm *stateMachine) SetTools(tools []model.D) {
	sm.toolNames = make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if name := declaredToolName(tool); name != "" {
			sm.toolNames[name] = struct{}{}
		}
	}
}

// declaredToolName returns the function name of an OpenAI-style tool
// declaration.
func declaredToolName(tool model.D) string {
	if tool["type"] != "function" {
		return ""
	}
	function, ok := tool["function"].(model.D)
	if !ok {
		return ""
	}
	name, _ := function["name"].(string)
	return name
}

// isLanguageTag reports whether s is a bare markdown info string.
func isLanguageTag(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
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
	if sm.fenceActive {
		candidate := sm.fenceBuf.String()
		sm.fenceActive = false
		sm.fenceBody = false
		sm.fenceBuf.Reset()

		if body, ok := fenceBodyOf(candidate); ok {
			if name, ok := envelopeName(strings.TrimSpace(body)); ok && sm.declaresTool(name) {
				sm.startToolCall(true, strings.TrimSpace(body))
				return sm.completeToolCall()
			}
		}
		return model.Result{Channel: model.ChannelAnswer, Content: candidate}
	}

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

// fenceBodyOf splits a reply-leading fenced block, returning the body between
// the opener line and a closing fence. A missing closing fence is tolerated
// so truncated generation still classifies. It reports false when the content
// does not begin with a valid fence opener.
func fenceBodyOf(content string) (string, bool) {
	s := strings.TrimLeft(content, " \t\r\n")
	rest, ok := strings.CutPrefix(s, fenceOpen)
	if !ok {
		return "", false
	}
	tag, body, found := strings.Cut(rest, "\n")
	if !found {
		return "", false
	}
	if tag = strings.TrimSpace(tag); tag != "" && !isLanguageTag(tag) {
		return "", false
	}
	if inner, _, found := strings.Cut(body, "\n"+fenceOpen); found {
		return inner, true
	}
	return body, true
}
