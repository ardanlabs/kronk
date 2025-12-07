package model

import (
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Objects represent the different types of data that is being processed.
const (
	ObjectChatUnknown = "chat.unknown"
	ObjectChatText    = "chat.text"
	ObjectChatMedia   = "chat.media"
)

// Roles represent the different roles that can be used in a chat.
const (
	RoleAssistant = "assistant"
)

// FinishReasons represent the different reasons a response can be finished.
const (
	FinishReasonStop  = "stop"
	FinishReasonTool  = "tool_calls"
	FinishReasonError = "error"
)

// =============================================================================

// ModelInfo represents the model's card information.
type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	HasEncoder    bool
	HasDecoder    bool
	IsRecurrent   bool
	IsHybrid      bool
	IsGPT         bool
	Metadata      map[string]string
}

func newModelInfo(cfg Config, model llama.Model) ModelInfo {
	desc := llama.ModelDesc(model)
	size := llama.ModelSize(model)
	encoder := llama.ModelHasEncoder(model)
	decoder := llama.ModelHasDecoder(model)
	recurrent := llama.ModelIsRecurrent(model)
	hybrid := llama.ModelIsHybrid(model)
	count := llama.ModelMetaCount(model)
	metadata := make(map[string]string)

	for i := range count {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					return
				}
			}()

			key, ok := llama.ModelMetaKeyByIndex(model, i)
			if !ok {
				return
			}

			value, ok := llama.ModelMetaValStrByIndex(model, i)
			if !ok {
				return
			}

			metadata[key] = value
		}()
	}

	filename := filepath.Base(cfg.ModelFile)
	modelID := strings.TrimSuffix(filename, path.Ext(filename))

	var isGPTModel bool
	if strings.Contains(modelID, "gpt") {
		isGPTModel = true
	}

	return ModelInfo{
		ID:            modelID,
		HasProjection: cfg.ProjectionFile != "",
		Desc:          desc,
		Size:          size,
		HasEncoder:    encoder,
		HasDecoder:    decoder,
		IsRecurrent:   recurrent,
		IsHybrid:      hybrid,
		IsGPT:         isGPTModel,
		Metadata:      metadata,
	}
}

// =============================================================================

// TextMessage create a new text message.
func TextMessage(role string, content string) D {
	return D{
		"role":    role,
		"content": content,
	}
}

// MediaMessage create a new media message.
func MediaMessage(media []byte) D {
	return D{
		"content": media,
	}
}

// DocumentArray creates a new document array and can apply the
// set of documents.
func DocumentArray(doc ...D) []D {
	msgs := make([]D, len(doc))
	copy(msgs, doc)
	return msgs
}

// D represents a generic docment of fields and values.
type D map[string]any

// =============================================================================

type ResponseToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Status    int            `json:"status"`
	Raw       string         `json:"raw"`
	Error     string         `json:"error"`
}

// ResponseMessage represents a single message in a response.
type ResponseMessage struct {
	Role      string             `json:"role"`
	Content   string             `json:"content"`
	Reasoning string             `json:"reasoning"`
	ToolCalls []ResponseToolCall `json:"tool_calls,omitempty"`
}

// Choice represents a single choice in a response.
type Choice struct {
	Index        int             `json:"index"`
	Delta        ResponseMessage `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

// Usage provides details usage information for the request.
type Usage struct {
	InputTokens      int     `json:"input_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TokensPerSecond  float64 `json:"tokens_per_second"`
}

// ChatResponse represents output for chat and vision models.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choice  []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	Prompt  string   `json:"prompt"`
}

func chatResponseDelta(id string, object string, model string, index int, content string, reasoning bool, u Usage) ChatResponse {
	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().UnixMilli(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Delta: ResponseMessage{
					Role:      RoleAssistant,
					Content:   forContent(content, reasoning),
					Reasoning: forReasoning(content, reasoning),
				},
				FinishReason: "",
			},
		},
		Usage: u,
	}
}

func forContent(content string, reasoning bool) string {
	if !reasoning {
		return content
	}

	return ""
}

func forReasoning(content string, reasoning bool) string {
	if reasoning {
		return content
	}

	return ""
}

func chatResponseFinal(id string, object string, model string, index int, prompt string, content string, reasoning string, respToolCall ResponseToolCall, u Usage) ChatResponse {
	finishReason := FinishReasonStop
	if respToolCall.ID != "" {
		finishReason = FinishReasonTool
	}

	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().UnixMilli(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Delta: ResponseMessage{
					Role:      RoleAssistant,
					Content:   content,
					Reasoning: reasoning,
					ToolCalls: []ResponseToolCall{respToolCall},
				},
				FinishReason: finishReason,
			},
		},
		Usage:  u,
		Prompt: prompt,
	}
}

func ChatResponseErr(id string, object string, model string, index int, prompt string, err error, u Usage) ChatResponse {
	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().UnixMilli(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Delta: ResponseMessage{
					Role:    RoleAssistant,
					Content: err.Error(),
				},
				FinishReason: FinishReasonError,
			},
		},
		Usage:  u,
		Prompt: prompt,
	}
}
