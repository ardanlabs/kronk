package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Objects represent the different types of data that is being processed.
const (
	ObjectChatUnknown = "chat.unknown"
	ObjectChatText    = "chat.completion.chunk"
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
	IsGPTModel    bool
	IsEmbedModel  bool
	Metadata      map[string]string
}

func toModelInfo(cfg Config, model llama.Model) ModelInfo {
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

	var isEmbedModel bool
	if strings.Contains(modelID, "embed") {
		isEmbedModel = true
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
		IsGPTModel:    isGPTModel,
		IsEmbedModel:  isEmbedModel,
		Metadata:      metadata,
	}
}

// =============================================================================

// D represents a generic docment of fields and values.
type D map[string]any

// TextMessage create a new text message.
func TextMessage(role string, content string) D {
	return D{
		"role":    role,
		"content": content,
	}
}

// MediaMessage create a new media message.
func MediaMessage(text string, media []byte) []D {
	return []D{
		{
			"role":    "user",
			"content": media,
		},
		{
			"role":    "user",
			"content": text,
		},
	}
}

// DocumentArray creates a new document array and can apply the
// set of documents.
func DocumentArray(doc ...D) []D {
	msgs := make([]D, len(doc))
	copy(msgs, doc)
	return msgs
}

// MapToModelD converts a map[string]any to a D.
func MapToModelD(m map[string]any) D {
	d := make(D, len(m))

	for k, v := range m {
		d[k] = convertValue(v)
	}

	return d
}

func convertValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return MapToModelD(val)

	case []any:
		allMaps := true
		for _, elem := range val {
			if _, ok := elem.(map[string]any); !ok {
				allMaps = false
				break
			}
		}

		if allMaps {
			result := make([]D, len(val))
			for i, elem := range val {
				result[i] = convertValue(elem).(D)
			}
			return result
		}

		for i, elem := range val {
			val[i] = convertValue(elem)
		}

		return val

	default:
		return v
	}
}

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
	PromptTokens     int     `json:"prompt_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TokensPerSecond  float64 `json:"tokens_per_second"`
}

// ChatResponse represents output for inference models.
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

func chatResponseFinal(id string, object string, model string, index int, prompt string, content string, reasoning string, respToolCalls []ResponseToolCall, u Usage) ChatResponse {
	finishReason := FinishReasonStop
	if len(respToolCalls) > 0 {
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
					ToolCalls: respToolCalls,
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

// =============================================================================

// EmbedData represents the data associated with an embedding call.
type EmbedData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// EmbedReponse represents the output for an embedding call.
type EmbedReponse struct {
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Data    []EmbedData `json:"data"`
}

func toEmbedResponse(modelID string, vec []float32) EmbedReponse {
	return EmbedReponse{
		Object:  "list",
		Created: time.Now().UnixMilli(),
		Model:   modelID,
		Data: []EmbedData{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: vec,
			},
		},
	}
}

// =============================================================================

type chatMessageURLData struct {
	// Only base64 encoded image is currently supported.
	URL string `json:"url"`
}

type chatMessageRawData struct {
	// Only base64 encoded audio is currently supported.
	Data string `json:"data"`
}

type chatMessageContent struct {
	Type      string             `json:"type"`
	Text      string             `json:"text"`
	ImageURL  chatMessageURLData `json:"image_url"`
	VideoURL  chatMessageURLData `json:"video_url"`
	AudioData chatMessageRawData `json:"input_audio"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string | []chatMessageContent
}

func (ccm *chatMessage) UnmarshalJSON(b []byte) error {
	var app struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	if err := json.Unmarshal(b, &app); err != nil {
		return err
	}

	if len(app.Content) == 0 {
		return errors.New("invalid input document")
	}

	var content any

	switch app.Content[0] {
	case '"':
		var str string
		err := json.Unmarshal(app.Content, &str)
		if err != nil {
			return err
		}

		content = str

	default:
		var multiContent []chatMessageContent
		if err := json.Unmarshal(app.Content, &multiContent); err != nil {
			return err
		}

		content = multiContent
	}

	*ccm = chatMessage{
		Role:    app.Role,
		Content: content,
	}

	return nil
}

type chatMessages struct {
	Messages []chatMessage `json:"messages"`
}

func toChatMessages(d D) (chatMessages, error) {
	jsonData, err := json.Marshal(d)
	if err != nil {
		return chatMessages{}, fmt.Errorf("marshaling: %w", err)
	}

	var msgs chatMessages
	if err := json.Unmarshal(jsonData, &msgs); err != nil {
		return chatMessages{}, fmt.Errorf("unmarshaling: %w", err)
	}

	return msgs, nil
}
