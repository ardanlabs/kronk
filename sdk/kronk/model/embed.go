package model

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Embeddings performs batch embedding for multiple inputs in a single
// forward pass. This is more efficient than calling Embeddings multiple times.
// Supported options in d:
//   - input ([]string): the texts to embed (required)
//   - truncate (bool): if true, truncate inputs to fit context window (default: false)
//   - truncate_direction (string): "right" (default) or "left"
//   - dimensions (int): reduce output to first N dimensions (for Matryoshka models)
func (m *Model) Embeddings(ctx context.Context, d D) (EmbedReponse, error) {
	if !m.modelInfo.IsEmbedModel {
		return EmbedReponse{}, fmt.Errorf("embeddings: model doesn't support embedding")
	}

	// Note: Multi-sequence batching doesn't work for embedding models
	// because GetEmbeddingsSeq only returns valid data for seqID 0.
	if m.cfg.NSeqMax > 1 {
		m.log(ctx, "embeddings: NSeqMax > 1 has no effect for embedding models (parallel sequence extraction not supported)")
	}

	var inputs []string

	switch v := d["input"].(type) {
	case string:
		inputs = []string{v}

	case []string:
		inputs = v

	case []any:
		inputs = make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return EmbedReponse{}, fmt.Errorf("embeddings: input[%d] is not a string", i)
			}
			inputs[i] = s
		}

	default:
		return EmbedReponse{}, fmt.Errorf("embeddings: missing or invalid input parameter (expected string or []string)")
	}

	if len(inputs) == 0 {
		return EmbedReponse{}, fmt.Errorf("embeddings: input cannot be empty")
	}

	// -------------------------------------------------------------------------

	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return EmbedReponse{}, fmt.Errorf("embeddings: unable to init from model: %w", err)
	}

	defer func() {
		llama.Synchronize(lctx)
		llama.Free(lctx)
	}()

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		return EmbedReponse{}, fmt.Errorf("embeddings: unable to get memory: %w", err)
	}

	select {
	case <-ctx.Done():
		return EmbedReponse{}, ctx.Err()

	default:
	}

	maxTokens := int(llama.NUBatch(lctx))
	ctxTokens := int(llama.NCtx(lctx))
	if ctxTokens < maxTokens {
		maxTokens = ctxTokens
	}

	truncate, _ := d["truncate"].(bool)
	direction, _ := d["truncate_direction"].(string)
	nativeDim := llama.ModelNEmbd(m.model)
	requestedDim, _ := d["dimensions"].(float64)

	if requestedDim > 0 && int(requestedDim) > int(nativeDim) {
		return EmbedReponse{}, fmt.Errorf("embeddings: requested %d dimensions but model only has %d", int(requestedDim), nativeDim)
	}

	// -------------------------------------------------------------------------

	// Tokenize all inputs upfront.
	allTokens := make([][]llama.Token, len(inputs))
	for i, input := range inputs {
		tokens := llama.Tokenize(m.vocab, input, true, true)

		if len(tokens) > maxTokens {
			if !truncate {
				return EmbedReponse{}, fmt.Errorf("embeddings: input[%d] has %d tokens but max is %d (set truncate=true to auto-truncate)", i, len(tokens), maxTokens)
			}

			originalLen := len(tokens)

			switch direction {
			case "left":
				tokens = tokens[len(tokens)-maxTokens:]

			default:
				tokens = tokens[:maxTokens]
			}

			m.log(ctx, "embeddings: truncated input", "index", i, "original_tokens", originalLen, "max_tokens", maxTokens, "direction", direction, "truncated_tokens", len(tokens))
		}

		allTokens[i] = tokens
	}

	// -------------------------------------------------------------------------

	// Process each input sequentially within the same context.

	embedData := make([]EmbedData, len(inputs))
	totalPromptTokens := 0

	for i, tokens := range allTokens {
		select {
		case <-ctx.Done():
			return EmbedReponse{}, ctx.Err()

		default:
		}

		totalPromptTokens += len(tokens)

		batch := llama.BatchGetOne(tokens)

		ret, err := llama.Decode(lctx, batch)
		if err != nil {
			return EmbedReponse{}, fmt.Errorf("embeddings: decode failed for input[%d]: %w", i, err)
		}

		if ret != 0 {
			return EmbedReponse{}, fmt.Errorf("embeddings: decode returned non-zero for input[%d]: %d", i, ret)
		}

		vec, err := llama.GetEmbeddingsSeq(lctx, 0, nativeDim)
		if err != nil {
			return EmbedReponse{}, fmt.Errorf("embeddings: unable to get embeddings for input[%d]: %w", i, err)
		}

		if requestedDim > 0 {
			vec = vec[:int(requestedDim)]
		}

		vec = normalizeVector(vec)

		embedData[i] = EmbedData{
			Object:    "embedding",
			Index:     i,
			Embedding: vec,
		}

		// Clear KV cache before next input.
		llama.MemoryClear(mem, true)
	}

	// -------------------------------------------------------------------------

	er := EmbedReponse{
		Object:  "list",
		Created: time.Now().UnixMilli(),
		Model:   m.modelInfo.ID,
		Data:    embedData,
		Usage: EmbedUsage{
			PromptTokens: totalPromptTokens,
			TotalTokens:  totalPromptTokens,
		},
	}

	return er, nil
}

// normalizeVector applies L2 normalization to the embedding vector.
func normalizeVector(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	if sum == 0 {
		return vec
	}

	sum = math.Sqrt(sum)
	norm := float32(1.0 / sum)

	for i, v := range vec {
		vec[i] = v * norm
	}

	return vec
}
