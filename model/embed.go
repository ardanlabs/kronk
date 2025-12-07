package model

import (
	"context"
	"fmt"
	"math"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Embed performs an embedding request and returns the final response.
func (m *Model) Embed(ctx context.Context, text string) ([]float32, error) {
	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return nil, fmt.Errorf("embed:unable to init from model: %w", err)
	}

	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tokens := llama.Tokenize(m.vocab, text, true, true)
	batch := llama.BatchGetOne(tokens)
	llama.Decode(lctx, batch)

	dimensions := llama.ModelNEmbd(m.model)
	vec, err := llama.GetEmbeddingsSeq(lctx, 0, dimensions)
	if err != nil {
		return nil, fmt.Errorf("embed:unable to get embeddings: %w", err)
	}

	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	if sum == 0 {
		return vec, nil
	}

	sum = math.Sqrt(sum)
	norm := float32(1.0 / sum)

	for i, v := range vec {
		vec[i] = v * norm
	}

	return vec, nil
}
