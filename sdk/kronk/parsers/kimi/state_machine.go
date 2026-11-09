package kimi

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine classifies Kimi reasoning, response, and tool output. Kimi
// control tags may be split across decoded tokens, so a possible tag is held
// until its terminating <|sep|> arrives.
type stateMachine struct {
	status         model.Channel
	pending        strings.Builder
	tools          strings.Builder
	inTag          bool
	inTools        bool
	afterTools     bool
	messageClosed  bool
	malformedTool  bool
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	detectedCalls  int
}

// Reset returns the stateMachine to its initial state for reuse.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pending.Reset()
	sm.tools.Reset()
	sm.inTag = false
	sm.inTools = false
	sm.afterTools = false
	sm.messageClosed = false
	sm.malformedTool = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.malformedTool {
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	if sm.inTools {
		sm.tools.WriteString(content)
		sm.updateToolCallDeltas()
		return sm.completeTools(), false
	}

	if sm.afterTools {
		return sm.classifyAfterTools(content)
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

// Flush drains an unresolved Kimi control-tag prefix or incomplete tools block.
func (sm *stateMachine) Flush() model.Result {
	if sm.afterTools {
		result, _ := sm.classifyAfterTools("")
		if result.Content != "" {
			return result
		}

		if sm.pending.Len() > 0 {
			result = model.Result{Channel: model.ChannelTool, Content: sm.pending.String()}
			sm.pending.Reset()
			sm.afterTools = false
			return result
		}
	}

	if sm.inTools {
		content := sm.tools.String()
		sm.tools.Reset()
		sm.inTools = false
		return model.Result{Channel: model.ChannelTool, Content: content}
	}

	if !sm.inTag {
		return model.Result{}
	}

	result := model.Result{Channel: sm.status, Content: sm.pending.String()}
	sm.pending.Reset()
	sm.inTag = false
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
		sm.afterTools = false
		sm.tools.Reset()
		sm.tools.WriteString(candidate)
		sm.detectedCalls = 0
		sm.updateToolCallDeltas()
		return sm.completeTools(), false
	default:
		// Message wrappers are structural. Unknown Kimi tags are also omitted
		// rather than leaking protocol markup into user-visible content.
	}

	if remainder == "" {
		return model.Result{}, false
	}
	return sm.Classify(remainder)
}

func (sm *stateMachine) completeTools() model.Result {
	content := sm.tools.String()
	closeAt := strings.LastIndex(content, toolsClose)
	if closeAt == -1 {
		return model.Result{}
	}
	end := closeAt + len(toolsClose)
	complete := content[:end]
	remainder := content[end:]
	sm.tools.Reset()
	sm.inTools = false
	sm.afterTools = true
	sm.pending.WriteString(remainder)
	return model.Result{Channel: model.ChannelTool, Content: complete}
}

// classifyAfterTools accepts only another tools block or the message trailer.
// Keep incomplete trailer markers on the tool channel at Flush so malformed
// output still invalidates the entire accumulated call list.
func (sm *stateMachine) classifyAfterTools(content string) (model.Result, bool) {
	sm.pending.WriteString(content)
	candidate := strings.TrimLeft(sm.pending.String(), " \t\r\n")
	sm.pending.Reset()
	if !sm.messageClosed && strings.HasPrefix(candidate, messageClose) {
		sm.messageClosed = true
		candidate = strings.TrimLeft(candidate[len(messageClose):], " \t\r\n")
	}
	switch {
	case candidate == "":
		return model.Result{}, false
	case strings.TrimSpace(candidate) == endOfMessage:
		sm.afterTools = false
		return model.Result{}, true
	case !sm.messageClosed && strings.HasPrefix(candidate, toolsOpen):
		sm.afterTools = false
		return sm.Classify(candidate)
	case strings.HasPrefix(endOfMessage, candidate),
		!sm.messageClosed && strings.HasPrefix(messageClose, candidate),
		!sm.messageClosed && strings.HasPrefix(toolsOpen, candidate):
		sm.pending.WriteString(candidate)
		return model.Result{}, false
	default:
		sm.afterTools = false
		sm.malformedTool = true
		return model.Result{Channel: model.ChannelTool, Content: candidate}, false
	}
}

func (sm *stateMachine) updateToolCallDeltas() {
	content := sm.tools.String()
	offset := 0
	seen := 0
	for {
		callAt := strings.Index(content[offset:], callOpen)
		if callAt == -1 {
			return
		}
		callAt += offset
		openerEnd := strings.Index(content[callAt:], sepToken)
		if openerEnd == -1 {
			return
		}
		openerEnd += callAt + len(sepToken)
		if seen >= sm.detectedCalls {
			attributes, err := parseElementAttributes(content[callAt:openerEnd], callOpen, sepToken, "tool", "index")
			if err == nil {
				sm.addToolCallDelta(attributes["tool"])
			}
		}
		seen++
		offset = openerEnd
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
