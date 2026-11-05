// Package providers supplies reusable document conversion, model completion,
// and model training clients for jobs.
package providers

import "errors"

// ErrCompletionTruncated identifies completions that reach their token limit.
var ErrCompletionTruncated = errors.New("model completion reached its token limit")

// Message is one model conversation message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest requests model output. A nil ResponseSchema leaves output
// unconstrained by a grammar.
type CompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseSchema map[string]any `json:"response_schema"`
	MaxTokens      int            `json:"max_tokens"`
	Seed           uint32         `json:"seed"`
	EnableThinking bool           `json:"enable_thinking,omitzero"`
}

// Completion contains generated content and model usage.
type Completion struct {
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}
