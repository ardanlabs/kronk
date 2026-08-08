package toolcall

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

var openMarkers = []string{"<tool_call>", "<|tool_call>"}
var closeMarkers = []string{"</tool_call>", "<tool_call|>"}

// stateMachine incrementally extracts marked JSON tool calls.
type stateMachine struct {
	status   model.Channel
	pending  string
	tool     strings.Builder
	inTool   bool
	toolDone bool
	inString bool
	escape   bool
	queue    []model.Result
}

func (sm *stateMachine) Reset() {
	*sm = stateMachine{status: model.ChannelAnswer}
}

func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	sm.pending += content
	for sm.pending != "" {
		if sm.inTool {
			marker, partial := matchingMarker(sm.pending, closeMarkers)
			if !sm.inString && marker != "" {
				sm.pending = sm.pending[len(marker):]
				body := sm.tool.String()
				sm.tool.Reset()
				sm.inTool = false
				sm.toolDone = true
				if strings.Trim(body, " \t\r\n") == "" {
					body = "<empty-tool-call>"
				} else if !completeObjectSequence(body) {
					body += "<invalid-tool-call-boundary>"
				}
				sm.enqueue(model.ChannelTool, body+"\n")
				continue
			}
			if !sm.inString && partial {
				break
			}
			sm.consumeToolByte(sm.pending[0])
			sm.pending = sm.pending[1:]
			continue
		}

		marker, partial := matchingMarker(sm.pending, openMarkers)
		if marker != "" {
			sm.pending = sm.pending[len(marker):]
			sm.beginTool()
			continue
		}
		if partial {
			break
		}
		if sm.toolDone && strings.ContainsRune(" \t\r\n", rune(sm.pending[0])) {
			sm.pending = sm.pending[1:]
			continue
		}
		if sm.toolDone {
			sm.enqueue(model.ChannelTool, "<unexpected-content-after-tool>"+sm.pending)
			sm.pending = ""
			break
		}
		if strings.HasPrefix(sm.pending, "<think>") {
			sm.pending = sm.pending[len("<think>"):]
			sm.status = model.ChannelReasoning
			continue
		}
		if strings.HasPrefix(sm.pending, "</think>") {
			sm.pending = sm.pending[len("</think>"):]
			sm.status = model.ChannelAnswer
			continue
		}
		if partialControlMarker(sm.pending) {
			break
		}
		sm.enqueue(sm.status, sm.pending[:1])
		sm.pending = sm.pending[1:]
	}

	return sm.dequeue(), false
}

func partialControlMarker(s string) bool {
	for _, marker := range []string{"<think>", "</think>"} {
		if strings.HasPrefix(marker, s) {
			return true
		}
	}
	return false
}

func completeObjectSequence(body string) bool {
	remaining := strings.TrimLeft(body, " \t\r\n")
	if remaining == "" {
		return false
	}
	for remaining != "" {
		end := findJSONObjectEnd(remaining)
		if end < 0 {
			return false
		}
		remaining = strings.TrimLeft(remaining[end:], " \t\r\n")
	}
	return true
}

func matchingMarker(s string, markers []string) (string, bool) {
	for _, marker := range markers {
		if strings.HasPrefix(s, marker) {
			return marker, false
		}
	}
	for _, marker := range markers {
		if strings.HasPrefix(marker, s) {
			return "", true
		}
	}
	return "", false
}

func (sm *stateMachine) beginTool() {
	sm.inTool = true
	sm.toolDone = false
	sm.tool.Reset()
	sm.inString = false
	sm.escape = false
	sm.status = model.ChannelTool
}

func (sm *stateMachine) consumeToolByte(c byte) {
	sm.tool.WriteByte(c)
	if sm.escape {
		sm.escape = false
		return
	}
	if sm.inString && c == '\\' {
		sm.escape = true
		return
	}
	if c == '"' {
		sm.inString = !sm.inString
		return
	}
	if sm.inString {
		return
	}
}

func (sm *stateMachine) enqueue(channel model.Channel, content string) {
	if content == "" {
		return
	}
	if len(sm.queue) > 0 && sm.queue[len(sm.queue)-1].Channel == channel {
		sm.queue[len(sm.queue)-1].Content += content
		return
	}
	sm.queue = append(sm.queue, model.Result{Channel: channel, Content: content})
}

func (sm *stateMachine) dequeue() model.Result {
	if len(sm.queue) == 0 {
		return model.Result{}
	}
	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result
}

// Flush emits incomplete framing as invalid tool-channel evidence.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.queue) > 0 {
		return sm.dequeue()
	}
	if !sm.inTool {
		if sm.pending != "" {
			content := sm.pending
			sm.pending = ""
			if sm.toolDone {
				return model.Result{Channel: model.ChannelTool, Content: "<unexpected-content-after-tool>" + content}
			}
			return model.Result{Channel: sm.status, Content: content}
		}
		return model.Result{}
	}
	body := sm.tool.String() + sm.pending
	sm.tool.Reset()
	sm.pending = ""
	sm.inTool = false
	if body == "" {
		body = "<tool_call>"
	} else {
		body += "<missing-tool-call-close>"
	}
	return model.Result{Channel: model.ChannelTool, Content: body}
}
