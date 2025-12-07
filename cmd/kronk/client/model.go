package client

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type D map[string]any

// =============================================================================

type Error struct {
	Err struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (err *Error) Error() string {
	return err.Err.Message
}

// =============================================================================

type Time struct {
	time.Time
}

func ToTime(sec int64) Time {
	return Time{
		Time: time.Unix(sec, 0),
	}
}

func (t *Time) UnmarshalJSON(data []byte) error {
	d := strings.Trim(string(data), "\"")

	num, err := strconv.Atoi(d)
	if err != nil {
		return err
	}

	t.Time = time.Unix(int64(num), 0)

	return nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	data := strconv.Itoa(int(t.Unix()))
	return []byte(data), nil
}

// =============================================================================

type Function struct {
	Name      string
	Arguments map[string]any
}

func (f *Function) UnmarshalJSON(b []byte) error {
	var tmp struct {
		Name         string `json:"name"`
		RawArguments string `json:"arguments"`
	}

	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}

	arguments := make(map[string]any)
	if err := json.Unmarshal([]byte(tmp.RawArguments), &arguments); err != nil {
		return err
	}

	*f = Function{
		Name:      tmp.Name,
		Arguments: arguments,
	}

	return nil
}

type ToolCall struct {
	ID       string   `json:"id,omitempty"`
	Index    int      `json:"index"`
	Type     string   `json:"type,omitempty"`
	Function Function `json:"function"`
}

type ChatDeltaSSE struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Reasoning string     `json:"reasoning"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ChatChoiceSSE struct {
	Index        int          `json:"index"`
	Delta        ChatDeltaSSE `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

type ChatSSE struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created Time            `json:"created"`
	Model   string          `json:"model"`
	Choices []ChatChoiceSSE `json:"choices"`
	Error   string          `json:"error"`
}

// =============================================================================

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatChoice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

type Chat struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created Time         `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
}

// =============================================================================

type EmbeddingData struct {
	Index     int       `json:"index"`
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
}

type Embedding struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created Time            `json:"created"`
	Model   string          `json:"model"`
	Data    []EmbeddingData `json:"data"`
}
