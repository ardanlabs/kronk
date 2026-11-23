package gpt

import (
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

var harmonyMarkers = []string{"<|constrain|>", "<|message|>", "<|channel|>", "<|return|>", "<|start|>", "<|call|>", "<|end|>"}

// stateMachine is a token-boundary-independent Harmony stream classifier.
type stateMachine struct {
	status            parsers.Channel
	collecting        bool
	awaitingHeader    bool
	awaitingChannel   bool
	awaitingConstrain bool
	headerBuf         strings.Builder
	channelBuf        strings.Builder
	constraintBuf     strings.Builder
	toolFuncName      string
	toolChannel       bool
	toolCallBuf       strings.Builder
	inputBuf          string
	results           []parsers.Result
	toolCallDeltas    []parsers.ResponseToolCallDelta
	startedCalls      []parsers.ResponseToolCallDelta
}

// Reset returns the stateMachine to its initial state.
func (sm *stateMachine) Reset() {
	*sm = stateMachine{status: parsers.ChannelNone}
}

// Classify consumes an arbitrary stream chunk and returns the first available result.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	sm.inputBuf += content
	eog := sm.consume(false)
	return sm.nextResult(), eog
}

// ClassifyToken recognizes Harmony framing only when the vocabulary identifies
// a complete special token. Ordinary tokens spelling a marker remain payload.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	var eog bool
	switch {
	case !special || !slices.Contains(harmonyMarkers, content):
		sm.consumeText(content)
	case sm.status == parsers.ChannelTool && sm.collecting && toolMarkerIsData(sm.toolCallBuf.String(), content):
		sm.toolCallBuf.WriteString(content)
	default:
		eog = sm.consumeMarker(content)
	}
	return sm.nextResult(), eog
}

// ConsumeVocabEOG consumes Harmony framing that llama also identifies as a
// vocabulary end-of-generation token, such as <|call|>.
func (sm *stateMachine) ConsumeVocabEOG(content string) {
	sm.inputBuf += content
	sm.consume(false)
}

func (sm *stateMachine) consume(flush bool) bool {
	var eog bool
	for sm.inputBuf != "" {
		idx, marker := nextHarmonyMarker(sm.inputBuf)
		if idx < 0 {
			keep := partialMarkerSuffix(sm.inputBuf)
			if flush {
				keep = 0
			}
			sm.consumeText(sm.inputBuf[:len(sm.inputBuf)-keep])
			sm.inputBuf = sm.inputBuf[len(sm.inputBuf)-keep:]
			break
		}

		if idx > 0 {
			sm.consumeText(sm.inputBuf[:idx])
			sm.inputBuf = sm.inputBuf[idx:]
			continue
		}

		// Harmony-like text in a tool JSON string is payload, not framing.
		if sm.status == parsers.ChannelTool && sm.collecting && toolMarkerIsData(sm.toolCallBuf.String(), marker) {
			sm.toolCallBuf.WriteString(marker)
			sm.inputBuf = sm.inputBuf[len(marker):]
			continue
		}

		sm.inputBuf = sm.inputBuf[len(marker):]
		if sm.consumeMarker(marker) {
			eog = true
			if sm.toolCallBuf.Len() > 0 && trimASCIIWhitespace(sm.inputBuf) != "" {
				sm.toolCallBuf.WriteString("<|post-eog|>")
				sm.toolCallBuf.WriteString(sm.inputBuf)
			}
			sm.inputBuf = ""
			break
		}
	}
	return eog
}

func (sm *stateMachine) consumeText(text string) {
	if text == "" {
		return
	}
	if sm.awaitingHeader {
		sm.headerBuf.WriteString(text)
		return
	}
	if sm.awaitingConstrain {
		sm.constraintBuf.WriteString(text)
		return
	}
	if sm.awaitingChannel {
		sm.channelBuf.WriteString(text)
		return
	}
	if !sm.collecting {
		return
	}
	if sm.status == parsers.ChannelTool {
		sm.toolCallBuf.WriteString(text)
		return
	}
	if sm.status != parsers.ChannelNone {
		sm.results = append(sm.results, parsers.Result{Channel: sm.status, Content: text})
	}
}

