// Package kronk provides support for working with models using llama.cpp via yzma.
package kronk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Version contains the current version of the kronk package.
const Version = "1.9.4"

// =============================================================================

type options struct {
	tr model.TemplateRetriever
}

// Option represents a functional option for configuring Kronk.
type Option func(*options)

// WithTemplateRetriever sets a custom Github repo for templates.
// If not set, the default repo will be used.
func WithTemplateRetriever(templates model.TemplateRetriever) Option {
	return func(o *options) {
		o.tr = templates
	}
}

// =============================================================================

// Kronk provides a concurrently safe api for using llama.cpp to access models.
type Kronk struct {
	cfg           model.Config
	models        chan *model.Model
	activeStreams atomic.Int32
	shutdown      sync.Mutex
	shutdownFlag  bool
	modelInfo     model.ModelInfo
}

// New provides the ability to use models in a concurrently safe way.
//
// modelInstances represents the number of instances of the model to create. Unless
// you have more than 1 GPU, the recommended number of instances is 1.
func New(modelInstances int, cfg model.Config, opts ...Option) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("the Init() function has not been called")
	}

	if modelInstances <= 0 {
		return nil, fmt.Errorf("instances must be > 0, got %d", modelInstances)
	}

	// -------------------------------------------------------------------------

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if o.tr == nil {
		templates, err := templates.New()
		if err != nil {
			return nil, fmt.Errorf("template new: %w", err)
		}

		o.tr = templates
	}

	// -------------------------------------------------------------------------

	models := make(chan *model.Model, modelInstances)
	var firstModel *model.Model

	for range modelInstances {
		m, err := model.NewModel(o.tr, cfg)
		if err != nil {
			close(models)
			for model := range models {
				model.Unload(context.Background())
			}

			return nil, err
		}

		models <- m

		if firstModel == nil {
			firstModel = m
		}
	}

	krn := Kronk{
		cfg:       firstModel.Config(),
		models:    models,
		modelInfo: firstModel.ModelInfo(),
	}

	return &krn, nil
}

// ModelConfig returns a copy of the configuration being used. This may be
// different from the configuration passed to New() if the model has
// overridden any of the settings.
func (krn *Kronk) ModelConfig() model.Config {
	return krn.cfg
}

// SystemInfo returns system information.
func (krn *Kronk) SystemInfo() map[string]string {
	result := make(map[string]string)

	for part := range strings.SplitSeq(llama.PrintSystemInfo(), "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Remove the "= 1" or similar suffix
		if idx := strings.Index(part, "="); idx != -1 {
			part = strings.TrimSpace(part[:idx])
		}

		// Check for "Key : Value" pattern
		switch kv := strings.SplitN(part, ":", 2); len(kv) {
		case 2:
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			result[key] = value
		default:
			result[part] = "on"
		}
	}

	return result
}

// ModelInfo returns the model information.
func (krn *Kronk) ModelInfo() model.ModelInfo {
	return krn.modelInfo
}

// ActiveStreams returns the number of active streams.
func (krn *Kronk) ActiveStreams() int {
	return int(krn.activeStreams.Load())
}

// Unload will close down all loaded models. You should call this only when you
// are completely done using the group.
func (krn *Kronk) Unload(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// -------------------------------------------------------------------------

	err := func() error {
		krn.shutdown.Lock()
		defer krn.shutdown.Unlock()

		if krn.shutdownFlag {
			return fmt.Errorf("unload:already unloaded")
		}

		for krn.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload:cannot unload: %d active streams: %w", krn.activeStreams.Load(), ctx.Err())

			case <-time.After(100 * time.Millisecond):
			}
		}

		krn.shutdownFlag = true
		return nil
	}()

	if err != nil {
		return err
	}

	// -------------------------------------------------------------------------

	var sb strings.Builder

	close(krn.models)
	for model := range krn.models {
		if err := model.Unload(ctx); err != nil {
			sb.WriteString(fmt.Sprintf("unload:failed to unload model: %s: %v\n", model.ModelInfo().ID, err))
		}
	}

	if sb.Len() > 0 {
		return fmt.Errorf("%s", sb.String())
	}

	return nil
}

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
			resp.Choice[0].Delta = model.ResponseMessage{}
			resp.Prompt = ""
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

// Embeddings provides support to interact with an embedding model.
func (krn *Kronk) Embeddings(ctx context.Context, input string) (model.EmbedReponse, error) {
	if !krn.ModelInfo().IsEmbedModel {
		return model.EmbedReponse{}, fmt.Errorf("embed:model doesn't support embedding")
	}

	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embed:context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.EmbedReponse, error) {
		return m.Embeddings(ctx, input)
	}

	return nonStreaming(ctx, krn, f)
}

// EmbeddingsHTTP provides http handler support for an embeddings call.
func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embeddings:context has no deadline, provide a reasonable timeout")
	}

	var input string
	inputReq, ok := d["input"].(string)
	if ok {
		input = inputReq
	}

	if input == "" {
		return model.EmbedReponse{}, fmt.Errorf("embeddings:missing input parameter")
	}

	resp, err := krn.Embeddings(ctx, input)
	if err != nil {
		return model.EmbedReponse{}, fmt.Errorf("chat-streaming-http:stream-response: %w", err)
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
