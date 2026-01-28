package model

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Objects represent the different types of data that is being processed.
const (
	ObjectChatUnknown   = "chat.unknown"
	ObjectChatText      = "chat.completion.chunk"
	ObjectChatTextFinal = "chat.completion"
	ObjectChatMedia     = "chat.media"
)

// Roles represent the different roles that can be used in a chat.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
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
	IsGPTModel    bool
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
	Template      Template
}

func (mi ModelInfo) String() string {
	var flags []string
	if mi.HasProjection {
		flags = append(flags, "projection")
	}
	if mi.IsGPTModel {
		flags = append(flags, "gpt")
	}
	if mi.IsEmbedModel {
		flags = append(flags, "embed")
	}
	if mi.IsRerankModel {
		flags = append(flags, "rerank")
	}

	flagStr := "none"
	if len(flags) > 0 {
		flagStr = strings.Join(flags, ", ")
	}

	sizeGB := float64(mi.Size) / (1024 * 1024 * 1024)

	return fmt.Sprintf("ID[%s]: Desc[%s]: Size[%.2fGB]: Template[%s]: Flags[%s]", mi.ID, mi.Desc, sizeGB, mi.Template.FileName, flagStr)
}

func toModelInfo(cfg Config, model llama.Model) ModelInfo {
	desc := llama.ModelDesc(model)
	size := llama.ModelSize(model)
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

	var filename string
	switch len(cfg.ModelFiles) {
	case 1:
		filename = filepath.Base(cfg.ModelFiles[0])
	default:
		filename = extractFolderName(cfg.ModelFiles[0])
	}

	modelID := strings.TrimSuffix(filename, path.Ext(filename))

	var isGPTModel bool
	if strings.Contains(modelID, "gpt") {
		isGPTModel = true
	}

	var isEmbedModel bool
	if strings.Contains(modelID, "embed") {
		isEmbedModel = true
	}

	var isRerankModel bool
	if strings.Contains(modelID, "rerank") {
		isRerankModel = true
	}

	return ModelInfo{
		ID:            modelID,
		HasProjection: cfg.ProjFile != "",
		Desc:          desc,
		Size:          size,
		IsGPTModel:    isGPTModel,
		IsEmbedModel:  isEmbedModel,
		IsRerankModel: isRerankModel,
		Metadata:      metadata,
	}
}

func extractFolderName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	dir := path.Dir(u.Path)
	return path.Base(dir)
}

// =============================================================================

// D represents a generic docment of fields and values.
type D map[string]any

// Clone creates a shallow copy of the document. This is useful when you need
// to modify the document without affecting the original.
func (d D) Clone() D {
	clone := make(D, len(d))
	maps.Copy(clone, d)
	return clone
}

// logSafeKeys defines the allowlist of fields that are safe to log.
// Fields like "messages" and "input" are excluded for privacy.
var logSafeKeys = []string{
	"model",
	"max_tokens",
	"temperature",
	"top_p",
	"top_k",
	"stream",
	"presence_penalty",
	"frequency_penalty",
	"stop",
	"seed",
	"n",
	"truncate",
	"truncate_direction",
	"top_n",
	"return_documents",
	"logprobs",
	"top_logprobs",
}

// String returns a string representation of the document containing only
// fields that are safe to log. This excludes sensitive fields like messages
// and input which may contain private user data.
func (d D) String() string {
	var b strings.Builder

	for i, key := range logSafeKeys {
		v, ok := d[key]
		if !ok {
			continue
		}

		if i > 0 {
			b.WriteString(" ")
		}

		fmt.Fprintf(&b, "%s[%v]:", key, v)
	}

	return b.String()
}

// TextMessage create a new text message.
func TextMessage(role string, content string) D {
	return D{
		"role":    role,
		"content": content,
	}
}

// TextMessageArray creates a new text message using the OpenAI array format
// where content is [{"type": "text", "text": "..."}].
func TextMessageArray(role string, content string) D {
	return D{
		"role": role,
		"content": []D{
			{"type": "text", "text": content},
		},
	}
}

// RawMediaMessage creates a new media message and should not be used for
// http based requests. On a http request, binary data is automatically
// converted to base64 and the system won't recognize this as media.
// Use ImageMessage, AudioMessage, or VideoMessage instead.
func RawMediaMessage(text string, media []byte) []D {
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

// ImageMessage create a new media message.
func ImageMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	url := fmt.Sprintf("data:image/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "text",
					"text": text,
				},
				{
					"type": "image_url",
					"image_url": D{
						"url": url,
					},
				},
			},
		},
	}
}

// AudioMessage create a new media message.
func AudioMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	data := fmt.Sprintf("data:audio/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "text",
					"text": text,
				},
				{
					"type": "input_audio",
					"input_audio": D{
						"data": data,
					},
				},
			},
		},
	}
}

// VideoMessage create a new media message.
func VideoMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	url := fmt.Sprintf("data:video/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "text",
					"text": text,
				},
				{
					"type": "video_url",
					"video_url": D{
						"url": url,
					},
				},
			},
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

// ToolCallArguments represents tool call arguments that marshal to a JSON
// string per OpenAI API spec, but can unmarshal from either a string or object.
type ToolCallArguments map[string]any

func (a ToolCallArguments) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte(`""`), nil
	}

	inner, err := json.Marshal(map[string]any(a))
	if err != nil {
		return nil, err
	}

	return json.Marshal(string(inner))
}

func (a *ToolCallArguments) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*a = nil
		return nil
	}

	// Try string first (OpenAI compliant format).
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}

		if s == "" {
			*a = nil
			return nil
		}

		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return err
		}

		*a = m
		return nil
	}

	// Fall back to object (non-compliant but some clients send this).
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	*a = m
	return nil
}

