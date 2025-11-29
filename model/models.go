package model

import (
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Objects represent the different types of data that can be returned.
const (
	ObjectChat   = "chat"
	ObjectVision = "vision"
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
	Name        string
	Desc        string
	Size        uint64
	HasEncoder  bool
	HasDecoder  bool
	IsRecurrent bool
	IsHybrid    bool
	IsGPT       bool
	Metadata    map[string]string
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
		key, ok := llama.ModelMetaKeyByIndex(model, i)
		if !ok {
			continue
		}

		if key == "tokenizer.chat_template" {
			continue
		}

		value, ok := llama.ModelMetaValStrByIndex(model, i)
		if !ok {
			continue
		}

		metadata[key] = value
	}

	filename := filepath.Base(cfg.ModelFile)
	modelName := strings.TrimSuffix(filename, path.Ext(filename))

	var isGPTModel bool
	if strings.Contains(modelName, "gpt") {
		isGPTModel = true
	}

	return ModelInfo{
		Name:        modelName,
		Desc:        desc,
		Size:        size,
		HasEncoder:  encoder,
		HasDecoder:  decoder,
		IsRecurrent: recurrent,
		IsHybrid:    hybrid,
		IsGPT:       isGPTModel,
		Metadata:    metadata,
	}
}

// =============================================================================

// ToolParameter represents a single parameter for the tool call.
type ToolParameter struct {
	Name        string `json:"-"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolFunction represents the definition of a function tool.
type ToolFunction struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Arguments   map[string]ToolParameter `json:"arguments"`
}

// Tool represents a tool that can be called by the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// NewToolFunction initialized a function tool for the model.
func NewToolFunction(name string, description string, params ...ToolParameter) Tool {
	tool := Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        name,
			Description: description,
			Arguments:   map[string]ToolParameter{},
		},
	}

	for _, param := range params {
		tool.Function.Arguments[param.Name] = param
	}

	return tool
}

// =============================================================================

// ChatMessage represent a single message in a chat.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents input for chat and vision models.
type ChatRequest struct {
	Messages []ChatMessage
	Tools    []Tool
	Params   Params
}

// VisionRequest represents input for vision models.
type VisionRequest struct {
	ImageFile string
	Message   ChatMessage
	Params    Params
}

// =============================================================================

type ResponseToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
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

func chatResponseFinal(id string, object string, model string, index int, content string, reasoning string, respToolCall ResponseToolCall, u Usage) ChatResponse {
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
		Usage: u,
	}
}

func ChatResponseErr(id string, object string, model string, index int, err error, u Usage) ChatResponse {
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
		Usage: u,
	}
}
