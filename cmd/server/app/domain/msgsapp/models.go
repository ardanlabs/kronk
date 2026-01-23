package msgsapp

import (
	"encoding/json"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// =============================================================================
// Request Types

// MessagesRequest represents an Anthropic Messages API request.
type MessagesRequest struct {
	Model         string        `json:"model"`
	Messages      []Message     `json:"messages"`
	MaxTokens     int           `json:"max_tokens"`
	System        SystemContent `json:"system,omitempty"`
	Stream        bool          `json:"stream,omitempty"`
	Tools         []Tool        `json:"tools,omitempty"`
	Temperature   *float64      `json:"temperature,omitempty"`
	TopP          *float64      `json:"top_p,omitempty"`
	TopK          *int          `json:"top_k,omitempty"`
	StopSequences []string      `json:"stop_sequences,omitempty"`
}

// SystemContent can be a string or array of content blocks.
type SystemContent struct {
	Text   string         // Simple string content
	Blocks []ContentBlock // Array of content blocks
}

// UnmarshalJSON handles both string and array system content formats.
func (s *SystemContent) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	// Try string first
	if data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		s.Text = str
		return nil
	}

	// Try array of content blocks
	if data[0] == '[' {
		var blocks []ContentBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return err
		}
		s.Blocks = blocks
		return nil
	}

	return nil
}

// String returns the system content as a single string.
func (s SystemContent) String() string {
	if s.Text != "" {
		return s.Text
	}

	var result string
	for _, block := range s.Blocks {
		if block.Type == "text" {
			result += block.Text
		}
	}
	return result
}

// Message represents a message in the conversation.
type Message struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// Content can be a string or array of content blocks.
type Content struct {
	Text   string         // Simple string content
	Blocks []ContentBlock // Array of content blocks
}

// UnmarshalJSON handles both string and array content formats.
func (c *Content) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	// Try string first
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		c.Text = s
		return nil
	}

	// Try array of content blocks
	if data[0] == '[' {
		var blocks []ContentBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return err
		}
		c.Blocks = blocks
		return nil
	}

	return nil
}

// ContentBlock represents a single content block in a message.
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "tool_use", "tool_result"

	// Text block fields
	Text string `json:"text,omitempty"`

	// Image block fields
	Source *ImageSource `json:"source,omitempty"`

	// Tool use block fields (in assistant messages)
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	// Tool result block fields (in user messages)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// ImageSource represents an image source.
type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema ToolSchema `json:"input_schema"`
}

// ToolSchema represents a JSON schema for tool input.
type ToolSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

