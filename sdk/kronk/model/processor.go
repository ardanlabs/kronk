package model

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	statusNone       = 0
	statusReasoning  = 1
	statusCompletion = 2
	statusTooling    = 3
)

type response struct {
	status  int
	content string
}

type processor struct {
	model           *Model
	status          int
	collecting      bool
	awaitingChannel bool

	// For accumulating tool call content across tokens (batch engine use).
	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool // Set after </tool_call> or <tool_call|>; next non-tool-call token triggers EOG.

	// For GPT models: accumulate channel name tokens and handle <|constrain|>.
	channelBuf        strings.Builder
	awaitingConstrain bool
	toolFuncName      string // Function name extracted from "to=NAME" in channel

	// For detecting split tags like "<function=" across multiple tokens.
	// Some models (Qwen3-Coder variants) emit <function=...> directly without
	// the <tool_call> wrapper, and the tag may be tokenized as "<", "function", "=".
	pendingTagBuf strings.Builder
	inPendingTag  bool
}

func newProcessor(m *Model) *processor {
	return &processor{
		model:  m,
		status: statusCompletion,
	}
}

func newToolCallID() string {
	return "call_" + uuid.NewString()
}

// =============================================================================
// Step methods for batch engine (no llama calls - pure state machine)
// =============================================================================

// stepStandard processes a single token for standard models without calling llama.
// This is used by the batch engine where decode/sample happens externally.
// Returns (response, endOfGeneration).
func (p *processor) stepStandard(content string) (response, bool) {
	// Handle pending tag accumulation for detecting split tags like "<function=".
	if p.inPendingTag {
		p.pendingTagBuf.WriteString(content)
		accumulated := p.pendingTagBuf.String()

		// Check if we've accumulated enough to detect <function=.
		if strings.HasPrefix(accumulated, "<function=") {
			// Found the pattern. Enter tool call mode and start accumulating.
			p.inPendingTag = false
			p.pendingTagBuf.Reset()
			p.status = statusTooling
			p.inToolCall = true
			p.toolCallBuf.Reset()
			p.toolCallBuf.WriteString(accumulated)
			return response{}, false
		}

		// Check if it's definitely not going to be <function=.
		if !strings.HasPrefix("<function=", accumulated) {
			// Flush accumulated content as normal output.
			p.inPendingTag = false
			p.pendingTagBuf.Reset()
			return response{status: p.status, content: accumulated}, false
		}

		// Still a prefix match, continue accumulating.
		return response{}, false
	}

	// Handle tool call accumulation mode.
	if p.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			// Nested or repeated tag, skip.
			return response{}, false

		case "</tool_call>", "<tool_call|>":
			// End of one tool call block. Check if we have accumulated content.
			toolContent := strings.Trim(p.toolCallBuf.String(), "\n")
			if toolContent != "" {
				toolContent = fmt.Sprintf("%s\n", toolContent)
			}

			p.toolCallBuf.Reset()
			p.inToolCall = false
			p.toolCallDone = true

			// Stay in tool call mode in case there are more tool calls.
			// The next token will be checked by the toolCallDone guard:
			// another <|tool_call> continues, anything else triggers EOG.
			return response{status: statusTooling, content: toolContent}, false

		case "[TOOL_CALLS]":
			// Another tool call starting - flush buffer and start new accumulation.
			p.toolCallBuf.Reset()
			p.toolCallBuf.WriteString("[TOOL_CALLS]")
			return response{}, false

		default:
			// Check if we're accumulating Mistral format (no closing tag).
			buf := p.toolCallBuf.String()
			if strings.HasPrefix(buf, "[TOOL_CALLS]") {
				// Mistral format: accumulate and stream to finalTooling.
				p.toolCallBuf.WriteString(content)
				return response{status: statusTooling, content: content}, false
			}

			// Standard format: accumulate in buffer only.
			p.toolCallBuf.WriteString(content)

			// Check if we've completed a function call (models that skip </tool_call>).
			accumulated := p.toolCallBuf.String()
			if strings.HasSuffix(strings.TrimSpace(accumulated), "</function>") {
				toolContent := strings.Trim(accumulated, "\n")
				if toolContent != "" {
					toolContent = fmt.Sprintf("%s\n", toolContent)
				}

				p.toolCallBuf.Reset()
				p.inToolCall = false
				p.toolCallDone = true

				return response{status: statusTooling, content: toolContent}, false
			}

			return response{}, false
		}
	}

	// After a tool call closes, only allow another tool call opener.
	// Anything else (reasoning, text, etc.) means the model is done.
	if p.toolCallDone {
		switch content {
		case "<tool_call>", "<|tool_call>":
			p.toolCallDone = false
			p.inToolCall = true
			p.toolCallBuf.Reset()
			return response{}, false
		default:
			p.toolCallDone = false
			return response{}, true // EOG — stop generation after tool call(s).
		}
	}

	// Handle Gemma4 channel: swallow the channel name token (e.g. "thought")
	// that follows <|channel>, then stream content as reasoning until <channel|>.
	if p.awaitingChannel {
		p.awaitingChannel = false
		p.status = statusReasoning
		return response{}, false
	}

	// Normal token processing.
	switch content {
	case "<think>":
		p.status = statusReasoning
		return response{}, false

	case "</think>", "<channel|>":
		p.status = statusCompletion
		return response{}, false

	case "<|channel>":
		p.awaitingChannel = true
		return response{}, false

	case "<tool_call>", "<|tool_call>":
		p.status = statusTooling
		p.inToolCall = true
		p.toolCallBuf.Reset()
		return response{}, false

	case "<tool_call|>", "<|tool_response>", "<tool_response|>":
		// Gemma4 structural markers outside of tool call accumulation; skip.
		return response{}, false

	case "[TOOL_CALLS]":
		// Mistral/Devstral format: [TOOL_CALLS]name[ARGS]{...}
		// Stream the marker to finalTooling for parsing at EOG.
		p.status = statusTooling
		p.inToolCall = true
		p.toolCallBuf.Reset()
		p.toolCallBuf.WriteString("[TOOL_CALLS]")
		return response{status: statusTooling, content: "[TOOL_CALLS]"}, false

	default:
		// Check for start of <function= pattern (may be split across tokens).
		if content == "<" || strings.HasPrefix(content, "<f") || strings.HasPrefix(content, "<function") {
			if strings.HasPrefix(content, "<function=") {
				// Complete tag in one token, enter tool call mode directly.
				p.status = statusTooling
				p.inToolCall = true
				p.toolCallBuf.Reset()
				p.toolCallBuf.WriteString(content)
				return response{}, false
			}

			// Could be start of <function=, start accumulating.
			if strings.HasPrefix("<function=", content) {
				p.inPendingTag = true
				p.pendingTagBuf.Reset()
				p.pendingTagBuf.WriteString(content)
				return response{}, false
			}
		}

		return response{status: p.status, content: content}, false
	}
}

// resetState resets the processor state for reuse in a new slot.
func (p *processor) resetState() {
	p.status = statusCompletion
	p.collecting = false
	p.awaitingChannel = false
	p.toolCallBuf.Reset()
	p.inToolCall = false
	p.toolCallDone = false
	p.channelBuf.Reset()
	p.awaitingConstrain = false
	p.toolFuncName = ""
	p.inPendingTag = false
	p.pendingTagBuf.Reset()
}
