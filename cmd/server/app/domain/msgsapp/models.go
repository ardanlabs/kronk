package msgsapp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// =============================================================================
// Request Types

// MessagesRequest represents an Anthropic Messages API request.
type MessagesRequest struct {
	Model         string          `json:"model"`
	Messages      []Message       `json:"messages"`
	MaxTokens     int             `json:"max_tokens"`
	System        SystemContent   `json:"system"`
	Stream        bool            `json:"stream,omitempty"`
	Tools         []Tool          `json:"tools,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	ToolChoice    *ToolChoice     `json:"tool_choice,omitempty"`
	Thinking      json.RawMessage `json:"thinking,omitempty"`
	OutputConfig  json.RawMessage `json:"output_config,omitempty"`
}

// ToolChoice selects whether and which custom tool the model should use.
type ToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use,omitempty"`
}

// UnmarshalJSON rejects unknown tool selection controls rather than ignoring them.
func (c *ToolChoice) UnmarshalJSON(data []byte) error {
	type choice ToolChoice
	var decoded choice
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	*c = ToolChoice(decoded)
	return nil
}

// UnmarshalJSON preserves exact numbers in tool schemas as well as history.
func (r *MessagesRequest) UnmarshalJSON(data []byte) error {
	type request MessagesRequest
	var decoded request
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	*r = MessagesRequest(decoded)
	return nil
}

// SystemContent can be a string or array of content blocks.
type SystemContent struct {
	Text   string         // Simple string content
	Blocks []ContentBlock // Array of content blocks
}

// UnmarshalJSON handles both string and array system content formats.
func (s *SystemContent) UnmarshalJSON(data []byte) error {
	var c Content
	if err := c.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("system: %w", err)
	}
	s.Text, s.Blocks = c.Text, c.Blocks
	return nil
}

// MarshalJSON serializes system content in its protocol string or block form.
func (s SystemContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(Content(s))
}

// String returns the system content as a single string.
func (s SystemContent) String() string {
	switch {
	case s.Text != "":
		return s.Text
	}

	var result strings.Builder
	for _, block := range s.Blocks {
		switch block.Type {
		case "text":
			result.WriteString(block.Text)
		}
	}
	return result.String()
}

// Message represents a message in the conversation.
type Message struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// UnmarshalJSON requires content rather than treating an absent field as empty text.
func (m *Message) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var content Content
	if err := content.UnmarshalJSON(decoded.Content); err != nil {
		return err
	}
	*m = Message{Role: decoded.Role, Content: content}
	return nil
}

// Content can be a string or array of content blocks.
type Content struct {
	Text   string         // Simple string content
	Blocks []ContentBlock // Array of content blocks
}

// MarshalJSON serializes Content as either a string or array of blocks.
func (c Content) MarshalJSON() ([]byte, error) {
	switch {
	case c.Blocks != nil:
		return json.Marshal(c.Blocks)
	}
	return json.Marshal(c.Text)
}

// UnmarshalJSON handles both string and array content formats.
func (c *Content) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return errors.New("content must be a string or array of blocks")
	}

	var decoded Content
	switch data[0] {
	case '"':
		if err := json.Unmarshal(data, &decoded.Text); err != nil {
			return err
		}
	case '[':
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&decoded.Blocks); err != nil {
			return err
		}
	default:
		return errors.New("content must be a string or array of blocks")
	}
	*c = decoded
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
	ToolUseID string  `json:"tool_use_id,omitempty"`
	Content   Content `json:"content"`
	IsError   bool    `json:"is_error,omitempty"`
}