func toOpenAI(req MessagesRequest) model.D {
	messages := make([]model.D, 0, len(req.Messages)+1)

	if sysContent := req.System.String(); sysContent != "" {
		messages = append(messages, model.D{
			"role":    "system",
			"content": sysContent,
		})
	}

	for _, msg := range req.Messages {
		converted := convertMessage(msg)
		messages = append(messages, converted...)
	}

	d := model.D{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
		"stream":     req.Stream,
	}

	if req.Temperature != nil {
		d["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		d["top_p"] = *req.TopP
	}
	if len(req.StopSequences) > 0 {
		d["stop"] = req.StopSequences
	}
	if len(req.Tools) > 0 {
		d["tools"] = convertTools(req.Tools)
	}

	return d
}

func convertMessage(msg Message) []model.D {
	// Simple text content - return single message
	if msg.Content.Text != "" {
		return []model.D{{
			"role":    msg.Role,
			"content": msg.Content.Text,
		}}
	}

	// No blocks - return empty content message
	if len(msg.Content.Blocks) == 0 {
		return []model.D{{
			"role":    msg.Role,
			"content": "",
		}}
	}

	// Handle blocks-based content
	if msg.Role == "assistant" {
		return convertAssistantMessage(msg.Content.Blocks)
	}

	return convertUserMessage(msg.Content.Blocks)
}

func convertAssistantMessage(blocks []ContentBlock) []model.D {
	// Separate tool_use blocks from content blocks
	var contentBlocks []ContentBlock
	var toolCalls []model.D

	for _, block := range blocks {
		if block.Type == "tool_use" {
			// Convert to OpenAI tool_call format
			// Note: Arguments need to be JSON-encoded as a string per OpenAI spec
			argsJSON, err := json.Marshal(block.Input)
			if err != nil {
				argsJSON = []byte("{}")
			}

			toolCalls = append(toolCalls, model.D{
				"id":   block.ID,
				"type": "function",
				"function": model.D{
					"name":      block.Name,
					"arguments": string(argsJSON),
				},
			})
		} else {
			contentBlocks = append(contentBlocks, block)
		}
	}

	// Build the assistant message
	msg := model.D{
		"role": "assistant",
	}

	// Add content if there are content blocks
	if len(contentBlocks) > 0 {
		converted := convertContentBlocks(contentBlocks)
		if len(converted) == 1 {
			// If only one text block, use string content
			if text, ok := converted[0]["text"].(string); ok {
				msg["content"] = text
			} else {
				msg["content"] = converted
			}
		} else {
			msg["content"] = converted
		}
	} else if len(toolCalls) == 0 {
		// No content and no tool calls - set empty content
		msg["content"] = ""
	}

	// Add tool_calls if present
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	return []model.D{msg}
}

func convertUserMessage(blocks []ContentBlock) []model.D {
	var messages []model.D
	var contentBlocks []ContentBlock

	// Separate tool_result blocks from regular content
	for _, block := range blocks {
		if block.Type == "tool_result" {
			// Create a separate tool role message for each tool result
			messages = append(messages, model.D{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      block.Content,
			})
		} else {
			contentBlocks = append(contentBlocks, block)
		}
	}

	// If there are content blocks, create a user message
	if len(contentBlocks) > 0 {
		userMsg := model.D{
			"role": "user",
		}

		converted := convertContentBlocks(contentBlocks)
		if len(converted) == 1 {
			// If only one text block, use string content
			if text, ok := converted[0]["text"].(string); ok {
				userMsg["content"] = text
			} else {
				userMsg["content"] = converted
			}
		} else {
			userMsg["content"] = converted
		}

		// Insert user message before tool messages to maintain order
		messages = append([]model.D{userMsg}, messages...)
	}

	// If no messages were created, return a single user message with empty content
	if len(messages) == 0 {
		messages = []model.D{{
			"role":    "user",
			"content": "",
		}}
	}

	return messages
}

func convertContentBlocks(blocks []ContentBlock) []model.D {
	result := make([]model.D, 0, len(blocks))

	for _, block := range blocks {
		switch block.Type {
		case "text":
			result = append(result, model.D{
				"type": "text",
				"text": block.Text,
			})

		case "image":
			if block.Source != nil {
				switch block.Source.Type {
				case "base64":
					result = append(result, model.D{
						"type": "image_url",
						"image_url": model.D{
							"url": fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data),
						},
					})
				case "url":
					result = append(result, model.D{
						"type": "image_url",
						"image_url": model.D{
							"url": block.Source.URL,
						},
					})
				}
			}

		// Note: tool_use and tool_result are handled at the message level,
		// not as content blocks, in convertAssistantMessage and convertUserMessage
		}
	}

	return result
}

