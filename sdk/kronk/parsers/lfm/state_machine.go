package lfm

import (
	"encoding/json"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

type stateMachine struct {
	channel model.Channel
	pending string
	tool    strings.Builder
	inTool  bool
	queue   []model.Result

	deltas  []model.ResponseToolCallDelta
	started []model.ResponseToolCallDelta
	seen    int
}

// Reset returns the state machine to its initial state.
func (sm *stateMachine) Reset() {
	sm.channel = model.ChannelAnswer
	sm.pending = ""
	sm.tool.Reset()
	sm.inTool = false
	sm.queue = nil
	sm.deltas = nil
	sm.started = nil
	sm.seen = 0
}

// Classify classifies one decoded output fragment.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	sm.process(sm.pending + content)
	if len(sm.queue) == 0 {
		return model.Result{}, false
	}
	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result, false
}

func (sm *stateMachine) process(text string) {
	sm.pending = ""
	for text != "" {
		if sm.inTool {
			before, after, ok := strings.Cut(text, toolClose)
			if !ok {
				at := partialSuffix(text, toolClose)
				sm.tool.WriteString(text[:at])
				sm.pending = text[at:]
				sm.updateDeltas(sm.tool.String())
				return
			}
			sm.tool.WriteString(before)
			body := sm.tool.String()
			sm.updateDeltas(body)
			sm.enqueue(model.ChannelTool, body)
			sm.tool.Reset()
			sm.inTool = false
			text = after
			continue
		}

		at, marker := nextMarker(text)
		if marker == "" {
			at = partialMarkerSuffix(text)
			if at < 0 {
				at = len(text)
			}
			sm.enqueue(sm.channel, text[:at])
			sm.pending = text[at:]
			return
		}
		sm.enqueue(sm.channel, text[:at])
		text = text[at+len(marker):]
		switch marker {
		case thinkOpen:
			sm.channel = model.ChannelReasoning
		case thinkClose:
			sm.channel = model.ChannelAnswer
		case toolOpen:
			sm.inTool = true
			sm.seen = 0
		}
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

// Flush drains queued content or an incomplete marker or tool block.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.queue) > 0 {
		result := sm.queue[0]
		sm.queue = sm.queue[1:]
		return result
	}
	if sm.inTool {
		content := sm.tool.String() + sm.pending
		sm.tool.Reset()
		sm.pending = ""
		sm.inTool = false
		return model.Result{Channel: model.ChannelTool, Content: content}
	}
	if sm.pending == "" {
		return model.Result{}
	}
	content := sm.pending
	sm.pending = ""
	return model.Result{Channel: sm.channel, Content: content}
}

// ToolCallDeltas drains tool-call start deltas found by the latest classification.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.deltas
	sm.deltas = nil
	return deltas
}

// StartedToolCalls returns all tool calls announced for this request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta { return sm.started }

func (sm *stateMachine) updateDeltas(content string) {
	names := toolCallNames(content)
	if sm.seen > len(names) {
		sm.seen = len(names)
	}
	for _, callName := range names[sm.seen:] {
		delta := model.ResponseToolCallDelta{ID: "call_" + uuid.NewString(), Index: len(sm.started), Type: "function", Function: model.ResponseToolCallDeltaFunction{Name: callName}}
		sm.deltas = append(sm.deltas, delta)
		sm.started = append(sm.started, delta)
	}
	sm.seen = len(names)
}

func toolCallNames(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	if trimmed[0] == '{' || jsonArrayEnvelope(trimmed) {
		var value any
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return nil
		}
		var names []string
		collectJSONNames(value, &names)
		return names
	}
	return pythonCallNames(trimmed)
}

func collectJSONNames(value any, names *[]string) {
	items := []any{value}
	if array, ok := value.([]any); ok {
		items = array
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, nameOK := object["name"].(string)
		_, argsOK := object["arguments"].(map[string]any)
		if nameOK && name != "" && argsOK {
			*names = append(*names, name)
		}
	}
}

func pythonCallNames(content string) []string {
	pos := skipWhitespace(content, 0)
	depth := 0
	if pos < len(content) && content[pos] == '[' {
		depth = 1
		pos++
	}
	var names []string
	expectCall := true
	for pos < len(content) {
		pos = skipWhitespace(content, pos)
		if expectCall && (depth == 0 || depth == 1) && pos < len(content) && isIdentStart(content[pos]) {
			start := pos
			for pos < len(content) && isIdent(content[pos]) {
				pos++
			}
			if next := skipWhitespace(content, pos); next < len(content) && content[next] == '(' {
				names = append(names, content[start:pos])
				expectCall = false
				pos = next
			}
		}
		if pos >= len(content) {
			break
		}
		ch := content[pos]
		if ch == '\'' || ch == '"' {
			pos = skipString(content, pos)
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', '}', ']':
			depth--
		case ',':
			if depth == 1 || depth == 0 {
				expectCall = true
			}
		}
		pos++
	}
	return names
}

func skipString(content string, pos int) int {
	quote := content[pos]
	for pos++; pos < len(content); pos++ {
		if content[pos] == '\\' {
			pos++
			continue
		}
		if content[pos] == quote {
			return pos + 1
		}
	}
	return pos
}

func nextMarker(content string) (int, string) {
	at := len(content)
	var found string
	for _, marker := range []string{toolOpen, thinkOpen, thinkClose} {
		if pos := strings.Index(content, marker); pos >= 0 && pos < at {
			at, found = pos, marker
		}
	}
	return at, found
}

func partialSuffix(content, marker string) int {
	for i := range content {
		if strings.HasPrefix(marker, content[i:]) {
			return i
		}
	}
	return len(content)
}

func partialMarkerSuffix(content string) int {
	at := len(content)
	for _, marker := range []string{toolOpen, thinkOpen, thinkClose} {
		at = min(at, partialSuffix(content, marker))
	}
	if at == len(content) {
		return -1
	}
	return at
}
