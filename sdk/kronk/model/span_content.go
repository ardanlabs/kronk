package model

import (
	"encoding/json"
	"strings"
)

// maxSpanMessageBytes caps the size of gen_ai.*.messages span attributes.
const maxSpanMessageBytes = 16 * 1024

func truncateSpanContent(s string) string {
	if len(s) <= maxSpanMessageBytes {
		return s
	}
	return s[:maxSpanMessageBytes] + "…(truncated)"
}

// spanInputPart is one content part of an input message in the
// gen_ai.input.messages schema.
type spanInputPart struct {
	Type      string `json:"type"`
	Content   any    `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments any    `json:"arguments,omitempty"`
	Response  any    `json:"response,omitempty"`
}

// spanInputMessage is one chat message in the gen_ai.input.messages schema.
type spanInputMessage struct {
	Role  string          `json:"role"`
	Parts []spanInputPart `json:"parts"`
}

// spanInputMessages serializes the request messages for the
// gen_ai.input.messages span attribute, converting the OpenAI-style
// request body to the semconv role/parts structure.
func spanInputMessages(d D) string {
	raw, exists := d["messages"]
	if !exists {
		return ""
	}

	msgs, ok := raw.([]any)
	if !ok {
		return ""
	}

	out := make([]spanInputMessage, 0, len(msgs))

	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}

		role, _ := msg["role"].(string)
		sm := spanInputMessage{Role: role, Parts: []spanInputPart{}}

		switch content := msg["content"].(type) {
		case string:
			if content != "" {
				if role == "tool" {
					id, _ := msg["tool_call_id"].(string)
					sm.Parts = append(sm.Parts, spanInputPart{Type: "tool_call_response", ID: id, Response: content})
				} else {
					sm.Parts = append(sm.Parts, spanInputPart{Type: "text", Content: content})
				}
			}

		case []any:
			for _, p := range content {
				part, ok := p.(map[string]any)
				if !ok {
					continue
				}
				typ, _ := part["type"].(string)
				if typ == "text" {
					sm.Parts = append(sm.Parts, spanInputPart{Type: "text", Content: part["text"]})
					continue
				}
				sm.Parts = append(sm.Parts, spanInputPart{Type: typ})
			}
		}

		if toolCalls, ok := msg["tool_calls"].([]any); ok {
			for _, t := range toolCalls {
				tc, ok := t.(map[string]any)
				if !ok {
					continue
				}
				id, _ := tc["id"].(string)
				var name string
				var args any
				if fn, ok := tc["function"].(map[string]any); ok {
					name, _ = fn["name"].(string)
					args = fn["arguments"]
				}
				sm.Parts = append(sm.Parts, spanInputPart{Type: "tool_call", ID: id, Name: name, Arguments: args})
			}
		}

		out = append(out, sm)
	}

	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}

	return truncateSpanContent(string(data))
}

type spanOutputPart struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments any    `json:"arguments,omitempty"`
}

type spanOutputMessage struct {
	Role         string           `json:"role"`
	Parts        []spanOutputPart `json:"parts"`
	FinishReason string           `json:"finish_reason,omitempty"`
}

// spanOutputMessages serializes the assistant output for the
// gen_ai.output.messages span attribute.
func spanOutputMessages(finishReason string, reasoning *strings.Builder, content *strings.Builder, toolCalls []ResponseToolCall) string {
	parts := make([]spanOutputPart, 0, 2+len(toolCalls))

	if reasoning.Len() > 0 {
		parts = append(parts, spanOutputPart{Type: "reasoning", Content: reasoning.String()})
	}

	if content.Len() > 0 {
		parts = append(parts, spanOutputPart{Type: "text", Content: content.String()})
	}

	for _, tc := range toolCalls {
		parts = append(parts, spanOutputPart{
			Type:      "tool_call",
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	out := []spanOutputMessage{{
		Role:         "assistant",
		Parts:        parts,
		FinishReason: finishReason,
	}}

	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}

	return truncateSpanContent(string(data))
}
