package prepare

import (
	"encoding/json"
	"errors"
)

// ErrCompletionTruncated reports that the model reached its output-token limit
// before completing the requested structured response.
var ErrCompletionTruncated = errors.New("model completion reached its token limit")

// Document is structured content extracted from a source file.
type Document struct {
	Markdown       string
	StructuredJSON json.RawMessage
	ProcessingTime float64
}

func validDoclingStructuredJSON(data []byte) bool {
	var document struct {
		Texts    []json.RawMessage `json:"texts"`
		Tables   []json.RawMessage `json:"tables"`
		Pictures []json.RawMessage `json:"pictures"`
	}
	return json.Unmarshal(data, &document) == nil && len(document.Texts)+len(document.Tables)+len(document.Pictures) > 0
}

// ExtractionProgress reports one document's position in a collection extraction.
type ExtractionProgress struct {
	Index     int
	Stage     string
	Completed bool
	Cached    bool
	Error     string
}

// ExtractionProgressFunc receives collection extraction updates.
type ExtractionProgressFunc func(ExtractionProgress)

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
