package deepseek

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// stateMachine classifies DeepSeek reasoning, answer, and DSML tool output.
// The pending opener buffer handles tokenizers that split the DSML marker and
// tag name across several decoded tokens.
type stateMachine struct {
	status  parsers.Channel
	markers dsmlMarkers

	pendingOpener strings.Builder
	toolCallBuf   strings.Builder
	inPending     bool
	inToolCall    bool
	afterToolCall bool
	malformedTool bool

	toolCallDeltas []parsers.ResponseToolCallDelta
	startedCalls   []parsers.ResponseToolCallDelta
	detectedCalls  int
	output         []parsers.Result
}

// Reset returns the stateMachine to its initial state for reuse.
func (sm *stateMachine) Reset() {
	sm.status = parsers.ChannelAnswer
	sm.markers = dsmlMarkers{}
	sm.pendingOpener.Reset()
	sm.toolCallBuf.Reset()
	sm.inPending = false
	sm.inToolCall = false
	sm.afterToolCall = false
	sm.malformedTool = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
	sm.output = nil
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	result, eog := sm.classify(content)
	sm.enqueue(result)

	return sm.popOutput(), eog
}

func (sm *stateMachine) classify(content string) (parsers.Result, bool) {
	if sm.malformedTool {
		return parsers.Result{Channel: parsers.ChannelTool, Content: content}, false
	}

	if sm.inToolCall {
		return sm.bufferToolContent(content), false
	}

	if sm.inPending {
		sm.pendingOpener.WriteString(content)
		candidate := sm.pendingOpener.String()

		switch {
		case strings.HasPrefix(candidate, toolCallsOpen) || strings.HasPrefix(candidate, toolCallsOpenV41):
			sm.inPending = false
			sm.pendingOpener.Reset()
			sm.startToolCall(candidate)
			return sm.completeBufferedTool(), false

		case strings.HasPrefix(toolCallsOpen, candidate) || strings.HasPrefix(toolCallsOpenV41, candidate):
			return parsers.Result{}, false

		default:
			sm.inPending = false
			sm.pendingOpener.Reset()
			if sm.afterToolCall {
				sm.afterToolCall = false
				sm.malformedTool = true
				return parsers.Result{Channel: parsers.ChannelTool, Content: candidate}, false
			}
			return sm.classifyContent(candidate)
		}
	}

	if sm.afterToolCall {
		trimmed := strings.TrimLeft(content, " \t\r\n")
		if trimmed == "" {
			return parsers.Result{}, false
		}
		if strings.HasPrefix(trimmed, toolCallsOpen) || strings.HasPrefix(toolCallsOpen, trimmed) ||
			strings.HasPrefix(trimmed, toolCallsOpenV41) || strings.HasPrefix(toolCallsOpenV41, trimmed) {
			return sm.classifyContent(trimmed)
		}
		sm.afterToolCall = false
		sm.malformedTool = true
		return parsers.Result{Channel: parsers.ChannelTool, Content: trimmed}, false
	}

	return sm.classifyContent(content)
}

// Flush drains an unresolved DSML opener prefix or an incomplete tools block.
func (sm *stateMachine) Flush() parsers.Result {
	if result := sm.popOutput(); result != (parsers.Result{}) {
		return result
	}

	if sm.inToolCall {
		content := sm.toolCallBuf.String()
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		return parsers.Result{Channel: parsers.ChannelTool, Content: content}
	}
	if !sm.inPending {
		return parsers.Result{}
	}

	channel := sm.status
	if sm.afterToolCall {
		channel = parsers.ChannelTool
		sm.afterToolCall = false
	}
	result := parsers.Result{Channel: channel, Content: sm.pendingOpener.String()}
	sm.pendingOpener.Reset()
	sm.inPending = false

	return result
}

// ToolCallDeltas drains tool-call activity deltas discovered since the last
// call.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities discovered in this request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta {
	return sm.startedCalls
}

