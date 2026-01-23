// Package msgsapp provides the Anthropic Messages API endpoints.
package msgsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type app struct {
	log   *logger.Logger
	cache *cache.Cache
}

func newApp(cfg Config) *app {
	return &app{
		log:   cfg.Log,
		cache: cfg.Cache,
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

	req.Model = "Qwen3-Coder-30B-A3B-Instruct-Q8_0"

	krn, err := a.cache.AquireModel(ctx, req.Model)
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
	w.WriteHeader(http.StatusOK)
	f.Flush()

	state := streamState{
		w:         w,
		f:         f,
		modelName: modelName,
	}

	for resp := range ch {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("client disconnected")
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
	outputTokens int
}

func (s *streamState) processChunk(resp model.ChatResponse) error {
	if !s.started {
		s.messageID = resp.ID

		if err := s.sendMessageStart(resp); err != nil {
			return err
		}

		s.started = true
	}

	if len(resp.Choice) == 0 {
		return nil
	}

	choice := resp.Choice[0]

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

	if resp.Usage != nil {
		s.outputTokens = resp.Usage.CompletionTokens
	}

	return nil
}

func (s *streamState) finish() error {
	if s.blockStarted {
		if err := s.sendContentBlockStop(); err != nil {
			return err
		}
	}

	stopReason := "end_turn"

	if err := s.sendMessageDelta(stopReason); err != nil {
		return err
	}

	return s.sendMessageStop()
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