func (sm *stateMachine) consumeMarker(marker string) bool {
	if sm.awaitingHeader {
		if marker == "<|channel|>" {
			sm.finishHeader()
			sm.awaitingChannel = true
			sm.channelBuf.Reset()
			return false
		}
		sm.awaitingHeader = false
		sm.headerBuf.Reset()
	}

	if sm.awaitingConstrain {
		if marker == messageMarker {
			constraint := trimASCIIWhitespace(sm.constraintBuf.String())
			if constraint != "json" && (sm.status != parsers.ChannelAnswer || !validConstraintType(constraint)) {
				sm.preserveMalformed(marker)
				return false
			}
			sm.constraintBuf.Reset()
			sm.awaitingConstrain = false
			sm.beginMessage()
			return false
		}
		sm.preserveMalformed(marker)
		return false
	}

	if sm.awaitingChannel {
		if marker == messageMarker || marker == "<|constrain|>" {
			sm.finishChannel(marker == "<|constrain|>")
			return false
		}
		sm.preserveMalformed(marker)
		return false
	}

	if sm.collecting {
		switch marker {
		case "<|call|>", "<|return|>":
			if sm.status == parsers.ChannelTool {
				if marker != "<|call|>" || !completeToolArguments(sm.toolCallBuf.String()) {
					sm.toolCallBuf.WriteString(marker)
				}
			}
			sm.collecting = false
			sm.status = parsers.ChannelNone
			return true
		case "<|end|>":
			if sm.status == parsers.ChannelTool {
				sm.toolCallBuf.WriteString(marker)
				sm.queueToolCall()
			}
			sm.collecting = false
			sm.status = parsers.ChannelNone
			return false
		case "<|start|>", "<|channel|>":
			if sm.status == parsers.ChannelTool {
				sm.toolCallBuf.WriteString(marker)
				sm.queueToolCall()
			}
			sm.collecting = false
			sm.status = parsers.ChannelNone
			if marker == "<|start|>" {
				sm.awaitingHeader = true
				sm.headerBuf.Reset()
			} else {
				sm.awaitingChannel = true
				sm.channelBuf.Reset()
			}
			return false
		default:
			if sm.status == parsers.ChannelTool {
				sm.toolCallBuf.WriteString(marker)
			}
			return false
		}
	}

	switch marker {
	case "<|start|>":
		sm.status = parsers.ChannelNone
		sm.awaitingHeader = true
		sm.headerBuf.Reset()
	case "<|channel|>":
		sm.awaitingChannel = true
		sm.channelBuf.Reset()
	case messageMarker:
		sm.collecting = true
	}
	return false
}

func (sm *stateMachine) finishHeader() {
	header := trimASCIIWhitespace(sm.headerBuf.String())
	sm.headerBuf.Reset()
	sm.awaitingHeader = false

	role, recipient, ok := strings.Cut(header, " to=functions.")
	if ok && role == "assistant" {
		sm.toolFuncName = recipient
	}
}

func (sm *stateMachine) finishChannel(constrained bool) {
	name := trimASCIIWhitespace(sm.channelBuf.String())
	sm.channelBuf.Reset()
	sm.awaitingChannel = false
	sm.status = parsers.ChannelNone

	channel, recipient, hasRecipient := strings.Cut(name, " to=functions.")
	if !constrained && !hasRecipient {
		if idx := strings.IndexFunc(channel, isASCIIWhitespace); idx > 0 && channel[:idx] == "final" && validConstraintType(trimASCIIWhitespace(channel[idx:])) {
			channel = "final"
		}
	}
	if hasRecipient && (channel == "analysis" || channel == "commentary") {
		sm.status = parsers.ChannelTool
		sm.toolFuncName = recipient
		sm.toolChannel = true
	} else if sm.toolFuncName != "" && (channel == "analysis" || channel == "commentary") {
		sm.status = parsers.ChannelTool
		sm.toolChannel = true
	} else {
		sm.toolFuncName = ""
		switch channel {
		case "analysis":
			sm.status = parsers.ChannelReasoning
		case "final":
			sm.status = parsers.ChannelAnswer
		}
	}
	if constrained {
		sm.awaitingConstrain = true
		sm.constraintBuf.Reset()
	} else {
		sm.beginMessage()
	}
}

