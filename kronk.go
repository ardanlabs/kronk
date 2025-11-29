// Package kronk provides support for working with models using llamacpp via yzma.
package kronk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Version contains the current version of the kronk package.
const Version = "0.21.0"

// =============================================================================

// LogLevel represents the logging level.
type LogLevel int

// Set of logging levels supported by llamacpp.
const (
	LogSilent LogLevel = iota + 1
	LogNormal
)

var (
	libraryLocation string
	initOnce        sync.Once
	initErr         error
)

// Init initializes the Kronk backend suport.
func Init(libPath string, logLevel LogLevel) error {
	initOnce.Do(func() {
		if err := llama.Load(libPath); err != nil {
			initErr = fmt.Errorf("unable to load library: %w", err)
			return
		}

		if err := mtmd.Load(libPath); err != nil {
			initErr = fmt.Errorf("unable to load mtmd library: %w", err)
			return
		}

		libraryLocation = libPath

		llama.Init()

		switch logLevel {
		case LogSilent:
			llama.LogSet(llama.LogSilent())
			mtmd.LogSet(llama.LogSilent())
		default:
			llama.LogSet(llama.LogNormal)
			mtmd.LogSet(llama.LogNormal)
		}
	})

	return initErr
}

// Kronk provides a concurrently safe api for using llamacpp to access models.
type Kronk struct {
	cfg       model.Config
	models    chan *model.Model
	wg        sync.WaitGroup
	closed    uint32
	modelInfo model.ModelInfo
}

