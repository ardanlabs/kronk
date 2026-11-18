package gemma

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

// stateMachine is a per-slot streaming state machine for Gemma.
//
// Recognized markers:
//
//   - <|channel> NAME … <channel|>  reasoning wrap (NAME token is swallowed)
//   - <tool_call> … </tool_call>    tool-call envelope (also <|tool_call>/<tool_call|>)
//   - <|tool_response>, <tool_response|> structural skips
type stateMachine struct {
	status parsers.Channel

	// Tool-call accumulation across tokens.
	toolCallBuf    strings.Builder
	inToolCall     bool
	toolCallDone   bool
	toolCallDeltas []parsers.ResponseToolCallDelta
	startedCalls   []parsers.ResponseToolCallDelta
	detectedCalls  int

	// Swallow the channel name token (e.g. "thought") that follows <|channel>.
	awaitingChannel bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = parsers.ChannelAnswer
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	sm.toolCallDone = false
	sm.awaitingChannel = false
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
	sm.detectedCalls = 0
}

// Classify classifies a single decoded token's content.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset must be invoked between requests.
func (sm *stateMachine) Classify(content string) (parsers.Result, bool) {
	// Inside a tool-call envelope: accumulate until close.
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			return parsers.Result{}, false

		case "</tool_call>", "<tool_call|>":
			result := sm.flushToolCall()
			sm.toolCallDone = true
			return result, false

		default:
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			return parsers.Result{}, false
		}
	}

	// After a tool call closes, only another opener avoids EOG.
	if sm.toolCallDone {
		content = strings.TrimLeft(content, " \t\r\n")
		switch content {
		case "<tool_call>", "<|tool_call>":
			sm.toolCallDone = false
			sm.inToolCall = true
			sm.toolCallBuf.Reset()
			sm.detectedCalls = 0
			return parsers.Result{}, false
		default:
			if content == "" {
				return parsers.Result{}, false
			}
			// Gemma's final parser validates the complete accumulated grammar.
			// Preserve unexpected continuations so malformed delimiter text
			// cannot disappear at a token boundary and expose partial calls.
			sm.toolCallDone = false
			return parsers.Result{Channel: parsers.ChannelTool, Content: content}, false
		}
	}

	// Swallow the channel name token after <|channel>, then stream content
	// as reasoning until <channel|>.
	if sm.awaitingChannel {
		sm.awaitingChannel = false
		sm.status = parsers.ChannelReasoning
		return parsers.Result{}, false
	}

	switch content {
	case "<|channel>":
		sm.awaitingChannel = true
		return parsers.Result{}, false

	case "<channel|>":
		sm.status = parsers.ChannelAnswer
		return parsers.Result{}, false

	case "<tool_call>", "<|tool_call>":
		sm.status = parsers.ChannelTool
		sm.inToolCall = true
		sm.toolCallBuf.Reset()
		sm.detectedCalls = 0
		return parsers.Result{}, false

	case "<tool_call|>", "<|tool_response>", "<tool_response|>":
		// Structural markers outside tool-call accumulation; skip silently.
		return parsers.Result{}, false

	default:
		return parsers.Result{Channel: sm.status, Content: content}, false
	}
}

// ToolCallDeltas drains OpenAI-compatible tool-call activity deltas.
func (sm *stateMachine) ToolCallDeltas() []parsers.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities emitted for this request.
func (sm *stateMachine) StartedToolCalls() []parsers.ResponseToolCallDelta { return sm.startedCalls }

func (sm *stateMachine) updateToolCallDeltas() {
	names := gemmaToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		delta := parsers.ResponseToolCallDelta{ID: newToolCallID(), Index: len(sm.startedCalls), Type: "function", Function: parsers.ResponseToolCallDeltaFunction{Name: name}}
		sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
		sm.startedCalls = append(sm.startedCalls, delta)
	}
	sm.detectedCalls = len(names)
}

func gemmaToolCallNames(content string) []string {
	var names []string
	for {
		start := strings.Index(content, "call:")
		if start == -1 {
			break
		}
		content = content[start+len("call:"):]
		end := strings.IndexByte(content, '{')
		if end == -1 {
			break
		}
		if name := strings.TrimSpace(content[:end]); name != "" {
			names = append(names, name)
		}
		content = content[end:]
		braceEnd := findGemmaBraceEnd(content)
		if braceEnd == -1 {
			break
		}
		content = content[braceEnd+1:]
	}
	return names
}

// Flush drains tool-call content held while waiting for a closing marker.
func (sm *stateMachine) Flush() parsers.Result {
	if !sm.inToolCall {
		return parsers.Result{}
	}

	return sm.flushToolCall()
}

func (sm *stateMachine) flushToolCall() parsers.Result {
	content := strings.Trim(sm.toolCallBuf.String(), "\n")
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	if content == "" {
		return parsers.Result{}
	}

	return parsers.Result{Channel: parsers.ChannelTool, Content: content + "\n"}
}
