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

	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// Llama represents a concurrency group of a specified model.
type Llama struct {
	modelName string
	llama     chan *model
	wg        sync.WaitGroup
}

// New provides the ability to use models in a concurrently safe way.
func New(concurrency int, libPath string, modelFile string, cfg Config, options ...func(llg *model) error) (*Llama, error) {
	llama := make(chan *model, concurrency)

	for range concurrency {
		l, err := newModel(libPath, modelFile, cfg, options...)
		if err != nil {
			return nil, err
		}

		llama <- l
	}

	mgr := Llama{
		modelName: strings.TrimSuffix(filepath.Base(modelFile), filepath.Ext(modelFile)),
		llama:     llama,
	}

	return &mgr, nil
}

func WithProjection(projFile string) func(m *model) error {
	return func(m *model) error {
		if err := mtmd.Load(m.libPath); err != nil {
			return fmt.Errorf("unable to load mtmd library: %w", err)
		}

		m.projFile = projFile

		return nil
	}
}

// ModelName returns the model name.
func (llm *Llama) ModelName() string {
	return llm.modelName
}

// Unload will close down all loaded models. You should call this only when you
// are completely done using the group.
func (llm *Llama) Unload() {
	llm.wg.Wait()

	close(llm.llama)
	for llama := range llm.llama {
		llama.unload()
	}
}

// ModelInfo provides support to extract the model card information.
func (llm *Llama) ModelInfo(ctx context.Context) (ModelInfo, error) {
	select {
	case <-ctx.Done():
		return ModelInfo{}, ctx.Err()

	case llama := <-llm.llama:
		llm.wg.Add(1)
		defer func() {
			llm.llama <- llama
			llm.wg.Done()
		}()

		return llama.modelInfo(), nil
	}
}

// ChatCompletions provides support to interact with an inference model.
// It will block until a model becomes available or the context times out.
func (llm *Llama) ChatCompletions(ctx context.Context, messages []ChatMessage, params Params) (<-chan ChatResponse, error) {
	ch := make(chan ChatResponse)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case llama := <-llm.llama:
		llm.wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				llm.llama <- llama
				llm.wg.Done()
			}()

			lch := llama.chatCompletions(messages, params)
			for msg := range lch {
				ch <- msg
			}
		}()
	}

	return ch, nil
}

// ChatVision provides support to interact with a vision language model. It will
// block until a model becomes available or the context times out.
func (llm *Llama) ChatVision(ctx context.Context, message ChatMessage, imageFile string, params Params) (<-chan ChatResponse, error) {
	ch := make(chan ChatResponse)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case llama := <-llm.llama:
		llm.wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				llm.llama <- llama
				llm.wg.Done()
			}()

			lch := llama.chatVision(message, imageFile, params)
			for msg := range lch {
				ch <- msg
			}
		}()
	}

	return ch, nil
}

// Embed provides support to interact with an embedding model. It will block
// until a model becomes available or the context times out.
func (llm *Llama) Embed(ctx context.Context, text string) ([]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case llama := <-llm.llama:
		llm.wg.Add(1)
		defer func() {
			llm.llama <- llama
			llm.wg.Done()
		}()

		vec, err := llama.embed(text)
		if err != nil {
			return nil, err
		}

		return vec, nil
	}
}

// Rerank provides support to rerank a set of embeddings.
func (llm *Llama) Rerank(rankingDocs []RankingDocument) ([]Ranking, error) {
	rerankedDocs := make([]Ranking, len(rankingDocs))

	// Simple scoring based on embedding magnitude and positive values.
	for i, doc := range rankingDocs {
		var sumPositive, sumTotal float64
		for _, val := range doc.Embedding {
			sumTotal += val * val
			if val > 0 {
				sumPositive += val
			}
		}

		if sumTotal == 0 {
			rerankedDocs[i] = Ranking{Document: doc.Document, Score: 0}
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
