package prepare

import "encoding/json"

// Document is structured content extracted from a source file.
type Document struct {
	Markdown       string
	StructuredJSON json.RawMessage
	ProcessingTime float64
}

// Message is one model-completion message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest contains controlled model-generation inputs.
type CompletionRequest struct {
	Model          string
	Messages       []Message
	ResponseSchema map[string]any
	MaxTokens      int
	Seed           uint32
}

// Completion contains generated content and its token measurements.
type Completion struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}