// New provides the ability to use models in a concurrently safe way.
//
// modelInstances represents the number of instances of the model to create. Unless
// you have more than 1 GPU, the recommended number of instances is 1.
func New(modelInstances int, cfg model.Config) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("the Init() function has not been called")
	}

	if modelInstances <= 0 {
		return nil, fmt.Errorf("instances must be > 0, got %d", modelInstances)
	}

	models := make(chan *model.Model, modelInstances)
	var firstModel *model.Model

	for range modelInstances {
		m, err := model.NewModel(cfg)
		if err != nil {
			close(models)
			for model := range models {
				model.Unload()
			}

			return nil, err
		}

		models <- m

		if firstModel == nil {
			firstModel = m
		}
	}

	cfg = firstModel.Config()

	krn := Kronk{
		cfg:       cfg,
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

// ModelInfo returns the model information.
func (krn *Kronk) ModelInfo() model.ModelInfo {
	return krn.modelInfo
}

// Unload will close down all loaded models. You should call this only when you
// are completely done using the group.
func (krn *Kronk) Unload() {
	if !atomic.CompareAndSwapUint32(&krn.closed, 0, 1) {
		return
	}

	krn.wg.Wait()

	close(krn.models)
	for model := range krn.models {
		model.Unload()
	}
}

// Chat provides support to interact with an inference model.
func (krn *Kronk) Chat(ctx context.Context, cr model.ChatRequest) (model.ChatResponse, error) {
	f := func(m *model.Model) (model.ChatResponse, error) {
		return m.Chat(ctx, cr)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// ChatStreaming provides support to interact with an inference model.
func (krn *Kronk) ChatStreaming(ctx context.Context, cr model.ChatRequest) (<-chan model.ChatResponse, error) {
	f := func(m *model.Model) <-chan model.ChatResponse {
		return m.ChatStreaming(ctx, cr)
	}

	ef := func(err error) model.ChatResponse {
		return model.ChatResponseErr("panic", model.ObjectChat, "", 0, err, model.Usage{})
	}

	return streaming(ctx, krn, &krn.closed, f, ef)
}

// Logger is a function type for logging.
type Logger func(ctx context.Context, format string, a ...any)

// ChatStreamingHTTP streams the response to an HTTP client.
func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, log Logger, w http.ResponseWriter, cr model.ChatRequest) error {
	log(ctx, "streamResponse: started")

	f, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	ch, err := krn.ChatStreaming(ctx, cr)
	if err != nil {
		return fmt.Errorf("streamResponse: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	var lr model.ChatResponse

	for resp := range ch {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return errors.New("client disconnected, do not send response")
			}
		}

		d, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("json.Marshal: %w", err)
		}

		if resp.Choice[0].FinishReason == model.FinishReasonError {
			log(ctx, "streamResponse: ERROR: %s", resp.Choice[0].Delta.Content)
			fmt.Fprintf(w, "data: %s\n", d)
			f.Flush()
			break
		}

		fmt.Fprintf(w, "data: %s\n", d)
		f.Flush()

		lr = resp
	}

	w.Write([]byte("data: [DONE]\n"))
	f.Flush()

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.InputTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	log(ctx, "streamResponse: Input: %d  Output: %d  Context: %d (%.0f%% of %.0fK) TPS: %.2f",
		lr.Usage.InputTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return nil
}

// Vision provides support to interact with a vision inference model.
func (krn *Kronk) Vision(ctx context.Context, vr model.VisionRequest) (model.ChatResponse, error) {
	f := func(m *model.Model) (model.ChatResponse, error) {
		return m.Vision(ctx, vr)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// VisionStreaming provides support to interact with a vision language model.
// It will block until a model becomes available or the context times out.
func (krn *Kronk) VisionStreaming(ctx context.Context, vr model.VisionRequest) (<-chan model.ChatResponse, error) {
	f := func(m *model.Model) <-chan model.ChatResponse {
		return m.VisionStreaming(ctx, vr)
	}

	ef := func(err error) model.ChatResponse {
		return model.ChatResponseErr("panic", model.ObjectVision, "", 0, err, model.Usage{})
	}

	return streaming(ctx, krn, &krn.closed, f, ef)
}

// Embed provides support to interact with an embedding model. It will block
// until a model becomes available or the context times out.
func (krn *Kronk) Embed(ctx context.Context, text string) ([]float32, error) {
	f := func(m *model.Model) ([]float32, error) {
		return m.Embed(ctx, text)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// =============================================================================

type nonStreamingFunc[T any] func(llama *model.Model) (T, error)

func nonStreaming[T any](ctx context.Context, krn *Kronk, closed *uint32, f nonStreamingFunc[T]) (T, error) {
	var zero T

	if atomic.LoadUint32(closed) == 1 {
		return zero, fmt.Errorf("Kronk has been unloaded")
	}

	select {
	case <-ctx.Done():
		return zero, ctx.Err()

	case llama, ok := <-krn.models:
		if !ok {
			return zero, fmt.Errorf("Kronk has been unloaded")
		}

		krn.wg.Add(1)

		defer func() {
			krn.models <- llama
			krn.wg.Done()
		}()

		return f(llama)
	}
}

type streamingFunc[T any] func(llama *model.Model) <-chan T
type errorFunc[T any] func(err error) T

func streaming[T any](ctx context.Context, krn *Kronk, closed *uint32, f streamingFunc[T], ef errorFunc[T]) (<-chan T, error) {
	var zero chan T

	if atomic.LoadUint32(closed) == 1 {
		return zero, fmt.Errorf("Kronk has been unloaded")
	}

	ch := make(chan T)

	select {
	case <-ctx.Done():
		return zero, ctx.Err()

	case llama, ok := <-krn.models:
		if !ok {
			return zero, fmt.Errorf("Kronk has been unloaded")
		}

		krn.wg.Add(1)

		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					select {
					case <-ctx.Done():
					case ch <- ef(fmt.Errorf("%v", rec)):
					default:
					}
				}

				close(ch)
				krn.models <- llama
				krn.wg.Done()
			}()

			lch := f(llama)
			for msg := range lch {
				select {
				case <-ctx.Done():
					return
				case ch <- msg:
				}
			}
		}()
	}

	return ch, nil
}