// UnmarshalJSON preserves numeric tool inputs and requires text on text blocks.
func (c *ContentBlock) UnmarshalJSON(data []byte) error {
	type block ContentBlock
	var decoded struct {
		block
		Text *string `json:"text"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	if decoded.Type == "text" && decoded.Text == nil {
		return errors.New("text block requires text")
	}
	if decoded.Text != nil {
		decoded.block.Text = *decoded.Text
	}
	*c = ContentBlock(decoded.block)
	return nil
}

// MarshalJSON emits result content only on tool results and required text on text blocks.
func (c ContentBlock) MarshalJSON() ([]byte, error) {
	type block ContentBlock
	var content *Content
	var text *string
	if c.Type == "tool_result" {
		content = &c.Content
	}
	if c.Type == "text" {
		text = &c.Text
	}
	return json.Marshal(struct {
		block
		Content *Content `json:"content,omitempty"`
		Text    *string  `json:"text,omitempty"`
	}{block: block(c), Content: content, Text: text})
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
	Type        string  `json:"type,omitempty"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	InputSchema model.D `json:"input_schema"`
}

// Validate checks the supported Messages protocol before acquiring a model.
func (r MessagesRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.Model) == "":
		return errors.New("missing model field")
	case r.MaxTokens == 0:
		return errors.New("missing max_tokens field")
	case r.MaxTokens < 0:
		return errors.New("max_tokens must be positive")
	case len(r.StopSequences) > 0:
		return errors.New("stop_sequences is not supported")
	case len(r.Messages) == 0:
		return model.ErrMessagesMissing
	case len(r.Thinking) > 0:
		return errors.New("thinking is not supported")
	case len(r.OutputConfig) > 0:
		return errors.New("output_config is not supported")
	case r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 1):
		return errors.New("temperature must be between 0 and 1")
	case r.TopP != nil && (*r.TopP < 0 || *r.TopP > 1):
		return errors.New("top_p must be between 0 and 1")
	case r.TopK != nil && *r.TopK < 0:
		return errors.New("top_k must be nonnegative")
	}
	for _, block := range r.System.Blocks {
		if block.Type != "text" {
			return errors.New("system supports only text blocks")
		}
	}
	pending := make(map[string]bool)
	callIDs := make(map[string]bool)
	for i, msg := range r.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			return fmt.Errorf("messages[%d]: role must be user or assistant", i)
		}
		if len(pending) > 0 && msg.Role != "user" {
			return fmt.Errorf("messages[%d]: tool_use must be followed by user tool_result blocks", i)
		}
		seenContent := false
		for j, block := range msg.Content.Blocks {
			if block.Type == "tool_result" && seenContent {
				return fmt.Errorf("messages[%d]: tool_result blocks must precede other content", i)
			}
			if block.Type != "tool_result" {
				seenContent = true
			}
			if err := validateBlock(block, msg.Role); err != nil {
				return fmt.Errorf("messages[%d].content[%d]: %w", i, j, err)
			}
			switch block.Type {
			case "tool_use":
				if callIDs[block.ID] {
					return fmt.Errorf("messages[%d]: duplicate tool_use id %q", i, block.ID)
				}
				callIDs[block.ID], pending[block.ID] = true, true
			case "tool_result":
				if !pending[block.ToolUseID] {
					return fmt.Errorf("messages[%d]: tool_result id %q has no pending tool_use", i, block.ToolUseID)
				}
				delete(pending, block.ToolUseID)
			}
		}
		if msg.Role == "user" && len(pending) > 0 {
			return fmt.Errorf("messages[%d]: missing tool_result for preceding tool_use", i)
		}
	}
	names := make(map[string]bool, len(r.Tools))
	for i, tool := range r.Tools {
		if tool.Type != "" && tool.Type != "custom" {
			return fmt.Errorf("tools[%d]: only custom tools are supported", i)
		}
		if strings.TrimSpace(tool.Name) == "" || names[tool.Name] {
			return fmt.Errorf("tools[%d]: tool name must be nonempty and unique", i)
		}
		if tool.InputSchema == nil || tool.InputSchema["type"] != "object" {
			return fmt.Errorf("tools[%d]: input_schema must be an object schema", i)
		}
		names[tool.Name] = true
	}
	return validateToolSelection(r.ToolChoice, names)
}

func validateToolSelection(choice *ToolChoice, names map[string]bool) error {
	if choice == nil {
		return nil
	}
	if choice.DisableParallelToolUse != nil && *choice.DisableParallelToolUse {
		return errors.New("disable_parallel_tool_use is not supported")
	}
	if choice.Type != "tool" && choice.Name != "" {
		return errors.New("tool_choice name is only supported with type tool")
	}
	switch choice.Type {
	case "auto", "none":
		return nil
	case "any":
		if len(names) > 0 {
			return nil
		}
		return errors.New("tool_choice any requires tools")
	case "tool":
		if names[choice.Name] {
			return nil
		}
		return errors.New("tool_choice tool requires a declared tool name")
	default:
		return fmt.Errorf("unsupported tool_choice type %q", choice.Type)
	}
}

