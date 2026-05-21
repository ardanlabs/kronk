// Package msgsapp provides the Anthropic Messages API endpoints.
package msgsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

type app struct {
	log  *logger.Logger
	pool *pool.Pool
}

func newApp(cfg Config) *app {
	return &app{
		log:  cfg.Log,
		pool: cfg.Pool,
	}
}

func (a *app) messages(ctx context.Context, r *http.Request) web.Encoder {
	var req MessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if req.Model == "" {
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	}

	if req.MaxTokens == 0 {
		return errs.Errorf(errs.InvalidArgument, "missing max_tokens field")
	}

	krn, err := a.pool.AquireModel(ctx, req.Model)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	a.log.Info(ctx, "messages", "model", req.Model)

	ctx, cancel := context.WithTimeout(ctx, 180*time.Minute)
	defer cancel()

	d := toOpenAI(req)

	if req.Stream {
		if err := a.handleStreaming(ctx, krn, d, req.Model); err != nil {
			return errs.New(errs.Internal, err)
		}

		return web.NewNoResponse()
	}

	resp, err := krn.Chat(ctx, d)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	// Set anthropic-request-id header for API compatibility
	w := web.GetWriter(ctx)
	if w != nil {
		w.Header().Set("anthropic-request-id", resp.ID)
	}

	return toMessagesResponse(resp)
}

func (a *app) handleStreaming(ctx context.Context, krn *kronk.Kronk, d model.D, modelName string) error {
	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	state := streamState{
		w:         w,
		f:         f,
		modelName: modelName,
	}

	for resp := range ch {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("client disconnected")
		}

		// Set anthropic-request-id header from first response
		if !state.started && resp.ID != "" {
			w.Header().Set("anthropic-request-id", resp.ID)
			w.WriteHeader(http.StatusOK)
			f.Flush()
		}

		if err := state.processChunk(resp); err != nil {
			return err
		}
	}

	return state.finish()
}

// =============================================================================

type streamState struct {
	w            http.ResponseWriter
	f            http.Flusher
	modelName    string
	messageID    string
	started      bool
	blockStarted bool
	blockIndex   int
	inputTokens  int
	outputTokens int
	finishReason string
}

func (s *streamState) processChunk(resp model.ChatResponse) error {
	if !s.started {
		s.messageID = resp.ID

		if err := s.sendMessageStart(resp); err != nil {
			return err
		}

		s.started = true
	}

	if len(resp.Choices) == 0 {
		return nil
	}

	choice := resp.Choices[0]

	// Skip delta content on final chunk (FinishReason set) - it duplicates previous content
	if choice.FinishReason() == "" && choice.Delta != nil && choice.Delta.Content != "" {
		if !s.blockStarted {
			if err := s.sendContentBlockStart("text", "", ""); err != nil {
				return err
			}
			s.blockStarted = true
		}

		if err := s.sendTextDelta(choice.Delta.Content); err != nil {
			return err
		}
	}

	if choice.Delta != nil && len(choice.Delta.ToolCalls) > 0 {
		for _, tc := range choice.Delta.ToolCalls {
			if s.blockStarted {
				if err := s.sendContentBlockStop(); err != nil {
					return err
				}

				s.blockIndex++
				s.blockStarted = false
			}

			if err := s.sendContentBlockStart("tool_use", tc.ID, tc.Function.Name); err != nil {
				return err
			}

			s.blockStarted = true

			// Marshal the underlying map directly to avoid double-encoding.
			// ToolCallArguments.MarshalJSON() wraps as JSON string per OpenAI spec,
			// but Anthropic expects raw JSON object in partial_json field.
			args, err := json.Marshal(map[string]any(tc.Function.Arguments))
			if err != nil {
				return err
			}

			if err := s.sendInputJSONDelta(string(args)); err != nil {
				return err
			}
		}
	}

	// Usage is only populated on the final chunk produced by chatResponseFinal.
	// Capture both PromptTokens (cache-inclusive input count from nPrompt =
	// cacheIdx + suffixTokens) and CompletionTokens so the closing
	// message_delta event can report accurate cumulative usage. Without this,
	// only message_start carried input_tokens — and on the first chunk
	// resp.Usage is nil, so input_tokens was always reported as 0.
	if resp.Usage != nil {
		s.inputTokens = resp.Usage.PromptTokens
		s.outputTokens = resp.Usage.CompletionTokens
	}

	// Capture the model's finish reason from the final chunk so finish() can
	// emit the correct Anthropic stop_reason instead of hard-coding "end_turn"
	// for every request (which masked tool_use completions).
	if fr := choice.FinishReason(); fr != "" {
		s.finishReason = fr
	}

	return nil
}

