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

		libraryLocation = libPath

		llama.Init()

		switch logLevel {
		case LogSilent:
			llama.LogSet(llama.LogSilent())
		default:
			llama.LogSet(llama.LogNormal)
		}
	})

	return initErr
}

// Kronk provides a concurrently safe api for using llamacpp to access models.
type Kronk struct {
	modelName string
	models    chan *model
	wg        sync.WaitGroup
	closed    uint32
}

// New provides the ability to use models in a concurrently safe way.
func New(concurrency int, modelFile string, cfg Config, options ...func(llg *model) error) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("the Init() function has not been called")
	}

	if concurrency <= 0 {
		return nil, fmt.Errorf("concurrency must be > 0, got %d", concurrency)
	}

	models := make(chan *model, concurrency)

	for range concurrency {
		model, err := newModel(modelFile, cfg, options...)
		if err != nil {
			close(models)
			for model := range models {
				model.unload()
			}

			return nil, err
		}

		models <- model
	}

	krn := Kronk{
		modelName: strings.TrimSuffix(filepath.Base(modelFile), filepath.Ext(modelFile)),
		models:    models,
	}

	return &krn, nil
}

func WithProjection(projFile string) func(m *model) error {
	return func(m *model) error {
		if err := mtmd.Load(libraryLocation); err != nil {
			return fmt.Errorf("unable to load mtmd library: %w", err)
		}

		m.projFile = projFile

		return nil
	}
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
	return nonStreaming(ctx, krn, &krn.closed, func(llama *model) (ModelInfo, error) {
		return llama.modelInfo(), nil
	})
}

// ChatCompletions provides support to interact with an inference model.
// It will block until a model becomes available or the context times out.
func (krn *Kronk) ChatCompletions(ctx context.Context, messages []ChatMessage, params Params) (<-chan ChatResponse, error) {
	return streaming(ctx, krn, &krn.closed, func(llama *model) <-chan ChatResponse {
		return llama.chatCompletions(ctx, messages, params)
	})
}

// ChatVision provides support to interact with a vision language model. It will
// block until a model becomes available or the context times out.
func (krn *Kronk) ChatVision(ctx context.Context, message ChatMessage, imageFile string, params Params) (<-chan ChatResponse, error) {
	return streaming(ctx, krn, &krn.closed, func(llama *model) <-chan ChatResponse {
		return llama.chatVision(ctx, message, imageFile, params)
	})
}

// Embed provides support to interact with an embedding model. It will block
// until a model becomes available or the context times out.
func (krn *Kronk) Embed(ctx context.Context, text string) ([]float32, error) {
	return nonStreaming(ctx, krn, &krn.closed, func(llama *model) ([]float32, error) {
		return llama.embed(ctx, text)
	})
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
