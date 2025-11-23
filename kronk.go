// Package kronk provides support for working with models using llamacpp via yzma.
package kronk

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

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
	cfg       ModelConfig
	modelName string
	models    chan *model
	wg        sync.WaitGroup
	closed    uint32
}

// New provides the ability to use models in a concurrently safe way.
func New(concurrency int, modelFile string, projFile string, cfg ModelConfig) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("the Init() function has not been called")
	}

	if concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be > 0, got %d", concurrency)
	}

	models := make(chan *model, concurrency)
	var firstModel *model

	for range concurrency {
		m, err := newModel(modelFile, projFile, cfg)
		if err != nil {
			close(models)
			for model := range models {
				model.unload()
			}

			return nil, err
		}

		models <- m

		if firstModel == nil {
			firstModel = m
		}
	}

	krn := Kronk{
		cfg:       firstModel.cfg,
		modelName: strings.TrimSuffix(filepath.Base(modelFile), filepath.Ext(modelFile)),
		models:    models,
	}

	return &krn, nil
}

// ModelConfig returns a copy of the configuration being used. This may be
// different from the configuration passed to New() if the model has
// overridden any of the settings.
func (krn *Kronk) ModelConfig() ModelConfig {
	return krn.cfg
}

// ModelName returns the model name.
func (krn *Kronk) ModelName() string {
	return krn.modelName
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
		model.unload()
	}
}

// ModelInfo provides support to extract the model card information.
func (krn *Kronk) ModelInfo(ctx context.Context) (ModelInfo, error) {
	f := func(m *model) (ModelInfo, error) {
		return m.modelInfo(), nil
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// Chat provides support to interact with an inference model.
func (krn *Kronk) Chat(ctx context.Context, messages []ChatMessage, params Params) (string, error) {
	f := func(m *model) (string, error) {
		return m.chat(ctx, messages, params)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// ChatStreaming provides support to interact with an inference model.
// It will block until a model becomes available or the context times out.
func (krn *Kronk) ChatStreaming(ctx context.Context, messages []ChatMessage, params Params) (<-chan ChatResponse, error) {
	f := func(m *model) <-chan ChatResponse {
		return m.chatStreaming(ctx, messages, params)
	}

	ef := func(err error) ChatResponse {
		return ChatResponse{Err: err}
	}

	return streaming(ctx, krn, &krn.closed, f, ef)
}

// Vision provides support to interact with a vision inference model.
func (krn *Kronk) Vision(ctx context.Context, message ChatMessage, imageFile string, params Params) (string, error) {
	f := func(m *model) (string, error) {
		return m.vision(ctx, message, imageFile, params)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// VisionStreaming provides support to interact with a vision language model.
// It will block until a model becomes available or the context times out.
func (krn *Kronk) VisionStreaming(ctx context.Context, message ChatMessage, imageFile string, params Params) (<-chan ChatResponse, error) {
	f := func(m *model) <-chan ChatResponse {
		return m.visionStreaming(ctx, message, imageFile, params)
	}

	ef := func(err error) ChatResponse {
		return ChatResponse{Err: err}
	}

	return streaming(ctx, krn, &krn.closed, f, ef)
}

// Embed provides support to interact with an embedding model. It will block
// until a model becomes available or the context times out.
func (krn *Kronk) Embed(ctx context.Context, text string) ([]float32, error) {
	f := func(m *model) ([]float32, error) {
		return m.embed(ctx, text)
	}

	return nonStreaming(ctx, krn, &krn.closed, f)
}

// Rerank provides support to rerank a set of embeddings.
func (krn *Kronk) Rerank(rankingDocs []RankingDocument) ([]Ranking, error) {
	rerankedDocs := make([]Ranking, len(rankingDocs))

	// Simple scoring based on embedding magnitude and positive values.
	for i, doc := range rankingDocs {
		if len(doc.Embedding) == 0 {
			rerankedDocs[i] = Ranking{Document: doc.Document, Score: 0}
			continue
		}

		var sumPositive, sumTotal float64
		for _, val := range doc.Embedding {
			sumTotal += val * val
			if val > 0 {
				sumPositive += val
			}
		}

		if sumTotal == 0 {
			rerankedDocs[i] = Ranking{Document: doc.Document, Score: 0}
			continue
		}

		// Normalize and combine magnitude with positive bias
		magnitude := math.Sqrt(sumTotal) / float64(len(doc.Embedding))
		positiveRatio := sumPositive / float64(len(doc.Embedding))
		score := (magnitude + positiveRatio) / 2

		rerankedDocs[i] = Ranking{Document: doc.Document, Score: score}
	}

	sort.Slice(rerankedDocs, func(i, j int) bool {
		return rerankedDocs[i].Score > rerankedDocs[j].Score
	})

	return rerankedDocs, nil
}
