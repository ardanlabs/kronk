package glm

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// stateMachine is a per-slot streaming state machine for GLM.
//
// Recognized markers:
//   - <think>…</think>             reasoning wrap
//   - <tool_call>…</tool_call>     tool-call envelope (also <|tool_call>/<tool_call|>)
type stateMachine struct {
	status parsers.Channel

	specialMarkers map[string]bool
	tokenMode      bool
	tokenPending   string
	tokenResults   []parsers.Result

	toolCallBuf      strings.Builder
	pendingEnvelope  strings.Builder
	inToolCall       bool
	pendingAfterTool bool
	toolCallDeltas   []parsers.ResponseToolCallDelta
	startedCalls     []parsers.ResponseToolCallDelta
	detectedCalls    int
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = parsers.ChannelAnswer
	sm.tokenMode = false
	sm.tokenPending = ""
	sm.tokenResults = nil
	sm.toolCallBuf.Reset()
	sm.pendingEnvelope.Reset()
	sm.inToolCall = false
	sm.pendingAfterTool = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// SetTokenVocabulary resolves both envelope and argument-tag framing. Laguna
// uses the same protocol with ordinary-text argument tags, unlike GLM.
func (sm *stateMachine) SetTokenVocabulary(isSpecial func(string) bool) {
	sm.specialMarkers = make(map[string]bool)
	for _, marker := range glmTokenMarkers {
		sm.specialMarkers[marker] = isSpecial(marker)
	}
}

var glmTokenMarkers = []string{
	"<think>", "</think>", "<tool_call>", "<|tool_call>", "</tool_call>", "<tool_call|>",
	"<arg_key>", "</arg_key>", "<arg_value>", "</arg_value>",
}

// ClassifyToken requires native identity for dedicated markers. Tool payloads
// retain literal '<' bytes in an escaped internal form until final decoding;
// otherwise the final argument-tag parser would mistake them for framing.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	sm.tokenMode = true
	if special && sm.specialMarkers[content] {
		sm.tokenLiteral(sm.tokenPending)
		sm.tokenPending = ""
		sm.tokenMarker(content)
		return sm.popTokenResult(), false
	}
	content = sm.tokenPending + content
	sm.tokenPending = ""
	start := 0
	for i := 0; i < len(content); {
		matched := false
		for _, marker := range glmTokenMarkers {
			if sm.specialMarkers[marker] {
				continue
			}
			if strings.HasPrefix(content[i:], marker) {
				sm.tokenLiteral(content[start:i])
				sm.tokenMarker(marker)
				i += len(marker)
				start = i
				matched = true
				break
			}
			if strings.HasPrefix(marker, content[i:]) {
				sm.tokenLiteral(content[start:i])
				sm.tokenPending = content[i:]
				return sm.popTokenResult(), false
			}
		}
		if !matched {
			i++
		}
	}
	sm.tokenLiteral(content[start:])
	return sm.popTokenResult(), false
}

func (sm *stateMachine) tokenMarker(marker string) {
	if sm.inToolCall || sm.pendingAfterTool || marker == "<tool_call>" || marker == "<|tool_call>" || marker == "<think>" || marker == "</think>" {
		r, _ := sm.Classify(marker)
		sm.pushTokenResult(r)
		return
	}
	sm.tokenLiteral(marker)
}

func (sm *stateMachine) tokenLiteral(content string) {
	if content == "" {
		return
	}
	if sm.inToolCall || sm.pendingAfterTool {
		// Escape the escape byte first, so arbitrary literal
		// NULs, entity-like strings and chunk boundaries round-trip exactly.
		content = strings.ReplaceAll(strings.ReplaceAll(content, "\x00", "\x000"), "<", "\x00l")
		r, _ := sm.Classify(content)
		sm.pushTokenResult(r)
		return
	}
	sm.pushTokenResult(parsers.Result{Channel: sm.status, Content: content})
}

func (sm *stateMachine) pushTokenResult(r parsers.Result) {
	if r.Content != "" {
		sm.tokenResults = append(sm.tokenResults, r)
	}
}

func (sm *stateMachine) popTokenResult() parsers.Result {
	if len(sm.tokenResults) == 0 {
		return parsers.Result{}
	}
	r := sm.tokenResults[0]
	sm.tokenResults = sm.tokenResults[1:]
	return r
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	if sm.pendingEnvelope.Len() > 0 {
		sm.pendingEnvelope.WriteString(content)
		candidate := sm.pendingEnvelope.String()
		if isGLMOpenerPrefix(candidate) {
			return parsers.Result{}, false
		}
		sm.pendingEnvelope.Reset()
		for _, opener := range glmOpeners {
			if strings.HasPrefix(candidate, opener) {
				sm.pendingAfterTool = false
				sm.startToolCall()
				return sm.consumeToolContent(candidate[len(opener):]), false
			}
		}
		if sm.pendingAfterTool {
			sm.pendingAfterTool = false
			return parsers.Result{Channel: parsers.ChannelTool, Content: "\n</tool_call>" + candidate}, false
		}
		return parsers.Result{Channel: sm.status, Content: candidate}, false
	}

	if sm.inToolCall {
		return sm.consumeToolContent(content), false
	}

	if sm.pendingAfterTool {
		content = strings.TrimLeft(content, " \t\r\n")
		if content == "" {
			return parsers.Result{}, false
		}
		for _, opener := range glmOpeners {
			if strings.HasPrefix(content, opener) {
				sm.pendingAfterTool = false
				sm.startToolCall()
				return sm.consumeToolContent(content[len(opener):]), false
			}
		}
		if isGLMOpenerPrefix(content) {
			sm.pendingEnvelope.WriteString(content)
			return parsers.Result{}, false
		}
		sm.pendingAfterTool = false
		return parsers.Result{Channel: parsers.ChannelTool, Content: "\n</tool_call>" + content}, false
	}

	switch content {
	case "<think>":
		sm.status = parsers.ChannelReasoning
		return parsers.Result{}, false

	case "</think>":
		sm.status = parsers.ChannelAnswer
		return parsers.Result{}, false

	default:
		for _, opener := range glmOpeners {
			if strings.HasPrefix(content, opener) {
				sm.status = parsers.ChannelTool
				sm.startToolCall()
				return sm.consumeToolContent(content[len(opener):]), false
			}
		}
		if isGLMOpenerPrefix(content) {
			sm.pendingEnvelope.WriteString(content)
			return parsers.Result{}, false
		}
		return parsers.Result{Channel: sm.status, Content: content}, false
	}
}

