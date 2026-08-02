package gpt

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// stateMachine is a per-slot streaming state machine for the GPT-OSS Harmony
// format. Direct port of model.stateMachine.stepGPT preserved as faithfully as
// possible.
//
// Behavior is undefined if Classify is called after a previous call returned
// eog=true. Reset returns the state machine to its initial configuration.
type stateMachine struct {
	status            model.Channel
	collecting        bool
	awaitingChannel   bool
	awaitingConstrain bool

	// Channel name accumulator (e.g. "analysis", "final", "commentary to=...").
	channelBuf strings.Builder

	// Function name extracted from "commentary to=functions.NAME" channel.
	toolFuncName string

	toolCallBuf    strings.Builder
	toolCallDeltas []model.ResponseToolCallDelta
	startedCalls   []model.ResponseToolCallDelta
}

// Reset returns the stateMachine to its initial state for reuse on a new
// request. Mirrors stateMachine.resetState() for the fields stepGPT touches.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelNone
	sm.collecting = false
	sm.awaitingChannel = false
	sm.awaitingConstrain = false
	sm.channelBuf.Reset()
	sm.toolFuncName = ""
	sm.toolCallBuf.Reset()
	sm.toolCallDeltas = nil
	sm.startedCalls = nil
}

// Classify classifies a single decoded token's content. Direct port of stepGPT from
// parser_gpt.go; comments and branch ordering are preserved so the diff
// against the original is mechanical.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.collecting {
		if content == "<|return|>" || content == "<|call|>" {
			sm.collecting = false
			sm.status = model.ChannelNone
			return model.Result{}, true // End of generation; Flush releases a buffered tool call.
		}

		if content == "<|end|>" {
			sm.collecting = false
			sm.status = model.ChannelNone
			return sm.releaseToolCall(), false
		}

		// Handle non-deterministic models that emit <|start|> or <|channel|>
		// without first closing the current block with <|end|>.
		if content == "<|start|>" {
			sm.collecting = false
			sm.status = model.ChannelNone
			sm.awaitingChannel = false
			sm.awaitingConstrain = false
			sm.channelBuf.Reset()
			return model.Result{}, false
		}

		if content == "<|channel|>" {
			sm.collecting = false
			sm.awaitingChannel = true
			sm.channelBuf.Reset()
			return model.Result{}, false
		}

		if sm.status == model.ChannelTool {
			sm.toolCallBuf.WriteString(content)
			return model.Result{}, false
		}

		return model.Result{Channel: sm.status, Content: content}, false
	}

	// Skip tokens between <|constrain|> and <|message|> (e.g., "json").
	if sm.awaitingConstrain {
		if content == "<|message|>" {
			sm.awaitingConstrain = false
			sm.collecting = true

			if sm.status == model.ChannelTool && sm.toolFuncName != "" {
				sm.toolCallBuf.WriteString("." + sm.toolFuncName + " <|message|>")
				sm.toolFuncName = ""
			}
		}
		return model.Result{}, false
	}

	// Accumulate channel name tokens until <|message|> or <|constrain|>.
	if sm.awaitingChannel {
		if content == "<|message|>" || content == "<|constrain|>" {
			sm.awaitingChannel = false
			channelName := strings.TrimSpace(sm.channelBuf.String())
			sm.channelBuf.Reset()

			// Determine status from channel name prefix.
			switch {
			case strings.HasPrefix(channelName, "analysis"):
				sm.status = model.ChannelReasoning

			case strings.HasPrefix(channelName, "final"):
				sm.status = model.ChannelAnswer

			case strings.HasPrefix(channelName, "commentary"):
				sm.status = model.ChannelTool

				// Extract function name from "commentary to=functions.FUNC_NAME".
				if _, after, ok := strings.Cut(channelName, " to="); ok {
					funcName := strings.TrimSpace(after)
					sm.toolFuncName = strings.TrimPrefix(funcName, "functions.")
					if sm.toolFuncName != "" {
						sm.startToolCall(sm.toolFuncName)
					}
				}
			}

			switch content == "<|constrain|>" {
			case true:
				sm.awaitingConstrain = true
			case false:
				sm.collecting = true
				if sm.status == model.ChannelTool && sm.toolFuncName != "" {
					sm.toolCallBuf.WriteString("." + sm.toolFuncName + " <|message|>")
					sm.toolFuncName = ""
				}
			}

			return model.Result{}, false
		}

		sm.channelBuf.WriteString(content)

		return model.Result{}, false
	}

	switch content {
	case "<|start|>":
		sm.status = model.ChannelNone
		sm.collecting = false
		sm.awaitingChannel = false
		sm.awaitingConstrain = false
		sm.channelBuf.Reset()
		return model.Result{}, false

	case "<|channel|>":
		sm.awaitingChannel = true
		sm.channelBuf.Reset()
		return model.Result{}, false

	case "<|message|>":
		sm.collecting = true
		return model.Result{}, false

	case "functions":
		sm.collecting = true
		sm.status = model.ChannelTool
		return model.Result{}, false

	default:
		return model.Result{}, false
	}
}

func (sm *stateMachine) startToolCall(name string) {
	delta := model.ResponseToolCallDelta{
		ID:    newToolCallID(),
		Index: len(sm.startedCalls),
		Type:  "function",
		Function: model.ResponseToolCallDeltaFunction{
			Name: name,
		},
	}
	sm.toolCallDeltas = append(sm.toolCallDeltas, delta)
	sm.startedCalls = append(sm.startedCalls, delta)
}

func (sm *stateMachine) releaseToolCall() model.Result {
	if sm.toolCallBuf.Len() == 0 {
		return model.Result{}
	}

	content := sm.toolCallBuf.String()
	sm.toolCallBuf.Reset()
	return model.Result{Channel: model.ChannelTool, Content: content}
}

// Flush releases an incomplete buffered native tool call.
func (sm *stateMachine) Flush() model.Result {
	return sm.releaseToolCall()
}

// ToolCallDeltas drains tool-call identity deltas produced by Classify.
func (sm *stateMachine) ToolCallDeltas() []model.ResponseToolCallDelta {
	deltas := sm.toolCallDeltas
	sm.toolCallDeltas = nil
	return deltas
}

// StartedToolCalls returns identities emitted during the current request.
func (sm *stateMachine) StartedToolCalls() []model.ResponseToolCallDelta {
	return sm.startedCalls
}
