package lfm

import (
	"encoding/json"
	"strings"

	"uuid"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

type stateMachine struct {
	// Vocabulary policy belongs to the slot and survives request resets.
	specialMarkers map[string]bool
	toolDone       bool
	tokenAware     bool

	channel parsers.Channel
	pending string
	tool    strings.Builder
	inTool  bool
	quote   byte
	escaped bool
	queue   []parsers.Result

	deltas  []parsers.ResponseToolCallDelta
	started []parsers.ResponseToolCallDelta
	seen    int
}

// Reset returns the state machine to its initial state.
func (sm *stateMachine) Reset() {
	sm.toolDone = false
	sm.tokenAware = false
	sm.channel = parsers.ChannelAnswer
	sm.pending = ""
	sm.tool.Reset()
	sm.inTool = false
	sm.quote = 0
	sm.escaped = false
	sm.queue = nil
	sm.deltas = nil
	sm.started = nil
	sm.seen = 0
}

// Classify classifies one decoded output fragment.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	return sm.classify(content, false, false)
}

// SetTokenVocabulary resolves which framing markers require native provenance.
func (sm *stateMachine) SetTokenVocabulary(isSpecial func(string) bool) {
	sm.specialMarkers = make(map[string]bool)
	for _, marker := range []string{toolOpen, toolClose, thinkOpen, thinkClose} {
		sm.specialMarkers[marker] = isSpecial(marker)
	}
}

// ClassifyToken accepts dedicated markers only as complete special tokens.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	return sm.classify(content, true, special)
}

func (sm *stateMachine) classify(content string, tokenAware, special bool) (parsers.Result, bool) {
	sm.tokenAware = tokenAware
	text := sm.pending + content
	tokenStart, totalLen := len(sm.pending), len(text)
	markerAt := func(marker string, pos int) bool {
		return !tokenAware || !sm.specialMarkers[marker] ||
			(special && pos == tokenStart && len(marker) == len(content))
	}
	sm.process(text, tokenAware, func(marker string, pos, remaining int) bool {
		if pos < 0 {
			return !tokenAware || !sm.specialMarkers[marker]
		}
		return markerAt(marker, totalLen-remaining+pos)
	})
	if len(sm.queue) == 0 {
		return parsers.Result{}, false
	}
	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result, false
}

func (sm *stateMachine) process(text string, tokenAware bool, markerAt func(string, int, int) bool) {
	sm.pending = ""
	for text != "" {
		if sm.inTool {
			closeAt, safeAt, ok := sm.toolCloser(text, markerAt)
			if !ok {
				sm.tool.WriteString(text[:safeAt])
				sm.pending = text[safeAt:]
				sm.updateDeltas(sm.tool.String())
				return
			}
			sm.tool.WriteString(text[:closeAt])
			body := sm.tool.String()
			sm.updateDeltas(body)
			// Validate before textual serialization can erase native boundaries.
			if tokenAware {
				if _, err := parseToolPayloadStrict(body); err != nil {
					body = "<invalid-tool-payload>" + body
				}
			}
			sm.enqueue(parsers.ChannelTool, toolOpen+body+toolClose)
			sm.tool.Reset()
			sm.inTool = false
			sm.toolDone = true
			sm.quote = 0
			sm.escaped = false
			text = text[closeAt+len(toolClose):]
			continue
		}

		at, marker := nextMarker(text, markerAt)
		if marker == "" {
			at = partialMarkerSuffix(text, markerAt)
			if at < 0 {
				at = len(text)
			}
			sm.enqueueText(text[:at], tokenAware)
			sm.pending = text[at:]
			return
		}
		sm.enqueueText(text[:at], tokenAware)
		if tokenAware && sm.toolDone && marker != toolOpen {
			sm.enqueue(parsers.ChannelTool, "<unexpected-content-after-tool>"+marker)
		}
		text = text[at+len(marker):]
		switch marker {
		case thinkOpen:
			sm.channel = parsers.ChannelReasoning
		case thinkClose:
			sm.channel = parsers.ChannelAnswer
		case toolOpen:
			sm.inTool = true
			sm.toolDone = false
			sm.seen = 0
		}
	}
}

func (sm *stateMachine) toolCloser(text string, markerAt func(string, int, int) bool) (int, int, bool) {
	for pos := 0; pos < len(text); pos++ {
		ch := text[pos]
		if sm.quote != 0 {
			if sm.escaped {
				sm.escaped = false
			} else if ch == '\\' {
				sm.escaped = true
			} else if ch == sm.quote {
				sm.quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			sm.quote = ch
			continue
		}
		if markerAt(toolClose, pos, len(text)) && strings.HasPrefix(text[pos:], toolClose) {
			return pos, pos, true
		}
	}
	if sm.quote != 0 || !markerAt(toolClose, -1, len(text)) {
		return 0, len(text), false
	}
	return 0, partialSuffix(text, toolClose), false
}

func (sm *stateMachine) enqueueText(content string, tokenAware bool) {
	if tokenAware && sm.toolDone {
		if strings.TrimSpace(content) != "" {
			sm.enqueue(parsers.ChannelTool, "<unexpected-content-after-tool>"+content)
		}
		return
	}
	sm.enqueue(sm.channel, content)
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

// Flush drains queued content or an incomplete marker or tool block.
func (sm *stateMachine) Flush() parsers.Result {
	if len(sm.queue) > 0 {
		result := sm.queue[0]
		sm.queue = sm.queue[1:]
		return result
	}
	if sm.inTool {
		content := toolOpen + sm.tool.String() + sm.pending
		if sm.tokenAware {
			content += "<missing-tool-close>"
		}
		sm.tool.Reset()
		sm.pending = ""
		sm.inTool = false
		sm.quote = 0
		sm.escaped = false
		return parsers.Result{Channel: parsers.ChannelTool, Content: content}
	}
	if sm.pending == "" {
		return parsers.Result{}
	}
	content := sm.pending
	sm.pending = ""
	if sm.tokenAware && sm.toolDone {
		return parsers.Result{Channel: parsers.ChannelTool, Content: "<unexpected-content-after-tool>" + content}
	}
	return parsers.Result{Channel: sm.channel, Content: content}
}

// ToolCallDeltas drains tool-call start deltas found by the latest classification.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.deltas
	sm.deltas = nil
	return deltas
}

// StartedToolCalls returns all tool calls announced for this request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta { return sm.started }

func (sm *stateMachine) updateDeltas(content string) {
	names := toolCallNames(content)
	if sm.seen > len(names) {
		sm.seen = len(names)
	}
	for _, callName := range names[sm.seen:] {
		delta := parsers.ResponseToolCallDelta{ID: "call_" + uuid.New().String(), Index: len(sm.started), Type: "function", Function: parsers.ResponseToolCallDeltaFunction{Name: callName}}
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

func nextMarker(content string, markerAt func(string, int, int) bool) (int, string) {
	at := len(content)
	var found string
	for _, marker := range []string{toolOpen, thinkOpen, thinkClose} {
		if pos := strings.Index(content, marker); pos >= 0 && pos < at && markerAt(marker, pos, len(content)) {
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

func partialMarkerSuffix(content string, markerAt func(string, int, int) bool) int {
	at := len(content)
	for _, marker := range []string{toolOpen, thinkOpen, thinkClose} {
		if markerAt(marker, -1, len(content)) {
			at = min(at, partialSuffix(content, marker))
		}
	}
	if at == len(content) {
		return -1
	}
	return at
}
