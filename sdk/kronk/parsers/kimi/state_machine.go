package kimi

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine classifies Kimi reasoning, response, and tool output. Kimi
// control tags may be split across decoded tokens, so a possible tag is held
// until its terminating <|sep|> arrives.
type stateMachine struct {
	status model.Channel

	pending strings.Builder
	inTag   bool
	inTools bool
}

// Reset returns the stateMachine to its initial state for reuse.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pending.Reset()
	sm.inTag = false
	sm.inTools = false
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.inTools {
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	if sm.inTag {
		sm.pending.WriteString(content)
		candidate := sm.pending.String()
		if !possibleControlTag(candidate) {
			sm.pending.Reset()
			sm.inTag = false
			return sm.Classify(candidate)
		}
		if !strings.Contains(candidate, sepToken) {
			return model.Result{}, false
		}

		sm.pending.Reset()
		sm.inTag = false
		return sm.classifyTag(candidate)
	}

	start := firstControlTag(content)
	if start == -1 {
		if content == "<|end_of_msg|>" {
			return model.Result{}, true
		}
		if suffixAt := pendingControlSuffix(content); suffixAt != -1 {
			sm.inTag = true
			sm.pending.WriteString(content[suffixAt:])
			content = content[:suffixAt]
			if content == "" {
				return model.Result{}, false
			}
		}
		return model.Result{Channel: sm.status, Content: content}, false
	}

	if start > 0 {
		sm.inTag = true
		sm.pending.WriteString(content[start:])
		return model.Result{Channel: sm.status, Content: content[:start]}, false
	}

	if !strings.Contains(content, sepToken) {
		sm.inTag = true
		sm.pending.WriteString(content)
		return model.Result{}, false
	}

	return sm.classifyTag(content)
}

func (sm *stateMachine) classifyTag(candidate string) (model.Result, bool) {
	tagEnd := strings.Index(candidate, sepToken) + len(sepToken)
	tag := candidate[:tagEnd]
	remainder := candidate[tagEnd:]

	switch tag {
	case thinkOpen:
		sm.status = model.ChannelReasoning
	case thinkClose, responseOpen, responseClose:
		sm.status = model.ChannelAnswer
	case toolsOpen:
		sm.status = model.ChannelTool
		sm.inTools = true
		return model.Result{Channel: model.ChannelTool, Content: candidate}, false
	default:
		// Message wrappers are structural. Unknown Kimi tags are also omitted
		// rather than leaking protocol markup into user-visible content.
	}

	if remainder == "" {
		return model.Result{}, false
	}
	return sm.Classify(remainder)
}

func firstControlTag(content string) int {
	openAt := strings.Index(content, openToken)
	closeAt := strings.Index(content, closeToken)
	if openAt == -1 {
		return closeAt
	}
	if closeAt == -1 || openAt < closeAt {
		return openAt
	}
	return closeAt
}

func possibleControlTag(candidate string) bool {
	return strings.HasPrefix(openToken, candidate) ||
		strings.HasPrefix(closeToken, candidate) ||
		strings.HasPrefix(candidate, openToken) ||
		strings.HasPrefix(candidate, closeToken)
}

func pendingControlSuffix(content string) int {
	for _, marker := range []string{openToken, closeToken} {
		limit := min(len(content), len(marker)-1)
		for size := limit; size > 0; size-- {
			if strings.HasSuffix(content, marker[:size]) {
				return len(content) - size
			}
		}
	}
	return -1
}