func (sm *stateMachine) classifyContent(content string) (parsers.Result, bool) {
	switch content {
	case "<think>":
		sm.status = parsers.ChannelReasoning
		return parsers.Result{}, false
	case "</think>":
		sm.status = parsers.ChannelAnswer
		return parsers.Result{}, false
	}

	openerAt := strings.Index(content, toolCallsOpen)
	if v41At := strings.Index(content, toolCallsOpenV41); v41At != -1 && (openerAt == -1 || v41At < openerAt) {
		openerAt = v41At
	}
	if openerAt != -1 {
		sm.startToolCall(content[openerAt:])
		result := sm.completeBufferedTool()
		if openerAt > 0 {
			prefix := parsers.Result{Channel: sm.status, Content: content[:openerAt]}
			if result.Content == "" {
				return prefix, false
			}
			sm.enqueue(prefix)
			sm.enqueue(result)
			return parsers.Result{}, false
		}
		return result, false
	}

	if suffixAt := pendingOpenerSuffix(content); suffixAt != -1 {
		sm.inPending = true
		sm.pendingOpener.WriteString(content[suffixAt:])
		content = content[:suffixAt]
		if content == "" {
			return parsers.Result{}, false
		}
	}

	return parsers.Result{Channel: sm.status, Content: content}, false
}

func (sm *stateMachine) enqueue(result parsers.Result) {
	if result != (parsers.Result{}) {
		sm.output = append(sm.output, result)
	}
}

func (sm *stateMachine) popOutput() parsers.Result {
	if len(sm.output) == 0 {
		return parsers.Result{}
	}

	result := sm.output[0]
	sm.output = sm.output[1:]
	return result
}

func (sm *stateMachine) startToolCall(content string) {
	sm.inToolCall = true
	sm.afterToolCall = false
	sm.markers = markersFor(content)
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(content)
	sm.detectedCalls = 0
	sm.updateToolCallDeltas()
}

func (sm *stateMachine) bufferToolContent(content string) parsers.Result {
	sm.toolCallBuf.WriteString(content)
	sm.updateToolCallDeltas()
	return sm.completeBufferedTool()
}

func (sm *stateMachine) completeBufferedTool() parsers.Result {
	content := sm.toolCallBuf.String()
	closeAt := strings.LastIndex(content, sm.markers.toolCallsClose)
	if closeAt == -1 {
		return parsers.Result{}
	}

	end := closeAt + len(sm.markers.toolCallsClose)
	complete := content[:end]
	remainder := content[end:]
	sm.toolCallBuf.Reset()
	sm.toolCallBuf.WriteString(remainder)
	sm.inToolCall = remainder != ""
	sm.afterToolCall = remainder == ""
	sm.detectedCalls = countInvokeOpeners(remainder, sm.markers.invokeOpen)
	return parsers.Result{Channel: parsers.ChannelTool, Content: complete}
}

func countInvokeOpeners(content, marker string) int {
	count := 0
	for offset := 0; ; count++ {
		invokeAt := strings.Index(content[offset:], marker)
		if invokeAt == -1 {
			return count
		}
		offset += invokeAt + len(marker)
	}
}

func (sm *stateMachine) updateToolCallDeltas() {
	content := sm.toolCallBuf.String()
	offset := 0
	seen := 0
	for {
		invokeAt := strings.Index(content[offset:], sm.markers.invokeOpen)
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
			attributes, err := parseElementAttributes(content[invokeAt:openerEnd+1], sm.markers.invokeOpen, ">", "name")
			if err == nil {
				sm.addToolCallDelta(attributes["name"])
			}
		}
		seen++
		offset = openerEnd + 1
	}
}

func (sm *stateMachine) addToolCallDelta(name string) {
	delta := parsers.ResponseToolCallDelta{
		ID:    newToolCallID(),
		Index: len(sm.startedCalls),
		Type:  "function",
		Function: parsers.ResponseToolCallDeltaFunction{
			Name: name,
		},
	}
	sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
	sm.startedCalls = append(sm.startedCalls, delta)
	sm.detectedCalls++
}

func pendingOpenerSuffix(content string) int {
	limit := min(len(content), max(len(toolCallsOpen), len(toolCallsOpenV41))-1)
	for size := limit; size > 0; size-- {
		if (size < len(toolCallsOpen) && strings.HasSuffix(content, toolCallsOpen[:size])) ||
			(size < len(toolCallsOpenV41) && strings.HasSuffix(content, toolCallsOpenV41[:size])) {
			return len(content) - size
		}
	}

	return -1
}
