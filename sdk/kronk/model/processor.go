package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/hybridgroup/yzma/pkg/llama"
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
	toolCallBuf strings.Builder
	inToolCall  bool
}

func newProcessor(m *Model) *processor {
	return &processor{
		model:  m,
		status: statusCompletion,
	}
}

// standardFirst samples the first token after prefill without re-decoding.
// Use this for the first token after prefill when logits are already computed.
func (p *processor) standardFirst(lctx llama.Context, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.sampleToken(lctx, sampler, buf)
	if err != nil {
		return response{}, token, err
	}

	return p.standardProcess(lctx, content, token, sampler, buf)
}

func (p *processor) standard(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.batchResponse(lctx, batch, sampler, buf)
	if err != nil {
		return response{}, token, err
	}

	return p.standardProcess(lctx, content, token, sampler, buf)
}

// standardProcess handles token content for standard (non-GPT) models.
func (p *processor) standardProcess(lctx llama.Context, content string, token llama.Token, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	switch content {
	case "<think>":
		p.status = statusReasoning
		return response{}, token, nil

	case "</think>":
		p.status = statusCompletion
		return response{}, token, nil

	case "<tool_call>":
		p.status = statusTooling
		var w strings.Builder

		for {
			batch, content, err := p.standardToolCall(lctx, token, sampler, buf)
			if err != nil {
				return response{}, token, err
			}

			w.WriteString(content)

			_, token, err = p.model.batchResponse(lctx, batch, sampler, buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				return response{}, token, err
			}
		}

		return response{status: p.status, content: w.String()}, token, nil

	default:
		return response{status: p.status, content: content}, token, nil
	}
}

func (p *processor) standardToolCall(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (llama.Batch, string, error) {
	var batch llama.Batch
	var content string
	var err error
	var data strings.Builder

	for {
		batch = p.model.nextBatch(token)
		content, token, err = p.model.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			return batch, "", err
		}

		if content == "<tool_call>" {
			continue
		}

		if content == "</tool_call>" {
			break
		}

		data.WriteString(content)
	}

	content = strings.Trim(data.String(), "\n")
	content = fmt.Sprintf("%s\n", content)

	batch = p.model.nextBatch(token)

	return batch, content, nil
}

// =============================================================================

// gptFirst samples the first token after prefill without re-decoding.
// Use this for the first token after prefill when logits are already computed.
func (p *processor) gptFirst(lctx llama.Context, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.sampleToken(lctx, sampler, buf)
	if err != nil {
		return response{}, token, err
	}

	return p.gptProcess(content, token)
}

func (p *processor) gpt(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.batchResponse(lctx, batch, sampler, buf)
	if err != nil {
		return response{}, token, err
	}

	return p.gptProcess(content, token)
}

// gptProcess handles token content for GPT models.
// Template format:
//   - Reasoning: <|start|>assistant<|channel|>analysis<|message|>...content...<|end|>
//   - Final: <|start|>assistant<|channel|>final<|message|>...content...<|return|>
//   - Tool call: <|start|>assistant to=functions.name<|channel|>commentary json<|message|>...args...<|call|>
func (p *processor) gptProcess(content string, token llama.Token) (response, llama.Token, error) {
	if p.collecting {
		if content == "<|return|>" || content == "<|call|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, token, io.EOF
		}

		if content == "<|end|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, token, nil
		}

		return response{status: p.status, content: content}, token, nil
	}

	if p.awaitingChannel {
		p.awaitingChannel = false
		switch content {
		case "analysis":
			p.status = statusReasoning
		case "final":
			p.status = statusCompletion
		case "commentary":
			p.status = statusTooling
		}
		return response{}, token, nil
	}

	switch content {
	case "<|start|>":
		p.status = statusNone
		p.collecting = false
		p.awaitingChannel = false
		return response{}, token, nil

	case "<|channel|>":
		p.awaitingChannel = true
		return response{}, token, nil

	case "<|message|>":
		p.collecting = true
		return response{}, token, nil

	case "functions":
		p.collecting = true
		p.status = statusTooling
		return response{}, token, nil

	default:
		return response{}, token, nil
	}
}

// =============================================================================

func parseGPTToolCall(content string) []ResponseToolCall {
	// .get_weather <|constrain|>json<|message|>{"location":"NYC"}
	// .get_weather <|constrain|>json<|message|>{"location":"NYC"}

	var jsonCalls []string

	for call := range strings.SplitSeq(content, "\n") {
		if call == "" {
			continue
		}

		// Extract tool name (remove leading dot)
		parts := strings.SplitN(call, " ", 2)
		name := strings.TrimPrefix(parts[0], ".")

		// Extract arguments JSON after <|message|>
		var args string
		if idx := strings.Index(call, "<|message|>"); idx != -1 {
			args = call[idx+11:]
		}

		// Build JSON: {"name":"get_weather","arguments":{"location":"NYC"}}
		jsonCall := `{"name":"` + name + `","arguments":` + args + `}`
		jsonCalls = append(jsonCalls, jsonCall)
	}

	return parseToolCall(strings.Join(jsonCalls, "\n"))
}

func parseToolCall(content string) []ResponseToolCall {

	// {"name":"get_weather", "arguments":{"location":"NYC"}
	if strings.HasPrefix(content, "{\"name\"") {
		return parseJSONFormat(content)
	}

	// <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
	// <function=invoke_cli_command>\n<parameter=call>\ngo version\n</parameter>\n</function>
	if strings.HasPrefix(content, "<function=") {
		return parseFunctionFormat(content)
	}

	// get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
	// GLM-style format with <arg_key>/<arg_value> tags
	if strings.Contains(content, "<arg_key>") {
		return parseArgKeyValueFormat(content)
	}

	return nil
}

func parseFunctionFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	// Handle escaped newlines (literal \n) by converting to actual newlines
	content = strings.ReplaceAll(content, "\\n", "\n")

	for {
		funcStart := strings.Index(content, "<function=")
		if funcStart == -1 {
			break
		}

		funcEnd := strings.Index(content[funcStart:], ">")
		if funcEnd == -1 {
			break
		}

		name := content[funcStart+10 : funcStart+funcEnd]

		closeFunc := strings.Index(content, "</function>")
		if closeFunc == -1 {
			break
		}

		funcBody := content[funcStart+funcEnd+1 : closeFunc]
		args := make(map[string]any)

		remaining := funcBody
		for {
			paramStart := strings.Index(remaining, "<parameter=")
			if paramStart == -1 {
				break
			}

			paramNameEnd := strings.Index(remaining[paramStart:], ">")
			if paramNameEnd == -1 {
				break
			}

			paramName := remaining[paramStart+11 : paramStart+paramNameEnd]

			paramClose := strings.Index(remaining, "</parameter>")
			if paramClose == -1 {
				break
			}

			paramValue := strings.TrimSpace(remaining[paramStart+paramNameEnd+1 : paramClose])
			args[paramName] = paramValue

			remaining = remaining[paramClose+12:]
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   uuid.NewString(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
			Raw: content[funcStart : closeFunc+11],
		})

		content = content[closeFunc+11:]
	}

	return toolCalls
}

func parseJSONFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	for call := range strings.SplitSeq(content, "\n") {
		toolCall := ResponseToolCall{
			ID:   uuid.NewString(),
			Type: "function",
			Raw:  call,
		}

		switch {
		case len(call) == 0:
			toolCall.Status = 1
			toolCall.Error = "response missing"

		default:
			if err := json.Unmarshal([]byte(call), &toolCall.Function); err != nil {
				toolCall.Status = 2
				toolCall.Error = err.Error()
			}
		}

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}

// parseArgKeyValueFormat parses GLM-style tool calls with <arg_key>/<arg_value> tags.
// Format: get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
func parseArgKeyValueFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	for call := range strings.SplitSeq(content, "\n") {
		if call == "" {
			continue
		}

		// Find the function name (everything before the first <arg_key>)
		argKeyIdx := strings.Index(call, "<arg_key>")
		if argKeyIdx == -1 {
			continue
		}

		name := strings.TrimSpace(call[:argKeyIdx])
		args := make(map[string]any)

		// Parse all <arg_key>...</arg_key><arg_value>...</arg_value> pairs
		remaining := call[argKeyIdx:]
		for {
			keyStart := strings.Index(remaining, "<arg_key>")
			if keyStart == -1 {
				break
			}

			keyEnd := strings.Index(remaining, "</arg_key>")
			if keyEnd == -1 {
				break
			}

			key := remaining[keyStart+9 : keyEnd]

			valStart := strings.Index(remaining, "<arg_value>")
			if valStart == -1 {
				break
			}

			valEnd := strings.Index(remaining, "</arg_value>")
			if valEnd == -1 {
				break
			}

			value := remaining[valStart+11 : valEnd]
			args[key] = value

			remaining = remaining[valEnd+12:]
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   uuid.NewString(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
			Raw: call,
		})
	}

	return toolCalls
}

// =============================================================================
// Step methods for batch engine (no llama calls - pure state machine)
// =============================================================================

// stepStandard processes a single token for standard models without calling llama.
// This is used by the batch engine where decode/sample happens externally.
// Returns (response, endOfGeneration).
func (p *processor) stepStandard(content string) (response, bool) {
	// Handle tool call accumulation mode.
	if p.inToolCall {
		switch content {
		case "<tool_call>":
			// Nested or repeated tag, skip.
			return response{}, false

		case "</tool_call>":
			// End of one tool call block. Check if we have accumulated content.
			toolContent := strings.Trim(p.toolCallBuf.String(), "\n")
			if toolContent != "" {
				toolContent = fmt.Sprintf("%s\n", toolContent)
			}

			p.toolCallBuf.Reset()

			// Stay in tool call mode in case there are more tool calls.
			// The caller will handle EOG detection separately.
			return response{status: statusTooling, content: toolContent}, false

		default:
			// Accumulate tool call content.
			p.toolCallBuf.WriteString(content)
			return response{}, false
		}
	}

	// Normal token processing.
	switch content {
	case "<think>":
		p.status = statusReasoning
		return response{}, false

	case "</think>":
		p.status = statusCompletion
		return response{}, false

	case "<tool_call>":
		p.status = statusTooling
		p.inToolCall = true
		p.toolCallBuf.Reset()
		return response{}, false

	default:
		return response{status: p.status, content: content}, false
	}
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

		return response{status: p.status, content: content}, false
	}

	if p.awaitingChannel {
		p.awaitingChannel = false
		switch content {
		case "analysis":
			p.status = statusReasoning

		case "final":
			p.status = statusCompletion

		case "commentary":
			p.status = statusTooling
		}

		return response{}, false
	}

	switch content {
	case "<|start|>":
		p.status = statusNone
		p.collecting = false
		p.awaitingChannel = false
		return response{}, false

	case "<|channel|>":
		p.awaitingChannel = true
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

// resetState resets the processor state for reuse in a new slot.
func (p *processor) resetState() {
	p.status = statusCompletion
	p.collecting = false
	p.awaitingChannel = false
	p.toolCallBuf.Reset()
	p.inToolCall = false
}
