package model

import (
	"context"
	"fmt"
	"math"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Embeddings performs an embedding request and returns the final response.
// Supported options in d:
//   - input (string): the text to embed (required)
//   - truncate (bool): if true, truncate input to fit context window (default: false)
//   - truncate_direction (string): "right" (default) or "left"
func (m *Model) Embeddings(ctx context.Context, d D) (EmbedReponse, error) {
	if !m.modelInfo.IsEmbedModel {
		return EmbedReponse{}, fmt.Errorf("embeddings: model doesn't support embedding")
	}

	input, _ := d["input"].(string)
	if input == "" {
		return EmbedReponse{}, fmt.Errorf("embeddings: missing input parameter")
	}

	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return EmbedReponse{}, fmt.Errorf("embeddings: unable to init from model: %w", err)
	}

	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	select {
	case <-ctx.Done():
		return EmbedReponse{}, ctx.Err()
	default:
	}

	tokens := llama.Tokenize(m.vocab, input, true, true)

	if truncate, _ := d["truncate"].(bool); truncate {
		maxTokens := int(llama.NCtx(lctx))

		if len(tokens) > maxTokens {
			direction, _ := d["truncate_direction"].(string)

			switch direction {
			case "left":
				tokens = tokens[len(tokens)-maxTokens:]
			default:
				tokens = tokens[:maxTokens]
			}
		}
	}

	batch := llama.BatchGetOne(tokens)
	llama.Decode(lctx, batch)

	dimensions := llama.ModelNEmbd(m.model)
	vec, err := llama.GetEmbeddingsSeq(lctx, 0, dimensions)
	if err != nil {
		return EmbedReponse{}, fmt.Errorf("embeddings: unable to get embeddings: %w", err)
	}

	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	resp := toEmbedResponse(m.modelInfo.ID, vec)

	if sum == 0 {
		return resp, nil
	}

	sum = math.Sqrt(sum)
	norm := float32(1.0 / sum)

	for i, v := range vec {
		vec[i] = v * norm
	}

	resp.Data[0].Embedding = vec

	return resp, nil
}
