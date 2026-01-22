package kronk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Chat provides support to interact with an inference model.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.ChatResponse{}, fmt.Errorf("chat: context has no deadline, provide a reasonable timeout")
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
	if _, exists := ctx.Deadline(); !exists {
		return nil, fmt.Errorf("chat-streaming: context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) <-chan model.ChatResponse {
		return m.ChatStreaming(ctx, d)
	}

	ef := func(err error) model.ChatResponse {
		return model.ChatResponseErr("panic", model.ObjectChatUnknown, krn.ModelInfo().ID, 0, "", err, model.Usage{})
	}

	return streaming(ctx, krn, f, ef)
}

// ChatStreamingHTTP provides http handler support for a chat/completions call.
// For text models, NSeqMax controls parallel sequence processing within a single
// model instance. For vision/audio models, NSeqMax creates multiple model
// instances in a pool for concurrent request handling.
func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: context has no deadline, provide a reasonable timeout")
	}

	var stream bool
	streamReq, ok := d["stream"].(bool)
	if ok {
		stream = streamReq
	}

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
		w.Write(data)

		return resp, nil
	}

	// -------------------------------------------------------------------------

	f, ok := w.(http.Flusher)
	if !ok {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: streaming not supported")
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http: stream-response: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	var lr model.ChatResponse

	for resp := range ch {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return resp, errors.New("chat-streaming-http: client disconnected, do not send response")
			}
		}

		// OpenAI does not expect the final chunk to have a message field.
		// The delta should be empty {} per OpenAI spec (except for tool calls).
		if fr := resp.Choice[0].FinishReason(); fr == model.FinishReasonStop || fr == model.FinishReasonTool {
			resp.Choice[0].Message = nil
			if delta := resp.Choice[0].Delta; delta != nil {
				if len(delta.ToolCalls) == 0 {
					resp.Choice[0].Delta = &model.ResponseMessage{}
				} else {
					delta.Role = ""
					delta.Content = ""
					delta.Reasoning = ""
				}
			}
		}

		d, err := json.Marshal(resp)
		if err != nil {
			return resp, fmt.Errorf("chat-streaming-http: marshal: %w", err)
		}

		// DEBUG: Show raw reponses
		// fmt.Println("[DEBUG]: string(d))

		fmt.Fprintf(w, "data: %s\n\n", d)
		f.Flush()

		lr = resp
	}

	w.Write([]byte("data: [DONE]\n\n"))
	f.Flush()

	return lr, nil
}
