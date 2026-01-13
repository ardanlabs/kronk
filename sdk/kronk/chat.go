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
func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.ChatResponse{}, fmt.Errorf("chat:context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.ChatResponse, error) {
		return m.Chat(ctx, d)
	}

	return nonStreaming(ctx, krn, f)
}

// ChatStreaming provides support to interact with an inference model.
func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (<-chan model.ChatResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return nil, fmt.Errorf("chat-streaming:context has no deadline, provide a reasonable timeout")
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
func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http:context has no deadline, provide a reasonable timeout")
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
			return model.ChatResponse{}, fmt.Errorf("chat-streaming-http:stream-response: %w", err)
		}

		data, err := json.Marshal(resp)
		if err != nil {
			return resp, fmt.Errorf("chat-streaming-http:marshal: %w", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)

		return resp, nil
	}

	// -------------------------------------------------------------------------

	f, ok := w.(http.Flusher)
	if !ok {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http:streaming not supported")
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("chat-streaming-http:stream-response: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	var lr model.ChatResponse

	for resp := range ch {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return resp, errors.New("chat-streaming-http:client disconnected, do not send response")
			}
		}

		// OpenAI does not expect the final delta to have content or reasoning.
		// Kronk returns the entire streamed content in the final chunk.
		if resp.Choice[0].FinishReason == model.FinishReasonStop {
			resp.Choice[0].Message = model.ResponseMessage{}
		}

		d, err := json.Marshal(resp)
		if err != nil {
			return resp, fmt.Errorf("chat-streaming-http:marshal: %w", err)
		}

		fmt.Fprintf(w, "data: %s\n", d)
		f.Flush()

		lr = resp
	}

	w.Write([]byte("data: [DONE]\n"))
	f.Flush()

	return lr, nil
}
