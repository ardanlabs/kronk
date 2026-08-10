package gemma

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for Gemma.
//
// Recognized markers:
//
//   - <|channel> NAME … <channel|>  reasoning wrap (NAME token is swallowed)
//   - <tool_call> … </tool_call>    tool-call envelope (also <|tool_call>/<tool_call|>)
//   - <|tool_response>, <tool_response|> structural skips
type stateMachine struct {
	status model.Channel

	// Tool-call accumulation across tokens.
	toolCallBuf    strings.Builder
	inToolCall     bool
	toolCallDone   bool
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
	detectedCalls  int

	// Swallow the channel name token (e.g. "thought") that follows <|channel>.
	awaitingChannel bool
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
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
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	// Inside a tool-call envelope: accumulate until close.
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			return model.Result{}, false

		case "</tool_call>", "<tool_call|>":
			if !gemmaWrapperCloseAllowed(sm.toolCallBuf.String()) {
				sm.toolCallBuf.WriteString(content)
				sm.updateToolCallDeltas()
				return model.Result{}, false
			}
			result := sm.flushToolCall(true)
			sm.toolCallDone = true
			return result, false

		default:
			sm.toolCallBuf.WriteString(content)
			sm.updateToolCallDeltas()
			return model.Result{}, false
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
			return model.Result{Channel: model.ChannelTool}, false
		default:
			if content == "" {
				return model.Result{}, false
			}
			// Gemma's final parser validates the complete accumulated grammar.
			// Preserve unexpected continuations so malformed delimiter text
			// cannot disappear at a token boundary and expose partial calls.
			sm.toolCallDone = false
			return model.Result{Channel: model.ChannelTool, Content: content}, false
		}
	}

	// Swallow the channel name token after <|channel>, then stream content
	// as reasoning until <channel|>.
	if sm.awaitingChannel {
		sm.awaitingChannel = false
		sm.status = model.ChannelReasoning
		return model.Result{}, false
	}

	switch content {
	case "<|channel>":
		sm.awaitingChannel = true
		return model.Result{}, false

	case "<channel|>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case "<tool_call>", "<|tool_call>":
		sm.status = model.ChannelTool
		sm.inToolCall = true
		sm.toolCallBuf.Reset()
		sm.detectedCalls = 0
		return model.Result{Channel: model.ChannelTool}, false

	case "<tool_call|>", "<|tool_response>", "<tool_response|>":
		// Structural markers outside tool-call accumulation; skip silently.
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}

// ToolCallDeltas drains OpenAI-compatible tool-call activity deltas.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns all tool-call identities emitted for this request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta { return sm.startedCalls }

func (sm *stateMachine) updateToolCallDeltas() {
	names := gemmaToolCallNames(sm.toolCallBuf.String())
	for _, name := range names[sm.detectedCalls:] {
		delta := model.ResponseToolCallDelta{ID: newToolCallID(), Index: len(sm.startedCalls), Type: "function", Function: model.ResponseToolCallDeltaFunction{Name: name}}
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
func (sm *stateMachine) Flush() model.Result {
	if !sm.inToolCall {
		return model.Result{}
	}

	return sm.flushToolCall(false)
}

func (sm *stateMachine) flushToolCall(closed bool) model.Result {
	content := strings.Trim(sm.toolCallBuf.String(), "\n")
	sm.toolCallBuf.Reset()
	sm.inToolCall = false
	if content == "" {
		return model.Result{}
	}

	content += "\n"
	content = encodeGemmaWrapperFrame(content, closed)

	return model.Result{Channel: model.ChannelTool, Content: content}
}

func gemmaWrapperCloseAllowed(content string) bool {
	gemmaQuoted := false
	standardQuoted := false
	escaped := false
	for cursor := 0; cursor < len(content); {
		if !standardQuoted && strings.HasPrefix(content[cursor:], `<|"|>`) {
			gemmaQuoted = !gemmaQuoted
			cursor += len(`<|"|>`)
			continue
		}
		if gemmaQuoted {
			cursor++
			continue
		}
		if escaped {
			escaped = false
			cursor++
			continue
		}
		if standardQuoted && content[cursor] == '\\' {
			escaped = true
			cursor++
			continue
		}
		if content[cursor] == '"' {
			standardQuoted = !standardQuoted
		}
		cursor++
	}
	return !gemmaQuoted && !standardQuoted
}
