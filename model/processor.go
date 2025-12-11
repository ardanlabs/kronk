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
	model      *Model
	status     int
	collecting bool
}

func newProcessor(m *Model) *processor {
	return &processor{
		model:  m,
		status: statusCompletion,
	}
}

func (p *processor) standard(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.batchResponse(lctx, batch, sampler, buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return response{}, token, io.EOF
		}

		return response{}, token, err
	}

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
			batch, content, err = p.standardToolCall(lctx, token, sampler, buf)
			if err != nil {
				return response{}, token, nil
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

func (p *processor) gpt(lctx llama.Context, batch llama.Batch, sampler llama.Sampler, buf []byte) (response, llama.Token, error) {
	content, token, err := p.model.batchResponse(lctx, batch, sampler, buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return response{}, token, io.EOF
		}

		return response{}, token, err
	}

	if p.collecting {
		if content == "<|end|>" || content == "<|call|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, token, nil
		}

		return response{status: p.status, content: content}, token, nil
	}

	switch content {
	case "<|message|>":
		p.collecting = true
		return response{}, token, nil

	case "analysis":
		p.status = statusReasoning
		return response{}, token, nil

	case "final":
		p.status = statusCompletion
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
	// {"name":"get_weather", "arguments":{"location":"NYC"})
	// {"name":"get_weather", "arguments":{"location":"NYC"})

	var toolCalls []ResponseToolCall

	for call := range strings.SplitSeq(content, "\n") {
		toolCall := ResponseToolCall{
			ID:  uuid.NewString(),
			Raw: call,
		}

		switch {
		case len(call) == 0:
			toolCall.Status = 1
			toolCall.Error = "response missing"

		default:
			if err := json.Unmarshal([]byte(call), &toolCall); err != nil {
				toolCall.Status = 2
				toolCall.Error = err.Error()
			}
		}

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}
