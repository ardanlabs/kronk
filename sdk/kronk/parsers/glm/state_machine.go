package glm

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for GLM.
//
// Recognized markers:
//   - <think>…</think>             reasoning wrap
//   - <tool_call>…</tool_call>     tool-call envelope (also <|tool_call>/<tool_call|>)
type stateMachine struct {
	status model.Channel

	toolCallBuf      strings.Builder
	pendingEnvelope  strings.Builder
	inToolCall       bool
	pendingAfterTool bool
	toolCallDeltas   []model.ResponseToolCallDelta
	startedCalls     []model.ResponseToolCallDelta
	detectedCalls    int
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.pendingEnvelope.Reset()
	sm.inToolCall = false
	sm.pendingAfterTool = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.pendingEnvelope.Len() > 0 {
		sm.pendingEnvelope.WriteString(content)
		candidate := sm.pendingEnvelope.String()
		if isGLMOpenerPrefix(candidate) {
			return model.Result{}, false
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
			return model.Result{Channel: model.ChannelTool, Content: candidate}, false
		}
		return model.Result{Channel: sm.status, Content: candidate}, false
	}

	if sm.inToolCall {
		return sm.consumeToolContent(content), false
	}

	if sm.pendingAfterTool {
		content = strings.TrimLeft(content, " \t\r\n")
		if content == "" {
			return model.Result{}, false
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
			return model.Result{}, false
		}
		sm.pendingAfterTool = false
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	default:
		for _, opener := range glmOpeners {
			if strings.HasPrefix(content, opener) {
				sm.status = model.ChannelTool
				sm.startToolCall()
				return sm.consumeToolContent(content[len(opener):]), false
			}
		}
		if isGLMOpenerPrefix(content) {
			sm.pendingEnvelope.WriteString(content)
			return model.Result{}, false
		}
		return model.Result{Channel: sm.status, Content: content}, false
	}
}

var glmOpeners = []string{"<tool_call>", "<|tool_call>"}
var glmClosers = []string{"</tool_call>", "<tool_call|>"}

const glmToolCallEvidence = "\n</tool_call>"
const glmMissingToolCallClose = "\n<missing-tool-call-close>"

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

func (sm *stateMachine) consumeToolContent(content string) model.Result {
	sm.toolCallBuf.WriteString(content)
	sm.updateToolCallDeltas()
	buffered := sm.toolCallBuf.String()
	closeAt, closer := firstGLMCloser(buffered)
	if closeAt == -1 {
		return model.Result{}
	}

	body := strings.Trim(buffered[:closeAt], "\n")
	complete := body + glmToolCallEvidence
	remainder := strings.TrimLeft(buffered[closeAt+len(closer):], " \t\r\n")
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.pendingAfterTool = true
	if remainder == "" {
		return model.Result{Channel: model.ChannelTool, Content: complete}
	}
	for _, opener := range glmOpeners {
		if strings.HasPrefix(remainder, opener) {
			sm.pendingAfterTool = false
			sm.startToolCall()
			next := sm.consumeToolContent(remainder[len(opener):])
			return model.Result{Channel: model.ChannelTool, Content: complete + next.Content}
		}
	}
	if isGLMOpenerPrefix(remainder) {
		sm.pendingEnvelope.WriteString(remainder)
		return model.Result{Channel: model.ChannelTool, Content: complete}
	}
	sm.pendingAfterTool = false
	return model.Result{Channel: model.ChannelTool, Content: complete + remainder}
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
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities emitted for this request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta { return sm.startedCalls }

func (sm *stateMachine) updateToolCallDeltas() {
	names := glmToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		delta := model.ResponseToolCallDelta{ID: newToolCallID(), Index: len(sm.startedCalls), Type: "function", Function: model.ResponseToolCallDeltaFunction{Name: name}}
		sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
		sm.startedCalls = append(sm.startedCalls, delta)
	}
	sm.detectedCalls = len(names)
}

func glmToolCallNames(content string) []string {
	var names []string
	for line := range strings.SplitSeq(content, "\n") {
		if before, _, ok := strings.Cut(line, "<arg_key>"); ok {
			if name := strings.TrimSpace(before); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// Flush drains tool-call content held while waiting for a closing marker.
func (sm *stateMachine) Flush() model.Result {
	if sm.inToolCall {
		content := strings.Trim(sm.toolCallBuf.String(), "\n")
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		return model.Result{Channel: model.ChannelTool, Content: content + glmMissingToolCallClose}
	}
	if sm.pendingEnvelope.Len() > 0 {
		content := sm.pendingEnvelope.String()
		sm.pendingEnvelope.Reset()
		channel := sm.status
		if sm.pendingAfterTool {
			channel = model.ChannelTool
		}
		sm.pendingAfterTool = false
		return model.Result{Channel: channel, Content: content}
	}
	return model.Result{}
}
