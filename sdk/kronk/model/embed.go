package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Embeddings performs embedding for one or more inputs.
//
// Supported options in d:
//   - input ([]string): the texts to embed (required)
//   - truncate (bool): if true, truncate inputs to fit context window (default: false)
//   - truncate_direction (string): "right" (default) or "left"
//   - dimensions (int): reduce output to first N dimensions (for Matryoshka models)
//
// Supported models process inputs together as a multi-sequence batch. Other
// models use the context-pool fallback.
func (m *Model) Embeddings(ctx context.Context, d D) (response EmbedReponse, err error) {
	if !m.modelInfo.IsEmbedModel {
		return EmbedReponse{}, fmt.Errorf("embeddings: model doesn't support embedding")
	}

	started := time.Now()
	runtimeName := "context_pool"
	if m.batchSeq != nil {
		runtimeName = "batchseq"
	}
	totalPromptTokens := 0
	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "embedding", runtimeName, 1)
	defer func() {
		metrics.AddInferenceActiveRequests(m.modelInfo.ID, "embedding", runtimeName, -1)
		status := "ok"
		if err != nil {
			status = "error"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				status = "cancel"
			}
		}
		metrics.ObserveInferenceRequest(m.modelInfo.ID, "embedding", runtimeName, status, time.Since(started), totalPromptTokens)
	}()

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

	truncate, _ := d["truncate"].(bool)
	direction, _ := d["truncate_direction"].(string)
	nativeDim := llama.ModelNEmbd(m.model)
	requestedDim, _ := d["dimensions"].(float64)

	if requestedDim > 0 && int(requestedDim) > int(nativeDim) {
		return EmbedReponse{}, fmt.Errorf("embeddings: requested %d dimensions but model only has %d", int(requestedDim), nativeDim)
	}

	// -------------------------------------------------------------------------

	var embedData []EmbedData

	if m.batchSeq != nil {
		embedData, totalPromptTokens, err = m.processEmbeddingsBatchSeq(ctx, inputs, truncate, direction, nativeDim, int(requestedDim))
	} else {
		// The fallback runtime processes concurrent requests on independent
		// single-sequence contexts.
		pc, acquireErr := m.pool.acquire(ctx)
		if acquireErr != nil {
			return EmbedReponse{}, acquireErr
		}
		defer m.pool.release(pc)

		embedData, totalPromptTokens, err = m.processEmbeddings(ctx, pc, inputs, truncate, direction, nativeDim, int(requestedDim))
	}
	if err != nil {
		return EmbedReponse{}, err
	}

	// -------------------------------------------------------------------------

	er := EmbedReponse{
		Object:  "list",
		Created: time.Now().Unix(),
		Model:   m.modelInfo.ID,
		Data:    embedData,
		Usage: EmbedUsage{
			PromptTokens: totalPromptTokens,
			TotalTokens:  totalPromptTokens,
		},
	}

	return er, nil
}

// processEmbeddingsBatchSeq processes inputs as multi-sequence batches on one
// llama context.
func (m *Model) processEmbeddingsBatchSeq(ctx context.Context, inputs []string, truncate bool, direction string, nativeDim int32, requestedDim int) ([]EmbedData, int, error) {
	maxTokens := min(m.batchSeq.maxTokens, m.cfg.ContextWindow())
	items := make([]batchSeqItem, len(inputs))
	totalTokens := 0

	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}

		tokens := llama.Tokenize(m.vocab, input, m.addBOSToken, true)
		if len(tokens) > maxTokens {
			if !truncate {
				return nil, 0, fmt.Errorf("embeddings: input[%d] has %d tokens but max is %d (set truncate=true to auto-truncate)", i, len(tokens), maxTokens)
			}

			originalLen := len(tokens)
			switch direction {
			case "left":
				tokens = tokens[len(tokens)-maxTokens:]
			default:
				tokens = tokens[:maxTokens]
			}

			m.log(ctx, "embeddings", "status", "truncated input", "index", i, "original_tokens", originalLen, "max_tokens", maxTokens, "direction", direction, "truncated_tokens", len(tokens))
		}

		items[i] = batchSeqItem{index: i, tokens: tokens}
		totalTokens += len(tokens)
	}

	outputs, err := m.batchSeq.run(ctx, items, int(nativeDim))
	if err != nil {
		return nil, 0, fmt.Errorf("embeddings: batchseq inference: %w", err)
	}

	embedData := make([]EmbedData, len(outputs))
	for i, output := range outputs {
		embedData[i] = EmbedData{
			Object:    "embedding",
			Index:     i,
			Embedding: embeddingVector(output, requestedDim),
		}
	}

	return embedData, totalTokens, nil
}

// processEmbeddings processes all inputs on a single context.
func (m *Model) processEmbeddings(ctx context.Context, pc poolContext, inputs []string, truncate bool, direction string, nativeDim int32, requestedDim int) ([]EmbedData, int, error) {
	maxTokens := int(llama.NUBatch(pc.lctx))
	ctxTokens := int(llama.NCtx(pc.lctx))
	if ctxTokens < maxTokens {
		maxTokens = ctxTokens
	}

	embedData := make([]EmbedData, len(inputs))
	totalTokens := 0

	for i, input := range inputs {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()

		default:
		}

		tokens := llama.Tokenize(m.vocab, input, m.addBOSToken, true)

		if len(tokens) > maxTokens {
			if !truncate {
				return nil, 0, fmt.Errorf("embeddings: input[%d] has %d tokens but max is %d (set truncate=true to auto-truncate)", i, len(tokens), maxTokens)
			}

			originalLen := len(tokens)

			switch direction {
			case "left":
				tokens = tokens[len(tokens)-maxTokens:]

			default:
				tokens = tokens[:maxTokens]
			}

			m.log(ctx, "embeddings", "status", "truncated input", "index", i, "original_tokens", originalLen, "max_tokens", maxTokens, "direction", direction, "truncated_tokens", len(tokens))
		}

		totalTokens += len(tokens)

		batch := llama.BatchGetOne(tokens)

		ret, err := llama.Decode(pc.lctx, batch)
		if err != nil {
			return nil, 0, fmt.Errorf("embeddings: decode failed for input[%d]: %w", i, err)
		}

		if ret != 0 {
			return nil, 0, fmt.Errorf("embeddings: decode returned non-zero for input[%d]: %d", i, ret)
		}

		rawVec, err := llama.GetEmbeddingsSeq(pc.lctx, 0, nativeDim)
		if err != nil {
			return nil, 0, fmt.Errorf("embeddings: unable to get embeddings for input[%d]: %w", i, err)
		}

		embedData[i] = EmbedData{
			Object:    "embedding",
			Index:     i,
			Embedding: embeddingVector(rawVec, requestedDim),
		}

		// Clear KV cache before next input.
		llama.MemoryClear(pc.mem, true)
	}

	return embedData, totalTokens, nil
}

// embeddingVector copies, optionally reduces, and normalizes a native
// embedding output.
func embeddingVector(rawVec []float32, requestedDim int) []float32 {
	vec := slices.Clone(rawVec)
	if requestedDim > 0 {
		vec = vec[:requestedDim]
	}

	return normalizeVector(vec)
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
