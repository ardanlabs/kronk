package kronk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

// =============================================================================
// OpenAI Responses API types

// ResponseResponse represents the OpenAI Responses API response format.
type ResponseResponse struct {
	ID               string               `json:"id"`
	Object           string               `json:"object"`
	CreatedAt        int64                `json:"created_at"`
	Status           string               `json:"status"`
	CompletedAt      *int64               `json:"completed_at"`
	Error            *ResponseError       `json:"error"`
	IncompleteDetail *IncompleteDetail    `json:"incomplete_details"`
	Instructions     *string              `json:"instructions"`
	MaxOutputTokens  *int                 `json:"max_output_tokens"`
	Model            string               `json:"model"`
	Output           []ResponseOutputItem `json:"output"`
	ParallelToolCall bool                 `json:"parallel_tool_calls"`
	PrevResponseID   *string              `json:"previous_response_id"`
	Reasoning        ResponseReasoning    `json:"reasoning"`
	Store            bool                 `json:"store"`
	Temperature      float64              `json:"temperature"`
	Text             ResponseTextFormat   `json:"text"`
	ToolChoice       string               `json:"tool_choice"`
	Tools            []any                `json:"tools"`
	TopP             float64              `json:"top_p"`
	Truncation       string               `json:"truncation"`
	Usage            ResponseUsage        `json:"usage"`
	User             *string              `json:"user"`
	Metadata         map[string]any       `json:"metadata"`
}

// ResponseError represents an error in the response.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IncompleteDetail provides details about why a response is incomplete.
type IncompleteDetail struct {
	Reason string `json:"reason"`
}