func validateBlock(block ContentBlock, role string) error {
	switch block.Type {
	case "text":
		return nil
	case "image":
		if role != "user" {
			return errors.New("image blocks require a user message")
		}
		return validateImageSource(block.Source)
	case "tool_use":
		if role != "assistant" || strings.TrimSpace(block.ID) == "" || strings.TrimSpace(block.Name) == "" {
			return errors.New("tool_use requires an assistant message, id and name")
		}
		switch input := block.Input.(type) {
		case map[string]any:
			if input == nil {
				return errors.New("tool_use input must be an object")
			}
		case model.D:
			if input == nil {
				return errors.New("tool_use input must be an object")
			}
		default:
			return errors.New("tool_use input must be an object")
		}
		if _, err := json.Marshal(block.Input); err != nil {
			return fmt.Errorf("encoding tool_use input: %w", err)
		}
		return nil
	case "tool_result":
		if role != "user" || strings.TrimSpace(block.ToolUseID) == "" {
			return errors.New("tool_result requires a user message and tool_use_id")
		}
		for _, part := range block.Content.Blocks {
			if part.Type != "text" && part.Type != "image" {
				return errors.New("tool_result content supports only text and image blocks")
			}
			if err := validateBlock(part, "user"); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported content block type %q", block.Type)
	}
}

func validateImageSource(source *ImageSource) error {
	if source == nil {
		return errors.New("image source is required")
	}
	switch source.Type {
	case "base64":
		switch source.MediaType {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return errors.New("unsupported image media_type")
		}
		if source.Data == "" {
			return errors.New("base64 image data is required")
		}
		if _, err := base64.StdEncoding.DecodeString(source.Data); err != nil {
			return fmt.Errorf("invalid base64 image data: %w", err)
		}
	case "url":
		u, err := url.Parse(source.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return errors.New("image url must be an absolute http or https URL")
		}
	default:
		return fmt.Errorf("unsupported image source type %q", source.Type)
	}
	return nil
}

func toolResultContent(block ContentBlock) any {
	// Chat templates have no portable is_error flag. Put the failure in visible
	// text so it survives both text-only flattening and multimodal rendering.
	if block.Content.Blocks == nil {
		text := block.Content.Text
		if block.IsError {
			text = "Tool execution failed: " + text
		}
		return text
	}
	parts := convertContentBlocks(block.Content.Blocks)
	if block.IsError {
		parts = append([]model.D{{"type": "text", "text": "Tool execution failed: "}}, parts...)
	}
	return parts
}

func normalizeSchema(value any) any {
	switch v := value.(type) {
	case model.D:
		return normalizeSchema(map[string]any(v))
	case map[string]any:
		d := make(model.D, len(v))
		for key, item := range v {
			d[key] = normalizeSchema(item)
		}
		return d
	case []any:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = normalizeSchema(item)
		}
		return items
	case []model.D:
		items := make([]model.D, len(v))
		for i, item := range v {
			items[i] = normalizeSchema(item).(model.D)
		}
		return items
	default:
		return value
	}
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
	case req.TopK != nil:
		d["top_k"] = *req.TopK
	}
	switch {
	case len(req.Tools) > 0:
		d["tools"] = convertTools(req.Tools)
	}
	if req.ToolChoice != nil {
		choice := req.ToolChoice
		switch choice.Type {
		case "auto", "none":
			d["tool_choice"] = choice.Type
		case "any":
			d["tool_choice"] = "required"
		case "tool":
			d["tool_choice"] = model.D{"type": "function", "function": model.D{"name": choice.Name}}
		}
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
			content := toolResultContent(block)
			messages = append(messages, model.D{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      content,
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

		// Validated tool results precede the user's follow-up content.
		messages = append(messages, userMsg)
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
				"parameters":  normalizeSchema(tool.InputSchema),
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

// MarshalJSON includes text even when empty, and never on tool blocks.
func (c ContentBlockMetadata) MarshalJSON() ([]byte, error) {
	if c.Type == "text" {
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: c.Type, Text: c.Text})
	}
	type metadata ContentBlockMetadata
	c.Text = ""
	return json.Marshal(metadata(c))
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

// DeltaUsage contains the cumulative token usage for the message at the end
// of the stream. Anthropic clients read input_tokens from message_delta to get
// the final, cache-inclusive prompt token count, since message_start fires
// before the model has computed it.
type DeltaUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// MessageStopEvent signals the end of the message.
type MessageStopEvent struct {
	Type string `json:"type"` // "message_stop"
}

func toMessagesResponse(resp model.ChatResponse) *MessagesResponse {
	content := make([]ResponseContentBlock, 0)

	switch {
	case len(resp.Choices) > 0:
		choice := resp.Choices[0]
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
				input := map[string]any(tc.Function.Arguments)
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, ResponseContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
		}
	}

	var stopReason string
	switch {
	case len(resp.Choices) > 0:
		stopReason = toAnthropicStopReason(resp.Choices[0].FinishReason())
	default:
		stopReason = toAnthropicStopReason("")
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