func (s *streamState) finish() error {
	if s.blockStarted {
		if err := s.sendContentBlockStop(); err != nil {
			return err
		}
	}

	if err := s.sendMessageDelta(toAnthropicStopReason(s.finishReason)); err != nil {
		return err
	}

	return s.sendMessageStop()
}

// toAnthropicStopReason maps a model.FinishReason* value to the Anthropic
// stop_reason string. Mirrors the mapping used by toMessagesResponse for the
// non-streaming path so streaming and non-streaming agree.
func toAnthropicStopReason(finishReason string) string {
	switch finishReason {
	case model.FinishReasonTool:
		return "tool_use"
	case model.FinishReasonStop:
		return "end_turn"
	case "":
		// No finish reason ever observed (e.g., client disconnect). Default
		// to end_turn so we still emit a syntactically valid message_delta.
		return "end_turn"
	default:
		return finishReason
	}
}

func (s *streamState) sendEvent(eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// fmt.Println("================= EVENT ===================")
	// fmt.Printf(`[DEBUG]: {"debug_request": %q}`+"\n", string(jsonData))
	// fmt.Println("================= EVENT ===================")

	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	s.f.Flush()

	return nil
}

func (s *streamState) sendMessageStart(resp model.ChatResponse) error {
	// resp here is the first chunk produced by chatResponseDelta, which does
	// not populate Usage — PromptTokens is only known after the model layer
	// runs startSlot* and is currently only emitted on the final chunk via
	// chatResponseFinal. As a result message_start.usage.input_tokens is
	// reported as 0. The real cache-inclusive count is surfaced later in the
	// closing message_delta event (see sendMessageDelta + processChunk).
	//
	// Clients that read input_tokens from message_delta (the cumulative
	// usage) will see the correct value; clients that read only from
	// message_start will see 0. Fully fixing message_start.input_tokens
	// requires the SDK to populate Usage{PromptTokens: nPrompt} on the
	// first streamed delta, since nPrompt is known in startSlot before
	// generation begins.
	var inputTokens int
	if resp.Usage != nil {
		inputTokens = resp.Usage.PromptTokens
	}

	event := MessageStartEvent{
		Type: "message_start",
		Message: MessageStartMetadata{
			ID:           resp.ID,
			Type:         "message",
			Role:         "assistant",
			Content:      []ResponseContentBlock{},
			Model:        s.modelName,
			StopReason:   nil,
			StopSequence: nil,
			Usage: Usage{
				InputTokens:  inputTokens,
				OutputTokens: 0,
			},
		},
	}

	return s.sendEvent("message_start", event)
}

func (s *streamState) sendContentBlockStart(blockType, toolID, toolName string) error {
	event := ContentBlockStartEvent{
		Type:  "content_block_start",
		Index: s.blockIndex,
		ContentBlock: ContentBlockMetadata{
			Type: blockType,
		},
	}

	switch blockType {
	case "text":
		event.ContentBlock.Text = ""

	case "tool_use":
		event.ContentBlock.ID = toolID
		event.ContentBlock.Name = toolName
		event.ContentBlock.Input = map[string]any{}
	}

	return s.sendEvent("content_block_start", event)
}

func (s *streamState) sendTextDelta(text string) error {
	event := ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: s.blockIndex,
		Delta: ContentDelta{
			Type: "text_delta",
			Text: text,
		},
	}

	return s.sendEvent("content_block_delta", event)
}

func (s *streamState) sendInputJSONDelta(partialJSON string) error {
	event := ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: s.blockIndex,
		Delta: ContentDelta{
			Type:        "input_json_delta",
			PartialJSON: partialJSON,
		},
	}

	return s.sendEvent("content_block_delta", event)
}

func (s *streamState) sendContentBlockStop() error {
	event := ContentBlockStopEvent{
		Type:  "content_block_stop",
		Index: s.blockIndex,
	}

	return s.sendEvent("content_block_stop", event)
}

func (s *streamState) sendMessageDelta(stopReason string) error {
	event := MessageDeltaEvent{
		Type: "message_delta",
		Delta: MessageDelta{
			StopReason: stopReason,
		},
		Usage: DeltaUsage{
			InputTokens:  s.inputTokens,
			OutputTokens: s.outputTokens,
		},
	}

	return s.sendEvent("message_delta", event)
}

func (s *streamState) sendMessageStop() error {
	event := MessageStopEvent{
		Type: "message_stop",
	}

	return s.sendEvent("message_stop", event)
}
