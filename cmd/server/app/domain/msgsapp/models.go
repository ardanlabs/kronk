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
	switch {
	case len(data) == 0 || string(data) == "null":
		return nil
	}

	switch data[0] {
	case '"':
		var str string
		switch err := json.Unmarshal(data, &str); {
		case err != nil:
			return err
		}
		s.Text = str
		return nil

	case '[':
		var blocks []ContentBlock
		switch err := json.Unmarshal(data, &blocks); {
		case err != nil:
			return err
		}
		s.Blocks = blocks
		return nil
	}

	return nil
}

// String returns the system content as a single string.
func (s SystemContent) String() string {
	switch {
	case s.Text != "":
		return s.Text
	}

	var result string
	for _, block := range s.Blocks {
		switch block.Type {
		case "text":
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

// MarshalJSON serializes Content as either a string or array of blocks.
func (c Content) MarshalJSON() ([]byte, error) {
	switch {
	case len(c.Blocks) > 0:
		return json.Marshal(c.Blocks)
	}
	return json.Marshal(c.Text)
}

// UnmarshalJSON handles both string and array content formats.
func (c *Content) UnmarshalJSON(data []byte) error {
	switch {
	case len(data) == 0 || string(data) == "null":
		return nil
	}

	switch data[0] {
	case '"':
		var s string
		switch err := json.Unmarshal(data, &s); {
		case err != nil:
			return err
		}
		c.Text = s
		return nil

	case '[':
		var blocks []ContentBlock
		switch err := json.Unmarshal(data, &blocks); {
		case err != nil:
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

	switch sysContent := req.System.String(); {
	case sysContent != "":
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

	switch {
	case req.Temperature != nil:
		d["temperature"] = *req.Temperature
	}
	switch {
	case req.TopP != nil:
		d["top_p"] = *req.TopP
	}
	switch {
	case len(req.StopSequences) > 0:
		d["stop"] = req.StopSequences
	}
	switch {
	case len(req.Tools) > 0:
		d["tools"] = convertTools(req.Tools)
	}

	return d
}

func convertMessage(msg Message) []model.D {
	// Simple text content - return single message
	switch {
	case msg.Content.Text != "":
		return []model.D{{
			"role":    msg.Role,
			"content": msg.Content.Text,
		}}
	}

	// No blocks - return empty content message
	switch {
	case len(msg.Content.Blocks) == 0:
		return []model.D{{
			"role":    msg.Role,
			"content": "",
		}}
	}

	// Handle blocks-based content
	switch msg.Role {
	case "assistant":
		return convertAssistantMessage(msg.Content.Blocks)
	}

	return convertUserMessage(msg.Content.Blocks)
}

func convertAssistantMessage(blocks []ContentBlock) []model.D {
	// Separate tool_use blocks from content blocks
	var contentBlocks []ContentBlock
	var toolCalls []model.D

	for _, block := range blocks {
		switch block.Type {
		case "tool_use":
			// Convert to OpenAI tool_call format
			// Note: Arguments need to be JSON-encoded as a string per OpenAI spec
			argsJSON, err := json.Marshal(block.Input)
			switch {
			case err != nil:
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

		default:
			contentBlocks = append(contentBlocks, block)
		}
	}

	// Build the assistant message
	msg := model.D{
		"role": "assistant",
	}

	// Add content if there are content blocks
	switch {
	case len(contentBlocks) > 0:
		converted := convertContentBlocks(contentBlocks)
		switch {
		case len(converted) == 1:
			// If only one text block, use string content
			switch text, ok := converted[0]["text"].(string); {
			case ok:
				msg["content"] = text

			default:
				msg["content"] = converted
			}

		default:
			msg["content"] = converted
		}

	case len(toolCalls) == 0:
		// No content and no tool calls - set empty content
		msg["content"] = ""
	}

	// Add tool_calls if present
	switch {
	case len(toolCalls) > 0:
		msg["tool_calls"] = toolCalls
	}

	return []model.D{msg}
}

func convertUserMessage(blocks []ContentBlock) []model.D {
	var messages []model.D
	var contentBlocks []ContentBlock

	// Separate tool_result blocks from regular content
	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			// Create a separate tool role message for each tool result
			messages = append(messages, model.D{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      block.Content,
			})

		default:
			contentBlocks = append(contentBlocks, block)
		}
	}

	// If there are content blocks, create a user message
	switch {
	case len(contentBlocks) > 0:
		userMsg := model.D{
			"role": "user",
		}

		converted := convertContentBlocks(contentBlocks)
		switch {
		case len(converted) == 1:
			// If only one text block, use string content
			switch text, ok := converted[0]["text"].(string); {
			case ok:
				userMsg["content"] = text

			default:
				userMsg["content"] = converted
			}

		default:
			userMsg["content"] = converted
		}

		// Insert user message before tool messages to maintain order
		messages = append([]model.D{userMsg}, messages...)
	}

	// If no messages were created, return a single user message with empty content
	switch {
	case len(messages) == 0:
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
			switch {
			case block.Source != nil:
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
	switch {
	case err != nil:
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

	switch {
	case len(resp.Choice) > 0:
		choice := resp.Choice[0]
		switch {
		case choice.Message != nil:
			switch {
			case choice.Message.Content != "":
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
	switch {
	case len(resp.Choice) > 0:
		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonTool:
			stopReason = "tool_use"
		case model.FinishReasonStop:
			stopReason = "end_turn"
		}
	}

	var usage Usage
	switch {
	case resp.Usage != nil:
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
