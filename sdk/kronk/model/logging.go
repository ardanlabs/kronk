package model

import (
	"fmt"
	"strings"
)

// StreamingResponseLogger captures the final streaming response for logging.
// It must capture data before forwarding since the caller may mutate the response.
type StreamingResponseLogger struct {
	finishReason string
	content      string
	reasoning    string
	toolCalls    []ResponseToolCall
}

// Capture captures data from a streaming response. Call this for each response
// before forwarding it. It only captures from the final response (when FinishReason is set).
func (l *StreamingResponseLogger) Capture(resp ChatResponse) {
	if len(resp.Choice) == 0 {
		return
	}

	fr := resp.Choice[0].FinishReason()
	if fr == "" {
		return
	}

	l.finishReason = fr
	if msg := resp.Choice[0].Message; msg != nil {
		l.content = msg.Content
		l.reasoning = msg.Reasoning
		l.toolCalls = append([]ResponseToolCall(nil), msg.ToolCalls...)
	}
}

// String returns a formatted string for logging.
func (l *StreamingResponseLogger) String() string {
	var b strings.Builder
	b.WriteString("\n")

	fmt.Fprintf(&b, "FinishReason: %s\n", l.finishReason)
	fmt.Fprintf(&b, "Role: assistant\n")

	if l.content != "" {
		fmt.Fprintf(&b, "Content (400): %.400s\n", l.content)
	}

	if l.reasoning != "" {
		fmt.Fprintf(&b, "Reasoning (400): %.400s\n", l.reasoning)
	}

	if len(l.toolCalls) > 0 {
		fmt.Fprintf(&b, "ToolCalls len=%d\n", len(l.toolCalls))
		for j, tc := range l.toolCalls {
			fmt.Fprintf(&b, "  tc[%d]: id=%s fn=%s args=%s\n", j, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}
	}

	return b.String()
}
