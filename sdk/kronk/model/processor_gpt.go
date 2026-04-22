package model

import (
	"fmt"
	"strings"
)

// parseGPTToolCall parses GPT-model tool calls.
// Format: .FUNC_NAME <|message|>JSON_ARGS
// The JSON may span multiple lines, so we can't split by newlines.
// Instead, find each ".NAME <|message|>" prefix and extract the JSON that follows.
func (p *processor) parseGPTToolCall(content string) []ResponseToolCall {
	var jsonCalls []string
	remaining := content

	for {
		// Find the start of a tool call (leading dot).
		dotIdx := strings.Index(remaining, ".")
		if dotIdx == -1 {
			break
		}

		remaining = remaining[dotIdx:]

		// Find <|message|> marker.
		msgIdx := strings.Index(remaining, "<|message|>")
		if msgIdx == -1 {
			break
		}

		// Extract function name (between dot and space before <|message|>).
		prefix := remaining[:msgIdx]
		parts := strings.SplitN(prefix, " ", 2)
		name := strings.TrimPrefix(parts[0], ".")

		// Move past <|message|> to get the JSON.
		jsonStart := msgIdx + 11
		remaining = remaining[jsonStart:]

		// Find the end of the JSON object by matching braces.
		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			// No valid JSON found, take the rest.
			jsonEnd = len(remaining)
		}

		args := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		// Build JSON: {"name":"get_weather","arguments":{"location":"NYC"}}
		jsonCall := `{"name":"` + name + `","arguments":` + args + `}`
		jsonCalls = append(jsonCalls, jsonCall)
	}

	return p.parseToolCall(strings.Join(jsonCalls, "\n"))
}

// stepGPT processes a single token for GPT models without calling llama.
// This is used by the batch engine where decode/sample happens externally.
// Returns (response, endOfGeneration).
func (p *processor) stepGPT(content string) (response, bool) {
	if p.collecting {
		if content == "<|return|>" || content == "<|call|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, true // End of generation
		}

		if content == "<|end|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, false
		}

		// Handle non-deterministic models that emit <|start|> or <|channel|>
		// without first closing the current block with <|end|>.
		if content == "<|start|>" {
			p.collecting = false
			p.status = statusNone
			p.awaitingChannel = false
			p.awaitingConstrain = false
			p.channelBuf.Reset()
			return response{}, false
		}

		if content == "<|channel|>" {
			p.collecting = false
			p.awaitingChannel = true
			p.channelBuf.Reset()
			return response{}, false
		}

		return response{status: p.status, content: content}, false
	}

	// Skip tokens between <|constrain|> and <|message|> (e.g., "json").
	if p.awaitingConstrain {
		if content == "<|message|>" {
			p.awaitingConstrain = false
			p.collecting = true

			// Emit the function name prefix for tool calls so parseGPTToolCall can parse it.
			// Format: ".FUNC_NAME <|message|>" which parseGPTToolCall expects.
			if p.status == statusTooling && p.toolFuncName != "" {
				prefix := fmt.Sprintf(".%s <|message|>", p.toolFuncName)
				p.toolFuncName = ""
				return response{status: p.status, content: prefix}, false
			}
		}
		return response{}, false
	}

	// Accumulate channel name tokens until <|message|> or <|constrain|>.
	if p.awaitingChannel {
		if content == "<|message|>" || content == "<|constrain|>" {
			p.awaitingChannel = false
			channelName := strings.TrimSpace(p.channelBuf.String())
			p.channelBuf.Reset()

			// Determine status from channel name prefix.
			switch {
			case strings.HasPrefix(channelName, "analysis"):
				p.status = statusReasoning

			case strings.HasPrefix(channelName, "final"):
				p.status = statusCompletion

			case strings.HasPrefix(channelName, "commentary"):
				p.status = statusTooling

				// Extract function name from "commentary to=functions.FUNC_NAME".
				if _, after, ok := strings.Cut(channelName, " to="); ok {
					funcName := strings.TrimSpace(after)
					p.toolFuncName = strings.TrimPrefix(funcName, "functions.")
				}
			}

			switch content == "<|constrain|>" {
			case true:
				p.awaitingConstrain = true
			case false:
				p.collecting = true
			}

			return response{}, false
		}

		p.channelBuf.WriteString(content)

		return response{}, false
	}

	switch content {
	case "<|start|>":
		p.status = statusNone
		p.collecting = false
		p.awaitingChannel = false
		p.awaitingConstrain = false
		p.channelBuf.Reset()
		return response{}, false

	case "<|channel|>":
		p.awaitingChannel = true
		p.channelBuf.Reset()
		return response{}, false

	case "<|message|>":
		p.collecting = true
		return response{}, false

	case "functions":
		p.collecting = true
		p.status = statusTooling
		return response{}, false

	default:
		return response{}, false
	}
}
