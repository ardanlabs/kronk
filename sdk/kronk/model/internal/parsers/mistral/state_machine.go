package mistral

import (
	"errors"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// stateMachine recognizes Mistral framing independently of decoded chunk
// boundaries. Native tool output is retained verbatim until EOS.
type stateMachine struct {
	status      parsers.Channel
	pending     strings.Builder
	toolCallBuf strings.Builder
	inToolCall  bool
	output      []parsers.Result
	// Vocabulary policy survives Reset. Provenance is request-local.
	specialMarkers map[string]bool
	toolMarkers    map[int]bool
	tokenAware     bool
}

var streamMarkers = []string{toolCallsMarker, "<think>", "</think>", "[THINK]", "[/THINK]"}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = parsers.ChannelAnswer
	sm.pending.Reset()
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.output = nil
	sm.toolMarkers = nil
	sm.tokenAware = false
}

// Classify classifies a single decoded chunk.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	return sm.classify(content, false, false)
}

// SetTokenVocabulary resolves dedicated framing without changing the shared
// parser's supported reasoning spellings.
func (sm *stateMachine) SetTokenVocabulary(isSpecial func(string) bool) {
	sm.specialMarkers = make(map[string]bool)
	for _, marker := range append([]string{argsMarker}, streamMarkers...) {
		sm.specialMarkers[marker] = isSpecial(marker)
	}
}

// ClassifyToken requires complete native tokens for dedicated markers.
func (sm *stateMachine) ClassifyToken(content string, special bool) (parsers.Result, bool) {
	return sm.classify(content, true, special)
}

func (sm *stateMachine) classify(content string, tokenAware, special bool) (parsers.Result, bool) {
	sm.tokenAware = sm.tokenAware || tokenAware
	if sm.inToolCall {
		if special && (content == toolCallsMarker || content == argsMarker) {
			if sm.toolMarkers == nil {
				sm.toolMarkers = make(map[int]bool)
			}
			sm.toolMarkers[sm.toolCallBuf.Len()] = true
		}
		sm.toolCallBuf.WriteString(content)
	} else {
		tokenStart := sm.pending.Len()
		sm.pending.WriteString(content)
		sm.scan(tokenAware, special, tokenStart, len(content))
	}
	return sm.popOutput(), false
}

func (sm *stateMachine) scan(tokenAware, special bool, tokenStart, tokenLen int) {
	allowed := func(marker string, offset int) bool {
		return !tokenAware || !sm.specialMarkers[marker] ||
			(special && offset == tokenStart && len(marker) == tokenLen)
	}
	for sm.pending.Len() > 0 {
		candidate := sm.pending.String()
		at, marker := nextMarker(candidate, allowed)
		if at >= 0 {
			sm.emit(candidate[:at])
			rest := candidate[at+len(marker):]
			sm.pending.Reset()
			if marker == toolCallsMarker {
				sm.inToolCall = true
				sm.status = parsers.ChannelTool
				sm.toolMarkers = map[int]bool{0: true}
				sm.toolCallBuf.WriteString(marker)
				sm.toolCallBuf.WriteString(rest)
				return
			}
			sm.setReasoning(marker)
			tokenStart -= at + len(marker)
			sm.pending.WriteString(rest)
			continue
		}

		keep := partialMarkerSuffix(candidate, allowed)
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
	result := parsers.Result{Channel: sm.status, Content: content}
	if len(sm.output) > 0 && sm.output[len(sm.output)-1].Channel == result.Channel {
		sm.output[len(sm.output)-1].Content += content
		return
	}
	sm.output = append(sm.output, result)
}

func (sm *stateMachine) setReasoning(marker string) {
	switch marker {
	case "<think>", "[THINK]":
		sm.status = parsers.ChannelReasoning
	default:
		sm.status = parsers.ChannelAnswer
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

func nextMarker(content string, allowed func(string, int) bool) (int, string) {
	at := -1
	marker := ""
	for _, candidate := range streamMarkers {
		for start := 0; start < len(content); {
			index := strings.Index(content[start:], candidate)
			if index < 0 {
				break
			}
			index += start
			if allowed(candidate, index) && (at < 0 || index < at) {
				at = index
				marker = candidate
			}
			start = index + 1
		}
	}
	return at, marker
}

func partialMarkerSuffix(content string, allowed func(string, int) bool) int {
	for length := min(len(content), len(toolCallsMarker)-1); length > 0; length-- {
		suffix := content[len(content)-length:]
		for _, marker := range streamMarkers {
			if strings.HasPrefix(marker, suffix) && allowed(marker, len(content)-length) {
				return length
			}
		}
	}
	return 0
}

// Flush releases buffered output at EOS. Partial marker prefixes are tool
// evidence so the strict full-buffer parser, rather than answer handling,
// decides their validity.
func (sm *stateMachine) Flush() parsers.Result {
	if result := sm.popOutput(); result != (parsers.Result{}) {
		return result
	}
	if sm.toolCallBuf.Len() > 0 {
		content := sm.toolCallBuf.String()
		if sm.tokenAware {
			_, err := parseCallsWithMarkers(content, func(marker string, offset int) bool {
				return !sm.specialMarkers[marker] || sm.toolMarkers[offset]
			})
			if errors.Is(err, errTokenProvenance) {
				// The final parser has no token metadata. Carry failure across
				// that boundary without allowing textual reparsing to bless it.
				content = invalidTokenFrame + content
			}
			// Other syntax errors are reported by the final parser verbatim.
		}
		sm.toolCallBuf.Reset()
		sm.inToolCall = false
		return parsers.Result{Channel: parsers.ChannelTool, Content: content}
	}
	if sm.pending.Len() > 0 {
		content := sm.pending.String()
		sm.pending.Reset()
		if strings.HasPrefix(toolCallsMarker, content) {
			return parsers.Result{Channel: parsers.ChannelTool, Content: content}
		}
		return parsers.Result{Channel: sm.status, Content: content}
	}
	return parsers.Result{}
}
