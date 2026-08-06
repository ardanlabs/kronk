package kronk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// ErrResponseCommitted indicates that an HTTP streaming response was already
// committed before the operation failed.
var ErrResponseCommitted = errors.New("response already committed")

// Chat provides support to interact with an inference model.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error) {
	if err := model.ValidateChatRequest(d); err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat: %w", err)
	}

	f := func(m *model.Model) (model.ChatResponse, error) {
		return m.Chat(ctx, d)
	}

	return nonStreaming(ctx, krn, f)
}

// ChatStreaming provides support to interact with an inference model.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (<-chan model.ChatResponse, error) {
	if err := model.ValidateChatRequest(d); err != nil {
		return nil, fmt.Errorf("chat-streaming: %w", err)
	}

	f := func(m *model.Model) <-chan model.ChatResponse {
		return m.ChatStreaming(ctx, d)
	}

	ef := func(err error) model.ChatResponse {
		return model.ChatResponseErr("panic", model.ObjectChatUnknown, krn.ModelInfo().ID, 0, err, model.Usage{})
	}

	return streaming(ctx, krn, f, ef)
}

// ChatStreamingHTTP provides http handler support for a chat/completions call.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error) {
	// [DEBUG]: Show raw input content.
	// fmt.Printf("[DEBUG]: {\"req\":%s}\n", debugChatRequest(d))

	var stream bool
	streamReq, ok := d["stream"].(bool)
	if ok {
		stream = streamReq
	}
	includeUsage := streamIncludeUsage(d)

	// -------------------------------------------------------------------------

	if !stream {
		resp, err := krn.Chat(ctx, d)
		if err != nil {
			return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: stream-response: %w", err)
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return resp, fmt.Errorf("chat-streaming-http: marshal: %w", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			return resp, fmt.Errorf("chat-streaming-http: %w: write response: %w", ErrResponseCommitted, err)
		}

		return resp, nil
	}

	// -------------------------------------------------------------------------

	if !supportsResponseFlush(w) {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: streaming not supported")
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: stream-response: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	if err := http.NewResponseController(w).Flush(); err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: %w: flush headers: %w", ErrResponseCommitted, err)
	}

	// Every 15 seconds we will send a SSE keep alive for responses
	// that are taking a long time to process. We won't reset this
	// in the processing loop to eliminate overhead.
	const keepAliveInterval = 15 * time.Second
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	var lr model.ChatResponse

	for {
		select {
		case <-ctx.Done():
			return lr, fmt.Errorf("chat-streaming-http: %w: %w", ErrResponseCommitted, ctx.Err())

		case resp, ok := <-ch:
			if !ok {
				if err := writeAndFlush(w, []byte("data: [DONE]\n\n")); err != nil {
					return lr, fmt.Errorf("chat-streaming-http: %w: write done event: %w", ErrResponseCommitted, err)
				}
				return lr, nil
			}

			if resp.Choices[0].FinishReason() == model.FinishReasonError {
				d, err := marshalChatStreamError(resp)
				if err != nil {
					return resp, fmt.Errorf("chat-streaming-http: %w: marshal error: %w", ErrResponseCommitted, err)
				}

				if err := writeAndFlush(w, fmt.Appendf(nil, "data: %s\n\n", d)); err != nil {
					return resp, fmt.Errorf("chat-streaming-http: %w: write error event: %w", ErrResponseCommitted, err)
				}
				return resp, nil
			}

			// OpenAI does not expect the final chunk to have a message field.
			// The terminal delta is empty; tool-call arguments arrive in the
			// preceding nonterminal chunk.
			if fr := resp.Choices[0].FinishReason(); fr == model.FinishReasonStop || fr == model.FinishReasonLength || fr == model.FinishReasonTool {
				resp.Choices[0].Message = nil
				resp.Choices[0].Delta = &model.ResponseMessage{}
			}

			wireResp := resp
			if !includeUsage {
				wireResp.Usage = nil
			}

			d, err := json.Marshal(wireResp)
			if err != nil {
				return resp, fmt.Errorf("chat-streaming-http: %w: marshal: %w", ErrResponseCommitted, err)
			}

			// [DEBUG]: Show raw output content.
			// fmt.Printf("[DEBUG]: {\"resp\":%q}", string(d))

			if err := writeAndFlush(w, fmt.Appendf(nil, "data: %s\n\n", d)); err != nil {
				return resp, fmt.Errorf("chat-streaming-http: %w: write event: %w", ErrResponseCommitted, err)
			}

			lr = resp

		case <-ticker.C:
			if krn.cfg.Log != nil {
				krn.cfg.Log(ctx, "chat-streaming-http", "status", "keep-alive sent")
			}

			if err := writeAndFlush(w, []byte(": keep-alive\n\n")); err != nil {
				return lr, fmt.Errorf("chat-streaming-http: %w: write keep-alive: %w", ErrResponseCommitted, err)
			}
		}
	}
}

func writeAndFlush(w http.ResponseWriter, data []byte) error {
	if _, err := w.Write(data); err != nil {
		return err
	}

	return http.NewResponseController(w).Flush()
}

func supportsResponseFlush(w http.ResponseWriter) bool {
	for w != nil {
		switch v := w.(type) {
		case interface{ FlushError() error }:
			return true
		case http.Flusher:
			return true
		case interface{ Unwrap() http.ResponseWriter }:
			w = v.Unwrap()
		default:
			return false
		}
	}

	return false
}

func marshalChatStreamError(resp model.ChatResponse) ([]byte, error) {
	choice := resp.Choices[0]

	var message string
	if choice.Delta != nil {
		message = choice.Delta.Content
	}
	if message == "" && choice.Message != nil {
		message = choice.Message.Content
	}

	wireResp := model.D{
		"error": model.D{
			"message": message,
			"type":    "server_error",
			"code":    "server_error",
		},
	}

	return json.Marshal(wireResp)
}

func streamIncludeUsage(d model.D) bool {
	streamOpts, exists := d["stream_options"]
	if !exists {
		return true
	}

	var optsMap model.D
	switch opts := streamOpts.(type) {
	case model.D:
		optsMap = opts
	case map[string]any:
		optsMap = model.D(opts)
	}

	includeUsage, exists := optsMap["include_usage"].(bool)
	if !exists {
		return true
	}

	return includeUsage
}

// func debugChatRequest(d model.D) string {
// 	d = d.Clone()

// 	messages, _ := d["messages"].([]model.D)
// 	for _, message := range messages {
// 		content, _ := message["content"].([]model.D)
// 		for _, part := range content {
// 			imageURL, _ := part["image_url"].(model.D)
// 			url, _ := imageURL["url"].(string)
// 			if url != "" {
// 				imageURL["url"] = fmt.Sprintf("[omitted image data: %d bytes]", len(url))
// 			}
// 		}
// 	}

// 	b, err := json.Marshal(d)
// 	if err != nil {
// 		return fmt.Sprintf("[unable to marshal request: %v]", err)
// 	}

// 	return string(b)
// }