func convertTools(tools []Tool) []model.D {
	result := make([]model.D, 0, len(tools))

	for _, tool := range tools {
		result = append(result, model.D{
			"type": "function",
			"function": model.D{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}

	return result
}

// =============================================================================
// Response Types

// MessagesResponse represents a non-streaming response.
type MessagesResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"` // "message"
	Role         string                 `json:"role"` // "assistant"
	Content      []ResponseContentBlock `json:"content"`
	Model        string                 `json:"model"`
	StopReason   string                 `json:"stop_reason,omitempty"` // "end_turn", "tool_use", "max_tokens"
	StopSequence *string                `json:"stop_sequence,omitempty"`
	Usage        Usage                  `json:"usage"`
}

// Encode implements web.Encoder.
func (r MessagesResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, "", err
	}
	return data, "application/json", nil
}

// ResponseContentBlock represents a content block in the response.
type ResponseContentBlock struct {
	Type string `json:"type"` // "text", "tool_use"

	// Text block
	Text string `json:"text,omitempty"`

	// Tool use block
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// Usage represents token usage.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// =============================================================================
// Streaming Event Types

// MessageStartEvent is sent at the start of a message.
type MessageStartEvent struct {
	Type    string               `json:"type"` // "message_start"
	Message MessageStartMetadata `json:"message"`
}

// MessageStartMetadata contains the initial message metadata.
type MessageStartMetadata struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"` // "message"
	Role         string                 `json:"role"` // "assistant"
	Content      []ResponseContentBlock `json:"content"`
	Model        string                 `json:"model"`
	StopReason   *string                `json:"stop_reason"`
	StopSequence *string                `json:"stop_sequence"`
	Usage        Usage                  `json:"usage"`
}

// ContentBlockStartEvent signals the start of a content block.
type ContentBlockStartEvent struct {
	Type         string               `json:"type"` // "content_block_start"
	Index        int                  `json:"index"`
	ContentBlock ContentBlockMetadata `json:"content_block"`
}

// ContentBlockMetadata contains initial content block info.
type ContentBlockMetadata struct {
	Type string `json:"type"` // "text", "tool_use"

	// For text blocks
	Text string `json:"text,omitempty"`

	// For tool_use blocks
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// ContentBlockDeltaEvent contains a delta update for a content block.
type ContentBlockDeltaEvent struct {
	Type  string       `json:"type"` // "content_block_delta"
	Index int          `json:"index"`
	Delta ContentDelta `json:"delta"`
}

// ContentDelta represents the delta payload.
type ContentDelta struct {
	Type string `json:"type"` // "text_delta", "input_json_delta"

	// For text_delta
	Text string `json:"text,omitempty"`

	// For input_json_delta (tool arguments)
	PartialJSON string `json:"partial_json,omitempty"`
}

// ContentBlockStopEvent signals the end of a content block.
type ContentBlockStopEvent struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

// MessageDeltaEvent contains the final message delta.
type MessageDeltaEvent struct {
	Type  string       `json:"type"` // "message_delta"
	Delta MessageDelta `json:"delta"`
	Usage DeltaUsage   `json:"usage"`
}

// MessageDelta contains the stop reason.
type MessageDelta struct {
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// DeltaUsage contains just output tokens for the delta.
type DeltaUsage struct {
	OutputTokens int `json:"output_tokens"`
}

// MessageStopEvent signals the end of the message.
type MessageStopEvent struct {
	Type string `json:"type"` // "message_stop"
}

func toMessagesResponse(resp model.ChatResponse) *MessagesResponse {
	content := make([]ResponseContentBlock, 0)

	if len(resp.Choice) > 0 {
		choice := resp.Choice[0]
		if choice.Message != nil {
			if choice.Message.Content != "" {
				content = append(content, ResponseContentBlock{
					Type: "text",
					Text: choice.Message.Content,
				})
			}

			for _, tc := range choice.Message.ToolCalls {
				content = append(content, ResponseContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: tc.Function.Arguments,
				})
			}
		}
	}

	stopReason := "end_turn"
	if len(resp.Choice) > 0 {
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonTool:
			stopReason = "tool_use"
		case model.FinishReasonStop:
			stopReason = "end_turn"
		}
	}

	var usage Usage
	if resp.Usage != nil {
		usage = Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	return &MessagesResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      resp.Model,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// =============================================================================
// Error Types

// ErrorResponse represents an Anthropic API error.
type ErrorResponse struct {
	Type  string      `json:"type"` // "error"
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Type    string `json:"type"` // "invalid_request_error", "authentication_error", etc.
	Message string `json:"message"`
}