var glmOpeners = []string{"<tool_call>", "<|tool_call>"}
var glmClosers = []string{"</tool_call>", "<tool_call|>"}

func isGLMOpenerPrefix(content string) bool {
	for _, opener := range glmOpeners {
		if len(content) < len(opener) && strings.HasPrefix(opener, content) {
			return true
		}
	}
	return false
}

func (sm *stateMachine) startToolCall() {
	sm.inToolCall = true
	sm.toolCallBuf.Reset()
	sm.detectedCalls = 0
}

func (sm *stateMachine) consumeToolContent(content string) parsers.Result {
	sm.toolCallBuf.WriteString(content)
	sm.updateToolCallDeltas()
	buffered := sm.toolCallBuf.String()
	closeAt, closer := firstGLMCloser(buffered)
	if closeAt == -1 {
		return parsers.Result{}
	}

	body := strings.Trim(buffered[:closeAt], "\n")
	complete := body + "\n"
	if strings.TrimSpace(body) == "" {
		complete = "</tool_call>\n"
	}
	if sm.tokenMode {
		complete = tokenToolPrefix + complete
	}
	remainder := strings.TrimLeft(buffered[closeAt+len(closer):], " \t\r\n")
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.pendingAfterTool = true
	if remainder == "" {
		return parsers.Result{Channel: parsers.ChannelTool, Content: complete}
	}
	for _, opener := range glmOpeners {
		if strings.HasPrefix(remainder, opener) {
			sm.pendingAfterTool = false
			sm.startToolCall()
			next := sm.consumeToolContent(remainder[len(opener):])
			return parsers.Result{Channel: parsers.ChannelTool, Content: complete + next.Content}
		}
	}
	if isGLMOpenerPrefix(remainder) {
		sm.pendingEnvelope.WriteString(remainder)
		return parsers.Result{Channel: parsers.ChannelTool, Content: complete}
	}
	sm.pendingAfterTool = false
	return parsers.Result{Channel: parsers.ChannelTool, Content: complete + "</tool_call>" + remainder}
}

func firstGLMCloser(content string) (int, string) {
	first := -1
	var marker string
	for _, closer := range glmClosers {
		if at := strings.Index(content, closer); at != -1 && (first == -1 || at < first) {
			first = at
			marker = closer
		}
	}
	return first, marker
}

// ToolCallDeltas drains OpenAI-compatible tool-call activity deltas.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities emitted for this request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta { return sm.startedCalls }

func (sm *stateMachine) updateToolCallDeltas() {
	names := glmToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		if sm.tokenMode {
			name = decodeTokenText(name)
		}
		delta := parsers.ResponseToolCallDelta{ID: newToolCallID(), Index: len(sm.startedCalls), Type: "function", Function: parsers.ResponseToolCallDeltaFunction{Name: name}}
		sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
		sm.startedCalls = append(sm.startedCalls, delta)
	}
	sm.detectedCalls = len(names)
}

func glmToolCallNames(content string) []string {
	before, _, ok := strings.Cut(content, "<arg_key>")
	if !ok {
		return nil
	}

	name := strings.TrimSpace(before)
	if name == "" {
		return nil
	}

	return []string{name}
}

// Flush drains tool-call content held while waiting for a closing marker.
func (sm *stateMachine) Flush() parsers.Result {
	if r := sm.popTokenResult(); r != (parsers.Result{}) {
		return r
	}
	if sm.tokenPending != "" {
		content := sm.tokenPending
		sm.tokenPending = ""
		sm.tokenLiteral(content)
		if r := sm.popTokenResult(); r != (parsers.Result{}) {
			return r
		}
	}
	if sm.inToolCall {
		content := strings.Trim(sm.toolCallBuf.String(), "\n")
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		if sm.tokenMode {
			content = tokenToolPrefix + content
		}
		return parsers.Result{Channel: parsers.ChannelTool, Content: content + "\n</tool_call>"}
	}
	if sm.pendingEnvelope.Len() > 0 {
		content := sm.pendingEnvelope.String()
		sm.pendingEnvelope.Reset()
		channel := sm.status
		if sm.pendingAfterTool {
			channel = parsers.ChannelTool
		}
		sm.pendingAfterTool = false
		return parsers.Result{Channel: channel, Content: content}
	}
	return parsers.Result{}
}
