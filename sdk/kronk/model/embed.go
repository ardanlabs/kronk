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

	embedData := make([]EmbedData, len(inputs))
	totalPromptTokens := 0

	// Determine max sequences per batch from NSeqMax config.
	maxSeqs := max(int(m.ctxParams.NSeqMax), 1)

	// Process inputs in chunks respecting NSeqMax.
	for chunkStart := 0; chunkStart < len(inputs); chunkStart += maxSeqs {
		select {
		case <-ctx.Done():
			return EmbedReponse{}, ctx.Err()

		default:
		}

		chunkEnd := min(chunkStart+maxSeqs, len(inputs))
		chunkSize := chunkEnd - chunkStart

		batch := llama.BatchInit(int32(ctxTokens), 0, int32(chunkSize))

		// Add all tokens for this chunk to the batch.
		for i := range chunkSize {
			globalIdx := chunkStart + i
			tokens := allTokens[globalIdx]
			seqID := llama.SeqId(i)
			totalPromptTokens += len(tokens)

			for pos, token := range tokens {
				isLast := pos == len(tokens)-1
				batchAdd(&batch, token, llama.Pos(pos), []llama.SeqId{seqID}, isLast)
			}
		}

		// Single decode for the entire chunk.
		ret, err := llama.Decode(lctx, batch)
		if err != nil {
			llama.BatchFree(batch)
			return EmbedReponse{}, fmt.Errorf("embeddings: decode failed: %w", err)
		}

		if ret != 0 {
			llama.BatchFree(batch)
			return EmbedReponse{}, fmt.Errorf("embeddings: decode returned non-zero: %d", ret)
		}

		// Extract embeddings for each sequence in this chunk.
		for i := range chunkSize {
			globalIdx := chunkStart + i
			seqID := llama.SeqId(i)

			vec, err := llama.GetEmbeddingsSeq(lctx, seqID, nativeDim)
			if err != nil {
				llama.BatchFree(batch)
				return EmbedReponse{}, fmt.Errorf("embeddings: unable to get embeddings for input[%d]: %w", globalIdx, err)
			}

			if requestedDim > 0 {
				vec = vec[:int(requestedDim)]
			}

			vec = normalizeVector(vec)

			embedData[globalIdx] = EmbedData{
				Object:    "embedding",
				Index:     globalIdx,
				Embedding: vec,
			}

			// Clear KV cache for this sequence before next chunk.
			llama.MemorySeqRm(m.mem, seqID, -1, -1)
		}

		llama.BatchFree(batch)
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
