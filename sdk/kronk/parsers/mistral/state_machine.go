package mistral

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine recognizes Mistral framing independently of decoded chunk
// boundaries. Native tool output is retained verbatim until EOS.
type stateMachine struct {
	status      model.Channel
	pending     strings.Builder
	toolCallBuf strings.Builder
	inToolCall  bool
	output      []model.Result
}

var streamMarkers = []string{toolCallsMarker, "<think>", "</think>", "[THINK]", "[/THINK]"}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pending.Reset()
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.output = nil
}

// Classify classifies a single decoded chunk.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.inToolCall {
		sm.toolCallBuf.WriteString(content)
	} else {
		sm.pending.WriteString(content)
		sm.scan()
	}
	return sm.popOutput(), false
}

func (sm *stateMachine) scan() {
	for sm.pending.Len() > 0 {
		candidate := sm.pending.String()
		at, marker := nextMarker(candidate)
		if at >= 0 {
			sm.emit(candidate[:at])
			rest := candidate[at+len(marker):]
			sm.pending.Reset()
			if marker == toolCallsMarker {
				sm.inToolCall = true
				sm.status = model.ChannelTool
				sm.toolCallBuf.WriteString(marker)
				sm.toolCallBuf.WriteString(rest)
				return
			}
			sm.setReasoning(marker)
			sm.pending.WriteString(rest)
			continue
		}

		keep := partialMarkerSuffix(candidate)
		sm.emit(candidate[:len(candidate)-keep])
		sm.pending.Reset()
		sm.pending.WriteString(candidate[len(candidate)-keep:])
		return
	}
}

func (sm *stateMachine) emit(content string) {
	if content == "" {
		return
	}
	result := model.Result{Channel: sm.status, Content: content}
	if len(sm.output) > 0 && sm.output[len(sm.output)-1].Channel == result.Channel {
		sm.output[len(sm.output)-1].Content += content
		return
	}
	sm.output = append(sm.output, result)
}

func (sm *stateMachine) setReasoning(marker string) {
	switch marker {
	case "<think>", "[THINK]":
		sm.status = model.ChannelReasoning
	default:
		sm.status = model.ChannelAnswer
	}
}

func (sm *stateMachine) popOutput() model.Result {
	if len(sm.output) == 0 {
		return model.Result{}
	}
	result := sm.output[0]
	sm.output = sm.output[1:]
	return result
}

func nextMarker(content string) (int, string) {
	at := -1
	marker := ""
	for _, candidate := range streamMarkers {
		index := strings.Index(content, candidate)
		if index >= 0 && (at < 0 || index < at) {
			at = index
			marker = candidate
		}
	}
	return at, marker
}

func partialMarkerSuffix(content string) int {
	for length := min(len(content), len(toolCallsMarker)-1); length > 0; length-- {
		suffix := content[len(content)-length:]
		for _, marker := range streamMarkers {
			if strings.HasPrefix(marker, suffix) {
				return length
			}
		}
	}
	return 0
}

// Flush releases buffered output at EOS. Partial marker prefixes are tool
// evidence so the strict full-buffer parser, rather than answer handling,
// decides their validity.
func (sm *stateMachine) Flush() model.Result {
	if result := sm.popOutput(); result != (model.Result{}) {
		return result
	}
	if sm.toolCallBuf.Len() > 0 {
		content := sm.toolCallBuf.String()
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		return model.Result{Channel: model.ChannelTool, Content: content}
	}
	if sm.pending.Len() > 0 {
		content := sm.pending.String()
		sm.pending.Reset()
		if strings.HasPrefix(toolCallsMarker, content) {
			return model.Result{Channel: model.ChannelTool, Content: content}
		}
		return model.Result{Channel: sm.status, Content: content}
	}
	return model.Result{}
}
