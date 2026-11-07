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

// spanInputMessages serializes the request messages for the
// gen_ai.input.messages span attribute.
func spanInputMessages(d D) string {
	msgs, exists := d["messages"]
	if !exists {
		return ""
	}

	data, err := json.Marshal(msgs)
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