// validConstraintType matches Harmony's nonempty [A-Za-z0-9_-] label.
func validConstraintType(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func (sm *stateMachine) beginMessage() {
	sm.collecting = true
	if sm.status == parsers.ChannelTool && sm.toolChannel {
		sm.toolCallBuf.WriteByte('.')
		sm.toolCallBuf.WriteString(sm.toolFuncName)
		sm.toolCallBuf.WriteByte(' ')
		sm.toolCallBuf.WriteString(messageMarker)
		sm.toolFuncName = ""
		sm.toolChannel = false
	}
}

func (sm *stateMachine) preserveMalformed(marker string) {
	channel := trimASCIIWhitespace(sm.channelBuf.String())
	if sm.toolCallBuf.Len() == 0 && sm.toolFuncName != "" {
		sm.toolCallBuf.WriteByte('.')
		sm.toolCallBuf.WriteString(sm.toolFuncName)
		sm.toolCallBuf.WriteString(" <|invalid-framing|>")
	} else if sm.toolCallBuf.Len() == 0 && strings.HasPrefix(channel, "commentary") {
		sm.toolCallBuf.WriteString(channel)
	}
	if sm.status == parsers.ChannelTool || sm.toolCallBuf.Len() > 0 {
		sm.toolCallBuf.WriteString(sm.constraintBuf.String())
		sm.toolCallBuf.WriteString(marker)
		sm.queueToolCall()
	}
	sm.channelBuf.Reset()
	sm.headerBuf.Reset()
	sm.constraintBuf.Reset()
	sm.toolFuncName = ""
	sm.toolChannel = false
	sm.awaitingHeader = false
	sm.awaitingChannel = false
	sm.awaitingConstrain = false
	sm.collecting = false
	sm.status = parsers.ChannelNone
}

func (sm *stateMachine) queueToolCall() {
	if sm.toolCallBuf.Len() == 0 {
		return
	}
	sm.results = append(sm.results, parsers.Result{Channel: parsers.ChannelTool, Content: sm.toolCallBuf.String()})
	sm.toolCallBuf.Reset()
}

func (sm *stateMachine) nextResult() parsers.Result {
	if len(sm.results) == 0 {
		return parsers.Result{}
	}
	result := sm.results[0]
	sm.results = sm.results[1:]
	return result
}

// Flush releases queued output and incomplete framing without discarding evidence.
func (sm *stateMachine) Flush() parsers.Result {
	if len(sm.results) > 0 {
		return sm.nextResult()
	}
	sm.consume(true)
	if sm.awaitingChannel && strings.HasPrefix(strings.TrimSpace(sm.channelBuf.String()), "commentary") {
		sm.toolCallBuf.WriteString(sm.channelBuf.String())
		sm.channelBuf.Reset()
		sm.awaitingChannel = false
	}
	if sm.toolFuncName != "" && sm.toolCallBuf.Len() == 0 {
		sm.toolCallBuf.WriteByte('.')
		sm.toolCallBuf.WriteString(sm.toolFuncName)
		sm.toolFuncName = ""
	}
	if sm.collecting && sm.status == parsers.ChannelTool && completeToolArguments(sm.toolCallBuf.String()) {
		sm.toolCallBuf.WriteString("<|missing-end|>")
	}
	if sm.toolCallBuf.Len() > 0 {
		sm.queueToolCall()
	}
	return sm.nextResult()
}

// ToolCallDeltas drains tool-call identity deltas produced by Classify.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns identities emitted during the current request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta { return sm.startedCalls }

func nextHarmonyMarker(s string) (int, string) {
	best := -1
	var marker string
	for _, candidate := range harmonyMarkers {
		if idx := strings.Index(s, candidate); idx >= 0 && (best < 0 || idx < best) {
			best, marker = idx, candidate
		}
	}
	return best, marker
}

func partialMarkerSuffix(s string) int {
	best := 0
	for _, marker := range harmonyMarkers {
		for n := 1; n < len(marker) && n <= len(s); n++ {
			if strings.HasSuffix(s, marker[:n]) {
				best = max(best, n)
			}
		}
	}
	return best
}

func toolMarkerIsData(payload, marker string) bool {
	_, inString, _ := jsonStructure(payload)
	return inString || completeToolArguments(payload) && marker == messageMarker
}

func completeToolArguments(payload string) bool {
	_, after, ok := strings.Cut(payload, messageMarker)
	if !ok {
		return false
	}
	args := trimASCIIWhitespace(after)
	end, err := strictJSONObjectEnd(args)
	return err == nil && trimASCIIWhitespace(args[end:]) == ""
}

func jsonStructure(s string) (depth int, inString, escape bool) {
	idx := strings.Index(s, messageMarker)
	if idx >= 0 {
		s = s[idx+len(messageMarker):]
	}
	for i := range len(s) {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inString && c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
		} else if !inString && c == '{' {
			depth++
		} else if !inString && c == '}' {
			depth--
		}
	}
	return depth, inString, escape
}
