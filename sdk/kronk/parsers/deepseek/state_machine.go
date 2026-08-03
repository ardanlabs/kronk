package deepseek

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine classifies DeepSeek reasoning, answer, and DSML tool output.
// The pending opener buffer handles tokenizers that split the DSML marker and
// tag name across several decoded tokens.
type stateMachine struct {
	status model.Channel

	pendingOpener strings.Builder
	toolCallBuf   strings.Builder
	inPending     bool
	inToolCall    bool

	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	detectedCalls  int
}

// Reset returns the stateMachine to its initial state for reuse.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pendingOpener.Reset()
	sm.toolCallBuf.Reset()
	sm.inPending = false
	sm.inToolCall = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.inToolCall {
		return sm.bufferToolContent(content), false
	}

	if sm.inPending {
		sm.pendingOpener.WriteString(content)
		candidate := sm.pendingOpener.String()

		switch {
		case strings.HasPrefix(candidate, toolCallsOpen):
			sm.inPending = false
			sm.pendingOpener.Reset()
			sm.startToolCall(candidate)
			return sm.completeBufferedTool(), false

		case strings.HasPrefix(toolCallsOpen, candidate):
			return model.Result{}, false

		default:
			sm.inPending = false
			sm.pendingOpener.Reset()
			return sm.classifyContent(candidate)
		}
	}

	return sm.classifyContent(content)
}

// Flush drains an unresolved DSML opener prefix or an incomplete tools block.
func (sm *stateMachine) Flush() model.Result {
	if sm.inToolCall {
		content := sm.toolCallBuf.String()
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		return model.Result{Channel: model.ChannelTool, Content: content}
	}
	if !sm.inPending {
		return model.Result{}
	}

	result := model.Result{Channel: sm.status, Content: sm.pendingOpener.String()}
	sm.pendingOpener.Reset()
	sm.inPending = false

	return result
}

// ToolCallDeltas drains tool-call activity deltas discovered since the last
// call.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities discovered in this request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta {
	return sm.startedCalls
}

func (sm *stateMachine) classifyContent(content string) (model.Result, bool) {
	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false
	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false
	}

	if openerAt := strings.Index(content, toolCallsOpen); openerAt != -1 {
		sm.startToolCall(content[openerAt:])
		result := sm.completeBufferedTool()
		if openerAt > 0 && result.Content == "" {
			return model.Result{Channel: sm.status, Content: content[:openerAt]}, false
		}
		return result, false
	}

	if suffixAt := pendingOpenerSuffix(content); suffixAt != -1 {
		sm.inPending = true
		sm.pendingOpener.WriteString(content[suffixAt:])
		content = content[:suffixAt]
		if content == "" {
			return model.Result{}, false
		}
	}

	return model.Result{Channel: sm.status, Content: content}, false
}

func (sm *stateMachine) startToolCall(content string) {
	sm.inToolCall = true
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
	sm.detectedCalls = 0
	sm.updateToolCallDeltas()
}

func (sm *stateMachine) bufferToolContent(content string) model.Result {
	sm.toolCallBuf.WriteString(content)
	sm.updateToolCallDeltas()
	return sm.completeBufferedTool()
}

func (sm *stateMachine) completeBufferedTool() model.Result {
	content := sm.toolCallBuf.String()
	closeAt := strings.LastIndex(content, toolCallsClose)
	if closeAt == -1 {
		return model.Result{}
	}

	end := closeAt + len(toolCallsClose)
	complete := content[:end]
	remainder := content[end:]
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(remainder)
	sm.inToolCall = remainder != ""
	sm.detectedCalls = countInvokeOpeners(remainder)
	return model.Result{Channel: model.ChannelTool, Content: complete}
}

func countInvokeOpeners(content string) int {
	count := 0
	for offset := 0; ; count++ {
		invokeAt := strings.Index(content[offset:], invokeOpen)
		if invokeAt == -1 {
			return count
		}
		offset += invokeAt + len(invokeOpen)
	}
}

func (sm *stateMachine) updateToolCallDeltas() {
	content := sm.toolCallBuf.String()
	offset := 0
	seen := 0
	for {
		invokeAt := strings.Index(content[offset:], invokeOpen)
		if invokeAt == -1 {
			return
		}
		invokeAt += offset
		openerEnd := strings.IndexByte(content[invokeAt:], '>')
		if openerEnd == -1 {
			return
		}
		openerEnd += invokeAt
		if seen >= sm.detectedCalls {
			name, err := attribute(content[invokeAt:openerEnd+1], "name")
			if err == nil {
				sm.addToolCallDelta(name)
			}
		}
		seen++
		offset = openerEnd + 1
	}
}

func (sm *stateMachine) addToolCallDelta(name string) {
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
	sm.detectedCalls++
}

func pendingOpenerSuffix(content string) int {
	limit := min(len(content), len(toolCallsOpen)-1)
	for size := limit; size > 0; size-- {
		if strings.HasSuffix(content, toolCallsOpen[:size]) {
			return len(content) - size
		}
	}

	return -1
}
