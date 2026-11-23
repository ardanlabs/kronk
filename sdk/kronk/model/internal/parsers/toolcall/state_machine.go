package toolcall

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

var openMarkers = []string{"<tool_call>", "<|tool_call>"}
var closeMarkers = []string{"</tool_call>", "<tool_call|>"}

// stateMachine incrementally extracts marked JSON tool calls.
type stateMachine struct {
	// Vocabulary policy is slot-owned and survives request resets.
	specialMarkers map[string]bool

	status   parsers.Channel
	pending  string
	tool     strings.Builder
	inTool   bool
	toolDone bool
	inString bool
	escape   bool
	queue    []parsers.Result
}

func (sm *stateMachine) Reset() {
	*sm = stateMachine{status: parsers.ChannelAnswer, specialMarkers: sm.specialMarkers}
}

func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	return sm.classify(content, false, false)
}

// SetTokenVocabulary resolves dedicated markers while preserving ordinary-text
// framing for templates such as Ripple's and the shared reasoning spellings.
func (sm *stateMachine) SetTokenVocabulary(isSpecial func(string) bool) {
	sm.specialMarkers = make(map[string]bool)
	for _, markers := range [][]string{openMarkers, closeMarkers, {"<think>", "</think>"}} {
		for _, marker := range markers {
			sm.specialMarkers[marker] = isSpecial(marker)
		}
	}
}

// ClassifyToken requires complete special-token provenance for dedicated markers.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	return sm.classify(content, true, special)
}

func (sm *stateMachine) classify(content string, tokenAware, special bool) (parsers.Result, bool) {
	tokenStart, tokenLen := len(sm.pending), len(content)
	sm.pending += content
	totalLen := len(sm.pending)
	markerAt := func(marker string) bool {
		return !tokenAware || !sm.specialMarkers[marker] ||
			(special && totalLen-len(sm.pending) == tokenStart && len(marker) == tokenLen)
	}
	for sm.pending != "" {
		if sm.inTool {
			marker, partial := matchingMarker(sm.pending, closeMarkers, markerAt)
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
				sm.enqueue(parsers.ChannelTool, body+"\n")
				continue
			}
			if !sm.inString && partial {
				break
			}
			sm.consumeToolByte(sm.pending[0])
			sm.pending = sm.pending[1:]
			continue
		}

		marker, partial := matchingMarker(sm.pending, openMarkers, markerAt)
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
			sm.enqueue(parsers.ChannelTool, "<unexpected-content-after-tool>"+sm.pending)
			sm.pending = ""
			break
		}
		marker, partial = matchingMarker(sm.pending, []string{"<think>", "</think>"}, markerAt)
		if marker == "<think>" {
			sm.pending = sm.pending[len("<think>"):]
			sm.status = parsers.ChannelReasoning
			continue
		}
		if marker == "</think>" {
			sm.pending = sm.pending[len("</think>"):]
			sm.status = parsers.ChannelAnswer
			continue
		}
		if partial {
			break
		}
		sm.enqueue(sm.status, sm.pending[:1])
		sm.pending = sm.pending[1:]
	}

	return sm.dequeue(), false
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

func matchingMarker(s string, markers []string, markerAt func(string) bool) (string, bool) {
	for _, marker := range markers {
		if markerAt(marker) && strings.HasPrefix(s, marker) {
			return marker, false
		}
	}
	for _, marker := range markers {
		if markerAt(marker) && strings.HasPrefix(marker, s) {
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
	sm.status = parsers.ChannelTool
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

func (sm *stateMachine) enqueue(channel parsers.Channel, content string) {
	if content == "" {
		return
	}
	if len(sm.queue) > 0 && sm.queue[len(sm.queue)-1].Channel == channel {
		sm.queue[len(sm.queue)-1].Content += content
		return
	}
	sm.queue = append(sm.queue, parsers.Result{Channel: channel, Content: content})
}

func (sm *stateMachine) dequeue() parsers.Result {
	if len(sm.queue) == 0 {
		return parsers.Result{}
	}
	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result
}

// Flush emits incomplete framing as invalid tool-channel evidence.
func (sm *stateMachine) Flush() parsers.Result {
	if len(sm.queue) > 0 {
		return sm.dequeue()
	}
	if !sm.inTool {
		if sm.pending != "" {
			content := sm.pending
			sm.pending = ""
			if sm.toolDone {
				return parsers.Result{Channel: parsers.ChannelTool, Content: "<unexpected-content-after-tool>" + content}
			}
			return parsers.Result{Channel: sm.status, Content: content}
		}
		return parsers.Result{}
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
	return parsers.Result{Channel: parsers.ChannelTool, Content: body}
}
