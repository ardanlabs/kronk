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
	input, ok := d["input"].(string)
	if ok {
		inputs = []string{input}
	}

	if inputs == nil {
		inputs, ok := d["input"].([]string)
		if !ok || len(inputs) == 0 {
			return EmbedReponse{}, fmt.Errorf("embeddings: missing or invalid input parameter (expected string or []string)")
		}
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

	maxTokens := int(llama.NUBatch(lctx))
	ctxTokens := int(llama.NCtx(lctx))
	if ctxTokens < maxTokens {
		maxTokens = ctxTokens
	}

	truncate, _ := d["truncate"].(bool)
	direction, _ := d["truncate_direction"].(string)

	type tokenizedInput struct {
		tokens []llama.Token
		seqID  llama.SeqId
	}

	tokenizedInputs := make([]tokenizedInput, len(inputs))
	totalPromptTokens := 0

	nSeqs := int32(len(inputs))
	batch := llama.BatchInit(int32(ctxTokens), 0, nSeqs)
	defer llama.BatchFree(batch)

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

		seqID := llama.SeqId(i)
		tokenizedInputs[i] = tokenizedInput{
			tokens: tokens,
			seqID:  seqID,
		}

		totalPromptTokens += len(tokens)

		for pos, token := range tokens {
			isLast := pos == len(tokens)-1
			batchAdd(&batch, token, llama.Pos(pos), []llama.SeqId{seqID}, isLast)
		}
	}

	ret, err := llama.Decode(lctx, batch)
	if err != nil {
		return EmbedReponse{}, fmt.Errorf("embeddings: decode failed: %w", err)
	}

	if ret != 0 {
		return EmbedReponse{}, fmt.Errorf("embeddings: decode returned non-zero: %d", ret)
	}

	nativeDim := llama.ModelNEmbd(m.model)
	requestedDim, _ := d["dimensions"].(float64)

	if requestedDim > 0 && int(requestedDim) > int(nativeDim) {
		return EmbedReponse{}, fmt.Errorf("embeddings: requested %d dimensions but model only has %d", int(requestedDim), nativeDim)
	}

	embedData := make([]EmbedData, len(inputs))

	for i, ti := range tokenizedInputs {
		vec, err := llama.GetEmbeddingsSeq(lctx, ti.seqID, nativeDim)
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
	}

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