// ResponseOutputItem represents an item in the output array.
// For type="message": ID, Status, Role, Content are used.
// For type="function_call": ID, Status, CallID, Name, Arguments are used.
// Arguments uses *string so function_call items always emit the field (even
// when empty) while message items omit it entirely.
type ResponseOutputItem struct {
	Type      string                `json:"type"`
	ID        string                `json:"id"`
	Status    string                `json:"status,omitempty"`
	Role      string                `json:"role,omitempty"`
	Content   []ResponseContentItem `json:"content,omitempty"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments *string               `json:"arguments,omitempty"`
}

// ResponseContentItem represents a content item within an output message.
type ResponseContentItem struct {
	Type        string   `json:"type"`
	Text        string   `json:"text"`
	Annotations []string `json:"annotations"`
}

// ResponseReasoning contains reasoning configuration/output.
type ResponseReasoning struct {
	Effort  *string `json:"effort"`
	Summary *string `json:"summary"`
}

// ResponseTextFormat specifies the text format configuration.
type ResponseTextFormat struct {
	Format ResponseFormatType `json:"format"`
}

// ResponseFormatType specifies the format type.
type ResponseFormatType struct {
	Type string `json:"type"`
}

// ResponseUsage contains token usage information.
type ResponseUsage struct {
	InputTokens        int                 `json:"input_tokens"`
	InputTokensDetails InputTokensDetails  `json:"input_tokens_details"`
	OutputTokens       int                 `json:"output_tokens"`
	OutputTokenDetail  OutputTokensDetails `json:"output_tokens_details"`
	TotalTokens        int                 `json:"total_tokens"`
}

// InputTokensDetails provides breakdown of input tokens.
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails provides breakdown of output tokens.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// =============================================================================
// Streaming event types

// ResponseStreamEvent represents a streaming event for the Responses API.
type ResponseStreamEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int                  `json:"sequence_number"`
	Response       *ResponseResponse    `json:"response,omitempty"`
	OutputIndex    *int                 `json:"output_index,omitempty"`
	ContentIndex   *int                 `json:"content_index,omitempty"`
	ItemID         string               `json:"item_id,omitempty"`
	Item           *ResponseOutputItem  `json:"item,omitempty"`
	Part           *ResponseContentItem `json:"part,omitempty"`
	Delta          string               `json:"delta,omitempty"`
	Text           string               `json:"text,omitempty"`
	Arguments      string               `json:"arguments,omitempty"`
	Name           string               `json:"name,omitempty"`
}

// =============================================================================

// Response provides support to interact with an inference model.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) Response(ctx context.Context, d model.D) (ResponseResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return ResponseResponse{}, fmt.Errorf("response: context has no deadline, provide a reasonable timeout")
	}

	d = convertInputToMessages(d)

	f := func(m *model.Model) (model.ChatResponse, error) {
		return m.Chat(ctx, d)
	}

	chatResp, err := nonStreaming(ctx, krn, f)
	if err != nil {
		return ResponseResponse{}, err
	}

	return toChatResponseToResponses(chatResp, d), nil
}

// ResponseStreaming provides streaming support for the Responses API.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) ResponseStreaming(ctx context.Context, d model.D) (<-chan ResponseStreamEvent, error) {
	if _, exists := ctx.Deadline(); !exists {
		return nil, fmt.Errorf("responses-streaming: context has no deadline, provide a reasonable timeout")
	}

	d = convertInputToMessages(d)

	f := func(m *model.Model) <-chan model.ChatResponse {
		return m.ChatStreaming(ctx, d)
	}

	ss := &streamState{
		responseID: "resp_" + uuid.New().String(),
		createdAt:  time.Now().Unix(),
		modelID:    krn.ModelInfo().ID,
		tools:      extractTools(d),
		params:     extractInputParams(d),
		d:          d,
	}

	p := streamProcessor[model.ChatResponse, ResponseStreamEvent]{
		Start:    ss.start,
		Process:  ss.process,
		Complete: ss.complete,
	}

	ef := func(err error) ResponseStreamEvent {
		return ResponseStreamEvent{
			Type: "error",
		}
	}

	return streamingWith(ctx, krn, f, p, ef)
}

// ResponseStreamingHTTP provides http handler support for a responses call.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) ResponseStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (ResponseResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return ResponseResponse{}, fmt.Errorf("responses-streaming-http: context has no deadline, provide a reasonable timeout")
	}

	var stream bool
	if streamReq, ok := d["stream"].(bool); ok {
		stream = streamReq
	}

	// -------------------------------------------------------------------------

	if !stream {
		resp, err := krn.Response(ctx, d)
		if err != nil {
			return ResponseResponse{}, fmt.Errorf("responses-streaming-http: response: %w", err)
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return resp, fmt.Errorf("responses-streaming-http: marshal: %w", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)

		return resp, nil
	}

	// -------------------------------------------------------------------------

	f, ok := w.(http.Flusher)
	if !ok {
		return ResponseResponse{}, fmt.Errorf("responses-streaming-http: streaming not supported")
	}

	ch, err := krn.ResponseStreaming(ctx, d)
	if err != nil {
		return ResponseResponse{}, fmt.Errorf("responses-streaming-http: stream-response: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	var lr ResponseResponse

	for event := range ch {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return lr, errors.New("responses-streaming-http: client disconnected, do not send response")
			}
		}

		data, err := json.Marshal(event)
		if err != nil {
			return lr, fmt.Errorf("responses-streaming-http: marshal: %w", err)
		}

		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
		f.Flush()

		if event.Response != nil {
			lr = *event.Response
		}
	}

	return lr, nil
}

// =============================================================================

type streamState struct {
	responseID      string
	createdAt       int64
	modelID         string
	tools           []any
	params          inputParams
	d               model.D
	seq             int
	outputIndex     int
	contentIndex    int
	msgID           string
	msgItemEmitted  bool
	fullText        string
	fullReasoning   string
	fcItems         []ResponseOutputItem
	fcIDs           []string
	fcArgsAccum     []string
	toolCallsSeenID map[string]int
}

func (ss *streamState) start() []ResponseStreamEvent {
	resp := ss.buildInProgressResponse()

	events := []ResponseStreamEvent{
		{
			Type:           "response.created",
			SequenceNumber: ss.seq,
			Response:       &resp,
		},
	}
	ss.seq++

	resp2 := ss.buildInProgressResponse()
	events = append(events, ResponseStreamEvent{
		Type:           "response.in_progress",
		SequenceNumber: ss.seq,
		Response:       &resp2,
	})
	ss.seq++

	return events
}

func (ss *streamState) process(chatResp model.ChatResponse) []ResponseStreamEvent {
	if len(chatResp.Choices) == 0 {
		return nil
	}

	choice := chatResp.Choices[0]

	var events []ResponseStreamEvent

	// When FinishReason is set, the text/reasoning content is duplicated from
	// previous chunks, so skip processing it. However, tool calls may only
	// arrive with the final response, so we must still process those.
	if choice.FinishReasonPtr == nil {
		if delta := choice.Delta.Reasoning; delta != "" {
			events = append(events, ss.handleReasoningDelta(delta)...)
		}

		if delta := choice.Delta.Content; delta != "" {
			events = append(events, ss.handleTextDelta(delta)...)
		}
	}

	if choice.Message != nil && len(choice.Message.ToolCalls) > 0 {
		events = append(events, ss.handleToolCalls(choice.Message.ToolCalls)...)
	}

	return events
}

func (ss *streamState) complete(lastResp model.ChatResponse) []ResponseStreamEvent {
	var events []ResponseStreamEvent

	if ss.msgItemEmitted {
		events = append(events, ss.finalizeMessageItem()...)
	}

	events = append(events, ss.finalizeToolCalls()...)

	finalResp := toChatResponseToResponses(lastResp, ss.d)
	finalResp.ID = ss.responseID
	finalResp.CreatedAt = ss.createdAt

	// Rebuild output items from streaming state so IDs and arguments
	// match what was already streamed to the client.
	var output []ResponseOutputItem
	if ss.msgItemEmitted {
		output = append(output, ResponseOutputItem{
			Type:   "message",
			ID:     ss.msgID,
			Status: "completed",
			Role:   model.RoleAssistant,
			Content: []ResponseContentItem{
				{Type: "output_text", Text: ss.fullText, Annotations: []string{}},
			},
		})
	}
	for i, fcItem := range ss.fcItems {
		fcItem.Status = "completed"
		a := ss.fcArgsAccum[i]
		fcItem.Arguments = &a
		output = append(output, fcItem)
	}
	finalResp.Output = output

	events = append(events, ResponseStreamEvent{
		Type:           "response.completed",
		SequenceNumber: ss.seq,
		Response:       &finalResp,
	})

	return events
}

func (ss *streamState) buildInProgressResponse() ResponseResponse {
	return ResponseResponse{
		ID:               ss.responseID,
		Object:           "response",
		CreatedAt:        ss.createdAt,
		Status:           "in_progress",
		Model:            ss.modelID,
		Output:           []ResponseOutputItem{},
		ParallelToolCall: ss.params.ParallelToolCalls,
		Reasoning:        ResponseReasoning{},
		Store:            ss.params.Store,
		Temperature:      ss.params.Temperature,
		Text:             ResponseTextFormat{Format: ResponseFormatType{Type: "text"}},
		ToolChoice:       ss.params.ToolChoice,
		Tools:            ss.tools,
		TopP:             ss.params.TopP,
		Truncation:       ss.params.Truncation,
		Usage:            ResponseUsage{},
		Metadata:         map[string]any{},
	}
}

func (ss *streamState) handleReasoningDelta(delta string) []ResponseStreamEvent {
	ss.fullReasoning += delta
	event := ResponseStreamEvent{
		Type:           "response.reasoning_summary_text.delta",
		SequenceNumber: ss.seq,
		Delta:          delta,
	}
	ss.seq++
	return []ResponseStreamEvent{event}
}

func (ss *streamState) handleTextDelta(delta string) []ResponseStreamEvent {
	var events []ResponseStreamEvent

	if !ss.msgItemEmitted {
		events = append(events, ss.emitMessageItemAdded()...)
	}

	ss.fullText += delta
	events = append(events, ResponseStreamEvent{
		Type:           "response.output_text.delta",
		SequenceNumber: ss.seq,
		ItemID:         ss.msgID,
		OutputIndex:    &ss.outputIndex,
		ContentIndex:   &ss.contentIndex,
		Delta:          delta,
	})
	ss.seq++

	return events
}

func (ss *streamState) emitMessageItemAdded() []ResponseStreamEvent {
	ss.msgID = "msg_" + uuid.New().String()
	ss.msgItemEmitted = true

	outputItem := ResponseOutputItem{
		Type:   "message",
		ID:     ss.msgID,
		Status: "in_progress",
		Role:   model.RoleAssistant,
		Content: []ResponseContentItem{
			{Type: "output_text", Text: "", Annotations: []string{}},
		},
	}

	events := []ResponseStreamEvent{
		{
			Type:           "response.output_item.added",
			SequenceNumber: ss.seq,
			OutputIndex:    &ss.outputIndex,
			Item:           &outputItem,
		},
	}
	ss.seq++

	contentPart := ResponseContentItem{Type: "output_text", Text: "", Annotations: []string{}}
	events = append(events, ResponseStreamEvent{
		Type:           "response.content_part.added",
		SequenceNumber: ss.seq,
		ItemID:         ss.msgID,
		OutputIndex:    &ss.outputIndex,
		ContentIndex:   &ss.contentIndex,
		Part:           &contentPart,
	})
	ss.seq++

	return events
}

func (ss *streamState) handleToolCalls(toolCalls []model.ResponseToolCall) []ResponseStreamEvent {
	if ss.toolCallsSeenID == nil {
		ss.toolCallsSeenID = make(map[string]int)
	}

	var events []ResponseStreamEvent

	for _, tc := range toolCalls {
		idx, seen := ss.toolCallsSeenID[tc.ID]
		if !seen {
			idx = len(ss.fcItems)
			ss.toolCallsSeenID[tc.ID] = idx

			if ss.msgItemEmitted {
				ss.outputIndex++
			}

			fcID := fmt.Sprintf("call_%s", uuid.New().String())
			ss.fcIDs = append(ss.fcIDs, fcID)
			ss.fcArgsAccum = append(ss.fcArgsAccum, "")

			emptyArgs := ""
			fcItem := ResponseOutputItem{
				Type:      "function_call",
				ID:        fcID,
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Status:    "in_progress",
				Arguments: &emptyArgs,
			}
			ss.fcItems = append(ss.fcItems, fcItem)

			outIdx := ss.outputIndex + idx
			events = append(events, ResponseStreamEvent{
				Type:           "response.output_item.added",
				SequenceNumber: ss.seq,
				OutputIndex:    &outIdx,
				Item:           &fcItem,
			})
			ss.seq++
		}

		args, _ := json.Marshal(map[string]any(tc.Function.Arguments))
		argsDelta := string(args)

		ss.fcArgsAccum[idx] = argsDelta

		outIdx := ss.outputIndex + idx
		events = append(events, ResponseStreamEvent{
			Type:           "response.function_call_arguments.delta",
			SequenceNumber: ss.seq,
			ItemID:         ss.fcIDs[idx],
			OutputIndex:    &outIdx,
			Delta:          argsDelta,
		})
		ss.seq++
	}

	return events
}

func (ss *streamState) finalizeMessageItem() []ResponseStreamEvent {
	events := []ResponseStreamEvent{
		{
			Type:           "response.output_text.done",
			SequenceNumber: ss.seq,
			ItemID:         ss.msgID,
			OutputIndex:    &ss.outputIndex,
			ContentIndex:   &ss.contentIndex,
			Text:           ss.fullText,
		},
	}
	ss.seq++

	contentPart := ResponseContentItem{Type: "output_text", Text: ss.fullText, Annotations: []string{}}
	events = append(events, ResponseStreamEvent{
		Type:           "response.content_part.done",
		SequenceNumber: ss.seq,
		ItemID:         ss.msgID,
		OutputIndex:    &ss.outputIndex,
		ContentIndex:   &ss.contentIndex,
		Part:           &contentPart,
	})
	ss.seq++

	outputItem := ResponseOutputItem{
		Type:   "message",
		ID:     ss.msgID,
		Status: "completed",
		Role:   model.RoleAssistant,
		Content: []ResponseContentItem{
			{Type: "output_text", Text: ss.fullText, Annotations: []string{}},
		},
	}
	events = append(events, ResponseStreamEvent{
		Type:           "response.output_item.done",
		SequenceNumber: ss.seq,
		OutputIndex:    &ss.outputIndex,
		Item:           &outputItem,
	})
	ss.seq++

	return events
}

func (ss *streamState) finalizeToolCalls() []ResponseStreamEvent {
	var events []ResponseStreamEvent

	for i, fcItem := range ss.fcItems {
		outIdx := ss.outputIndex + i
		if ss.msgItemEmitted {
			outIdx = ss.outputIndex + i + 1
		}

		events = append(events, ResponseStreamEvent{
			Type:           "response.function_call_arguments.done",
			SequenceNumber: ss.seq,
			ItemID:         ss.fcIDs[i],
			OutputIndex:    &outIdx,
			Name:           fcItem.Name,
			Arguments:      ss.fcArgsAccum[i],
		})
		ss.seq++

		fcItem.Status = "completed"
		a := ss.fcArgsAccum[i]
		fcItem.Arguments = &a
		events = append(events, ResponseStreamEvent{
			Type:           "response.output_item.done",
			SequenceNumber: ss.seq,
			OutputIndex:    &outIdx,
			Item:           &fcItem,
		})
		ss.seq++
	}

	return events
}

// =============================================================================

func toChatResponseToResponses(chatResp model.ChatResponse, d model.D) ResponseResponse {
	now := time.Now().Unix()

	var outputText string
	var finishReason string
	var toolCalls []model.ResponseToolCall
	var reasoning string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		msg := choice.Message
		if msg == nil && choice.Delta != nil {
			msg = choice.Delta
		}
		if msg != nil {
			outputText = msg.Content
			toolCalls = msg.ToolCalls
			reasoning = msg.Reasoning
		}
		if choice.FinishReasonPtr != nil {
			finishReason = *choice.FinishReasonPtr
		}
	}

	status := "completed"
	var respError *ResponseError
	if finishReason == model.FinishReasonError {
		status = "failed"
		respError = &ResponseError{
			Code:    "error",
			Message: outputText,
		}
		outputText = ""
	}

	var completedAt *int64
	if status == "completed" {
		completedAt = &now
	}

	outputItems := buildOutputItems(outputText, toolCalls, status)

	var reasoningSummary *string
	if reasoning != "" {
		reasoningSummary = &reasoning
	}

	tools := extractTools(d)
	inputParams := extractInputParams(d)

	return ResponseResponse{
		ID:               "resp_" + chatResp.ID,
		Object:           "response",
		CreatedAt:        chatResp.Created / 1000,
		Status:           status,
		CompletedAt:      completedAt,
		Error:            respError,
		IncompleteDetail: nil,
		Instructions:     inputParams.Instructions,
		MaxOutputTokens:  inputParams.MaxOutputTokens,
		Model:            chatResp.Model,
		Output:           outputItems,
		ParallelToolCall: inputParams.ParallelToolCalls,
		PrevResponseID:   nil,
		Reasoning: ResponseReasoning{
			Effort:  nil,
			Summary: reasoningSummary,
		},
		Store:       inputParams.Store,
		Temperature: inputParams.Temperature,
		Text: ResponseTextFormat{
			Format: ResponseFormatType{
				Type: "text",
			},
		},
		ToolChoice: inputParams.ToolChoice,
		Tools:      tools,
		TopP:       inputParams.TopP,
		Truncation: inputParams.Truncation,
		Usage: ResponseUsage{
			InputTokens: chatResp.Usage.PromptTokens,
			InputTokensDetails: InputTokensDetails{
				CachedTokens: 0,
			},
			OutputTokens: chatResp.Usage.CompletionTokens,
			OutputTokenDetail: OutputTokensDetails{
				ReasoningTokens: chatResp.Usage.ReasoningTokens,
			},
			TotalTokens: chatResp.Usage.TotalTokens,
		},
		User:     nil,
		Metadata: map[string]any{},
	}
}

func buildOutputItems(outputText string, toolCalls []model.ResponseToolCall, status string) []ResponseOutputItem {
	var outputItems []ResponseOutputItem

	if len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			// Marshal the underlying map directly to avoid the custom
			// MarshalJSON on ToolCallArguments which double-encodes
			// (wraps in an extra JSON string layer for Chat Completions).
			args, _ := json.Marshal(map[string]any(tc.Function.Arguments))

			argsStr := string(args)
			outputItems = append(outputItems, ResponseOutputItem{
				Type:      "function_call",
				ID:        fmt.Sprintf("call_%s", uuid.New().String()),
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: &argsStr,
				Status:    "completed",
			})
		}

		return outputItems
	}

	if outputText != "" || status == "completed" {
		outputItems = append(outputItems, ResponseOutputItem{
			Type:   "message",
			ID:     "msg_" + uuid.New().String(),
			Status: "completed",
			Role:   model.RoleAssistant,
			Content: []ResponseContentItem{
				{
					Type:        "output_text",
					Text:        outputText,
					Annotations: []string{},
				},
			},
		})
	}

	return outputItems
}

// =============================================================================

type inputParams struct {
	Temperature       float64
	TopP              float64
	ToolChoice        string
	Truncation        string
	MaxOutputTokens   *int
	ParallelToolCalls bool
	Store             bool
	Instructions      *string
}

func extractInputParams(d model.D) inputParams {
	params := inputParams{
		Temperature:       1.0,
		TopP:              1.0,
		ToolChoice:        "auto",
		Truncation:        "disabled",
		ParallelToolCalls: true,
		Store:             true,
	}

	if v, ok := d["temperature"].(float64); ok {
		params.Temperature = v
	}

	if v, ok := d["top_p"].(float64); ok {
		params.TopP = v
	}

	if v, ok := d["tool_choice"].(string); ok {
		params.ToolChoice = v
	}

	if v, ok := d["truncation"].(string); ok {
		params.Truncation = v
	}

	if v, ok := d["max_tokens"].(int); ok {
		params.MaxOutputTokens = &v
	}

	if v, ok := d["parallel_tool_calls"].(bool); ok {
		params.ParallelToolCalls = v
	}

	if v, ok := d["store"].(bool); ok {
		params.Store = v
	}

	if v, ok := d["instructions"].(string); ok {
		params.Instructions = &v
	}

	return params
}

func extractTools(d model.D) []any {
	toolsVal, exists := d["tools"]
	if !exists {
		return []any{}
	}

	tools, ok := toolsVal.([]model.D)
	if !ok {
		return []any{}
	}

	result := make([]any, len(tools))
	for i, t := range tools {
		result[i] = t
	}

	return result
}

func convertInputToMessages(d model.D) model.D {
	if _, hasMessages := d["messages"]; !hasMessages {
		input, hasInput := d["input"]
		if !hasInput {
			return d
		}

		d["messages"] = inputToMessages(input)
		delete(d, "input")
	}

	normalizeResponsesItems(d)
	normalizeResponsesContent(d)
	normalizeTools(d)
	injectInstructions(d)

	return d
}

// normalizeResponsesItems converts Open Responses item types (function_call,
// function_call_output) in the messages array to Chat Completions messages.
// Consecutive function_call items are grouped into a single assistant message
// with multiple tool_calls.
func normalizeResponsesItems(d model.D) {
	messages, ok := d["messages"].([]model.D)
	if !ok {
		return
	}

	var result []model.D
	var modified bool

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		typ, _ := msg["type"].(string)

		switch typ {
		case "function_call":
			modified = true

			// Group consecutive function_call items into one assistant
			// message with multiple tool_calls.
			var toolCalls []model.D
			for i < len(messages) {
				fc := messages[i]
				ft, _ := fc["type"].(string)
				if ft != "function_call" {
					break
				}

				callID, _ := fc["call_id"].(string)
				name, _ := fc["name"].(string)
				args, _ := fc["arguments"].(string)

				toolCalls = append(toolCalls, model.D{
					"id":   callID,
					"type": "function",
					"function": model.D{
						"name":      name,
						"arguments": args,
					},
				})
				i++
			}
			i-- // outer loop will increment

			result = append(result, model.D{
				"role":       "assistant",
				"tool_calls": toolCalls,
			})

		case "function_call_output":
			modified = true

			callID, _ := msg["call_id"].(string)
			output, _ := msg["output"].(string)

			result = append(result, model.D{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      output,
			})

		default:
			result = append(result, msg)
		}
	}

	if modified {
		d["messages"] = result
	}
}

// normalizeResponsesContent converts Responses API content types to Chat
// Completions format within existing messages. This handles clients that send
// "messages" with Responses API content parts (e.g. "input_text" instead of
// "text", or "output_text" from previous assistant turns).
func normalizeResponsesContent(d model.D) {
	messages, ok := d["messages"].([]model.D)
	if !ok {
		return
	}

	for i, msg := range messages {
		content, ok := msg["content"].([]model.D)
		if !ok {
			continue
		}

		var modified bool
		for j, part := range content {
			typ, _ := part["type"].(string)

			switch typ {
			case "input_text", "output_text":
				if !modified {
					newContent := make([]model.D, len(content))
					copy(newContent, content)
					content = newContent
					modified = true
				}
				content[j] = model.D{
					"type": "text",
					"text": part["text"],
				}
			}
		}

		if modified {
			newMsg := msg.ShallowClone()
			newMsg["content"] = content
			messages[i] = newMsg
		}
	}
}

// normalizeTools converts Open Responses tool definitions to Chat Completions
// format. Open Responses uses a flat structure with name/description/parameters
// at the top level, while Chat Completions nests them under a "function" key.
func normalizeTools(d model.D) {
	tools, ok := d["tools"].([]model.D)
	if !ok {
		return
	}

	var modified bool
	for i, tool := range tools {
		if tool["type"] != "function" {
			continue
		}

		// Already in Chat Completions format.
		if _, hasFunc := tool["function"]; hasFunc {
			continue
		}

		// Flat Open Responses format: wrap name/description/parameters
		// into a "function" sub-object.
		name, _ := tool["name"].(string)
		if name == "" {
			continue
		}

		if !modified {
			newTools := make([]model.D, len(tools))
			copy(newTools, tools)
			tools = newTools
			modified = true
		}

		fn := model.D{
			"name": name,
		}

		if desc, ok := tool["description"].(string); ok {
			fn["description"] = desc
		}

		if params, ok := tool["parameters"]; ok {
			fn["parameters"] = params
		}

		if strict, ok := tool["strict"]; ok {
			fn["strict"] = strict
		}

		tools[i] = model.D{
			"type":     "function",
			"function": fn,
		}
	}

	if modified {
		d["tools"] = tools
	}
}

// injectInstructions converts the Responses API "instructions" field into a
// system message prepended to the messages slice.
func injectInstructions(d model.D) {
	instructions, ok := d["instructions"].(string)
	if !ok || instructions == "" {
		return
	}

	messages, ok := d["messages"].([]model.D)
	if !ok {
		return
	}

	systemMsg := model.D{
		"role":    "system",
		"content": instructions,
	}

	d["messages"] = append([]model.D{systemMsg}, messages...)
	delete(d, "instructions")
}

func inputToMessages(input any) []model.D {
	inputItems, ok := input.([]any)
	if !ok {
		if str, ok := input.(string); ok {
			return []model.D{
				{"role": "user", "content": str},
			}
		}
		if docs, ok := input.([]model.D); ok {
			return docs
		}
		return nil
	}

	if len(inputItems) == 0 {
		return nil
	}

	firstItem, ok := inputItems[0].(map[string]any)
	if !ok {
		firstDoc, ok := inputItems[0].(model.D)
		switch ok {
		case true:
			firstItem = firstDoc
		case false:
			return nil
		}
	}

	if _, hasRole := firstItem["role"]; hasRole {
		var messages []model.D
		for _, item := range inputItems {
			switch v := item.(type) {
			case map[string]any:
				messages = append(messages, model.D(v))
			case model.D:
				messages = append(messages, v)
			}
		}
		return messages
	}

	var content []model.D
	for _, item := range inputItems {
		var itemMap map[string]any

		switch v := item.(type) {
		case map[string]any:
			itemMap = v
		case model.D:
			itemMap = v
		default:
			continue
		}

		switch itemMap["type"] {
		case "input_text":
			content = append(content, model.D{
				"type": "text",
				"text": itemMap["text"],
			})
		case "input_image":
			content = append(content, model.D{
				"type": "image_url",
				"image_url": model.D{
					"url": itemMap["image_url"],
				},
			})
		}
	}

	return []model.D{
		{"role": "user", "content": content},
	}
}