type ResponseToolCallFunction struct {
	Name      string            `json:"name"`
	Arguments ToolCallArguments `json:"arguments"`
}

type ResponseToolCall struct {
	ID       string                   `json:"id"`
	Index    int                      `json:"index"`
	Type     string                   `json:"type"`
	Function ResponseToolCallFunction `json:"function"`
	Status   int                      `json:"status,omitempty"`
	Raw      string                   `json:"raw,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

// ResponseMessage represents a single message in a response.
type ResponseMessage struct {
	Role      string             `json:"role,omitempty"`
	Content   string             `json:"content,omitempty"`
	Reasoning string             `json:"reasoning_content,omitempty"`
	ToolCalls []ResponseToolCall `json:"tool_calls,omitempty"`
}

// Choice represents a single choice in a response.
type Choice struct {
	Index           int              `json:"index"`
	Message         *ResponseMessage `json:"message,omitempty"`
	Delta           *ResponseMessage `json:"delta,omitempty"`
	Logprobs        *Logprobs        `json:"logprobs,omitempty"`
	FinishReasonPtr *string          `json:"finish_reason"`
}

// FinishReason return the finish reason as an empty
// string if it is nil.
func (c Choice) FinishReason() string {
	if c.FinishReasonPtr == nil {
		return ""
	}
	return *c.FinishReasonPtr
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

// TopLogprob represents a single token with its log probability.
type TopLogprob struct {
	Token   string  `json:"token"`
	Logprob float32 `json:"logprob"`
	Bytes   []byte  `json:"bytes,omitempty"`
}

// ContentLogprob represents log probability information for a single token.
type ContentLogprob struct {
	Token       string       `json:"token"`
	Logprob     float32      `json:"logprob"`
	Bytes       []byte       `json:"bytes,omitempty"`
	TopLogprobs []TopLogprob `json:"top_logprobs,omitempty"`
}

// Logprobs contains log probability information for the response.
type Logprobs struct {
	Content []ContentLogprob `json:"content,omitempty"`
}

// ChatResponse represents output for inference models.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choice  []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
	Prompt  string   `json:"prompt,omitempty"`
}

func chatResponseDelta(id string, object string, model string, index int, content string, reasoning bool, logprob *ContentLogprob) ChatResponse {
	var logprobs *Logprobs
	if logprob != nil {
		logprobs = &Logprobs{Content: []ContentLogprob{*logprob}}
	}

	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().Unix(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Delta: &ResponseMessage{
					Role:      RoleAssistant,
					Content:   forContent(content, reasoning),
					Reasoning: forReasoning(content, reasoning),
				},
				Logprobs:        logprobs,
				FinishReasonPtr: nil,
			},
		},
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

func chatResponseFinal(id string, object string, model string, index int, prompt string, content string, reasoning string, respToolCalls []ResponseToolCall, logprobsData []ContentLogprob, u Usage) ChatResponse {
	finishReason := FinishReasonStop
	if len(respToolCalls) > 0 {
		finishReason = FinishReasonTool
	}

	var logprobs *Logprobs
	if len(logprobsData) > 0 {
		logprobs = &Logprobs{Content: logprobsData}
	}

	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().Unix(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Message: &ResponseMessage{
					Role:      RoleAssistant,
					Content:   content,
					Reasoning: reasoning,
					ToolCalls: respToolCalls,
				},
				Delta: &ResponseMessage{
					Role:      RoleAssistant,
					Content:   content,
					Reasoning: reasoning,
					ToolCalls: respToolCalls,
				},
				Logprobs:        logprobs,
				FinishReasonPtr: &finishReason,
			},
		},
		Usage:  &u,
		Prompt: prompt,
	}
}

func ChatResponseErr(id string, object string, model string, index int, prompt string, err error, u Usage) ChatResponse {
	finishReason := FinishReasonError
	return ChatResponse{
		ID:      id,
		Object:  object,
		Created: time.Now().Unix(),
		Model:   model,
		Choice: []Choice{
			{
				Index: index,
				Delta: &ResponseMessage{
					Role:    RoleAssistant,
					Content: err.Error(),
				},
				FinishReasonPtr: &finishReason,
			},
		},
		Usage:  &u,
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

// EmbedUsage provides token usage information for embeddings.
type EmbedUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbedReponse represents the output for an embedding call.
type EmbedReponse struct {
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Data    []EmbedData `json:"data"`
	Usage   EmbedUsage  `json:"usage"`
}

// =============================================================================

// RerankResult represents a single document's reranking result.
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float32 `json:"relevance_score"`
	Document       string  `json:"document,omitempty"`
}

// RerankUsage provides token usage information for reranking.
type RerankUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// RerankResponse represents the output for a reranking call.
type RerankResponse struct {
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Data    []RerankResult `json:"data"`
	Usage   RerankUsage    `json:"usage"`
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
	Content any    `json:"content"` // string | []chatMessageContent | nil
}

func (ccm *chatMessage) UnmarshalJSON(b []byte) error {
	var app struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	if err := json.Unmarshal(b, &app); err != nil {
		return err
	}

	// Content can be empty for assistant messages with tool_calls,
	// or for tool role messages where content is optional.
	if len(app.Content) == 0 || string(app.Content) == "null" {
		*ccm = chatMessage{
			Role:    app.Role,
			Content: nil,
		}
		return nil
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

// =============================================================================

// Template provides the template file name.
type Template struct {
	FileName string
	Script   string
}
