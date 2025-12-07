package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func (m *Model) gptChannel(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (llama.Batch, string, error) {
	// <|channel|>analysis<|message|>REASONING<|end|><|start|>assistant<|channel|>final<|message|>RESPONSE
	// <|channel|>analysis<|message|>REASONING<|end|><|start|>assistant<|channel|>commentary to=functions.get_weather <|constrain|>json<|message|>{"location":"NYC"}

	var batch llama.Batch
	var content string
	var err error
	var data strings.Builder

	// Collect the content up to the location of <|message|>.
	for {
		batch = m.nextBatch(token)
		content, token, err = m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			return batch, "<|error|>", err
		}

		if content == "<|message|>" {
			batch = m.nextBatch(token)
			break
		}

		data.WriteString(content)
	}

	msg := data.String()

	switch {
	case msg == "analysis":
		return batch, "<|reasoning|>", nil

	case msg == "final":
		return batch, "<|completion|>", nil

	case len(msg) > 10 && msg[:10] == "commentary":
		toolCall, err := m.gptToolCall(msg, batch, lctx, sampler, buf)
		if err != nil {
			return llama.Batch{}, fmt.Sprintf("<|tool_call|>%s<|error|>%s", toolCall, err), nil
		}

		return llama.Batch{}, fmt.Sprintf("<|tool_call|>%s", toolCall), nil

	default:
		batch = m.nextBatch(token)
		return batch, "<|error|>", fmt.Errorf("unknown channel type: %s", msg)
	}
}

func (m *Model) gptToolCall(msg string, batch llama.Batch, lctx llama.Context, sampler llama.Sampler, buf []byte) (string, error) {
	var args strings.Builder

	// Collect the remaining tokens until the end of the message.
	for {
		v, token, err := m.batchResponse(lctx, batch, sampler, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}

		args.WriteString(v)

		batch = m.nextBatch(token)
	}

	// msg : commentary to=functions.get_weather <|constrain|>json
	// args: {"location":"NYC"}

	// We will return the raw data back to the user if there is failure
	// so we can debug the issue.
	raw := fmt.Sprintf("%s<|message|>%s", msg, args.String())

	arguments := make(map[string]any)
	err := json.Unmarshal([]byte(args.String()), &arguments)
	if err != nil {
		return raw, fmt.Errorf("gpt-tool-call:failed to unmarshal tool call arguments: %w", err)
	}

	rtc := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}{
		Name:      extractFunctionName(msg),
		Arguments: arguments,
	}

	data, err := json.Marshal(rtc)
	if err != nil {
		return raw, fmt.Errorf("gpt-tool-call:failed to marshal tool call: %w", err)
	}

	return string(data), nil
}

func (m *Model) gptEnd(lctx llama.Context, token llama.Token, sampler llama.Sampler, buf []byte) (llama.Batch, error) {
	batch := m.nextBatch(token)

	_, token, err := m.batchResponse(lctx, batch, sampler, buf) // <|start|>
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)

	_, token, err = m.batchResponse(lctx, batch, sampler, buf) // assistant
	if err != nil {
		return batch, err
	}

	batch = m.nextBatch(token)
	return batch, nil
}

// =============================================================================

func extractFunctionName(s string) string {
	for field := range strings.FieldsSeq(s) {
		if _, after, ok := strings.Cut(field, "="); ok {
			split := strings.Split(after, ".")
			if len(split) != 2 {
				return ""
			}

			switch split[0] {
			case "functions":
				return split[1]
			}

			return ""
		}
	}

	return ""
}
