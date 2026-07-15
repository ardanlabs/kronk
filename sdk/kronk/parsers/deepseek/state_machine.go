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
	inPending     bool
	inToolCall    bool
}

// Reset returns the stateMachine to its initial state for reuse.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pendingOpener.Reset()
	sm.inPending = false
	sm.inToolCall = false
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.inToolCall {
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	if sm.inPending {
		sm.pendingOpener.WriteString(content)
		candidate := sm.pendingOpener.String()

		switch {
		case strings.HasPrefix(candidate, toolCallsOpen):
			sm.inPending = false
			sm.pendingOpener.Reset()
			sm.inToolCall = true
			return model.Result{Channel: model.ChannelTool, Content: candidate}, false

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
		sm.inToolCall = true
		return model.Result{Channel: model.ChannelTool, Content: content}, false
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

func pendingOpenerSuffix(content string) int {
	limit := min(len(content), len(toolCallsOpen)-1)
	for size := limit; size > 0; size-- {
		if strings.HasSuffix(content, toolCallsOpen[:size]) {
			return len(content) - size
		}
	}

	return -1
}
