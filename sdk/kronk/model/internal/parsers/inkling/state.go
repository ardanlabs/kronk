package inkling

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

type stateMachine struct {
	input   string
	channel parsers.Channel
	output  []parsers.Result
	ended   bool
	// Tool frames stay in input until EOG or Flush. The cursor scans only new
	// bytes, ignoring marker-like strings in JSON, including escaped quotes.
	cursor  int
	quoted  bool
	escaped bool
}

// Reset discards every retained byte and queued channel from the previous turn.
func (sm *stateMachine) Reset() { *sm = stateMachine{} }

// Classify consumes a piece independently of tokenizer boundaries.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	sm.input += content
	sm.consume(false)
	return sm.pop(), sm.ended
}

// ConsumeVocabEOG retains the native turn terminator for final validation.
func (sm *stateMachine) ConsumeVocabEOG(content string) {
	if content == endSampling && sm.channel == parsers.ChannelTool {
		// Native token identity is authoritative even with truncated JSON.
		sm.input += content
		sm.emit(parsers.ChannelTool, sm.input)
		sm.input = ""
		sm.ended = true
		return
	}
	sm.input += content
	sm.consume(false)
}

func (sm *stateMachine) consume(flush bool) {
	for sm.input != "" && !sm.ended {
		switch sm.channel {
		case parsers.ChannelNone:
			at, marker := nextMarker(sm.input, thinking, text, invoke, endSampling)
			if at < 0 {
				if flush && strings.TrimSpace(sm.input) != "" && sm.input != messageModel {
					sm.emit(parsers.ChannelTool, sm.input)
					sm.input = ""
				}
				return
			}
			if marker == endSampling {
				if strings.TrimSpace(sm.input[:at]) != "" {
					sm.emit(parsers.ChannelTool, sm.input[:at])
				}
				sm.input = sm.input[at+len(marker):]
				sm.ended = true
				continue
			}
			if marker == invoke {
				// Accept the open assistant header used by earlier Inkling
				// templates, as well as Inkling 2's complete message prefix.
				if !strings.HasPrefix(sm.input, messageModel) {
					sm.input = messageModel + sm.input
				}
				sm.channel = parsers.ChannelTool
				continue
			}
			sm.input = sm.input[at+len(marker):]
			sm.channel = parsers.ChannelAnswer
			if marker == thinking {
				sm.channel = parsers.ChannelReasoning
			}
		case parsers.ChannelTool:
			sm.scanTools()
			if flush && !sm.ended {
				sm.emit(parsers.ChannelTool, sm.input)
				sm.input = ""
			}
			return
		default:
			at, marker := nextMarker(sm.input, endMessage, endSampling)
			if at < 0 {
				keep := 0
				if !flush {
					keep = max(partialSuffix(sm.input, endMessage), partialSuffix(sm.input, endSampling))
				}
				sm.emit(sm.channel, sm.input[:len(sm.input)-keep])
				sm.input = sm.input[len(sm.input)-keep:]
				return
			}
			sm.emit(sm.channel, sm.input[:at])
			sm.input = sm.input[at+len(marker):]
			sm.channel = parsers.ChannelNone
			sm.ended = marker == endSampling
		}
	}
	if sm.ended && sm.input != "" {
		// Do not silently discard malformed continuations after a terminator.
		sm.emit(parsers.ChannelTool, sm.input)
		sm.input = ""
	}
}

func (sm *stateMachine) scanTools() {
	for sm.cursor < len(sm.input) {
		c := sm.input[sm.cursor]
		if !sm.quoted && c == '<' {
			rest := sm.input[sm.cursor:]
			if strings.HasPrefix(rest, endSampling) {
				sm.emit(parsers.ChannelTool, sm.input)
				sm.input = ""
				sm.ended = true
				return
			}
			if strings.HasPrefix(endSampling, rest) {
				return
			}
		}
		switch {
		case sm.escaped:
			sm.escaped = false
		case sm.quoted && c == '\\':
			sm.escaped = true
		case c == '"':
			sm.quoted = !sm.quoted
		}
		sm.cursor++
	}
}

func (sm *stateMachine) emit(channel parsers.Channel, content string) {
	if content != "" {
		sm.output = append(sm.output, parsers.Result{Channel: channel, Content: content})
	}
}

func (sm *stateMachine) pop() parsers.Result {
	if len(sm.output) == 0 {
		return parsers.Result{}
	}
	r := sm.output[0]
	sm.output = sm.output[1:]
	return r
}

// Flush drains trailing channels, including malformed or truncated tool frames.
func (sm *stateMachine) Flush() parsers.Result {
	if len(sm.output) == 0 {
		sm.consume(true)
	}
	return sm.pop()
}

func nextMarker(s string, markers ...string) (int, string) {
	idx, marker := -1, ""
	for _, candidate := range markers {
		if at := strings.Index(s, candidate); at >= 0 && (idx < 0 || at < idx) {
			idx, marker = at, candidate
		}
	}
	return idx, marker
}

func partialSuffix(s, marker string) int {
	for n := min(len(s), len(marker)-1); n > 0; n-- {
		if strings.HasSuffix(s, marker[:n]) {
			return n
		}
	}
	return 0
}
