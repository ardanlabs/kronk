package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Rerank performs reranking for a query against multiple documents.
// It scores each document's relevance to the query and returns results
// sorted by relevance score (highest first).
//
// Supported options in d:
//   - query (string): the query to rank documents against (required)
//   - documents ([]string): the documents to rank (required)
//   - top_n (int): return only the top N results (optional, default: all)
//   - return_documents (bool): include document text in results (default: false)
//
// Supported models process documents together as a multi-sequence batch.
// Other models use the context-pool fallback.
func (m *Model) Rerank(ctx context.Context, d D) (response RerankResponse, err error) {
	if !m.modelInfo.IsRerankModel {
		return RerankResponse{}, fmt.Errorf("rerank: model doesn't support reranking")
	}

	started := time.Now()
	runtimeName := "context_pool"
	if m.batchSeq != nil {
		runtimeName = "batchseq"
	}
	totalPromptTokens := 0
	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "rerank", runtimeName, 1)
	defer func() {
		metrics.AddInferenceActiveRequests(m.modelInfo.ID, "rerank", runtimeName, -1)
		status := "ok"
		if err != nil {
			status = "error"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				status = "cancel"
			}
		}
		metrics.ObserveInferenceRequest(m.modelInfo.ID, "rerank", runtimeName, status, time.Since(started), totalPromptTokens)
	}()

	query, ok := d["query"].(string)
	if !ok || query == "" {
		return RerankResponse{}, fmt.Errorf("rerank: missing or invalid query parameter")
	}

	var documents []string

	switch v := d["documents"].(type) {
	case []string:
		documents = v

	case []any:
		documents = make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return RerankResponse{}, fmt.Errorf("rerank: documents[%d] is not a string", i)
			}
			documents[i] = s
		}

	default:
		return RerankResponse{}, fmt.Errorf("rerank: missing or invalid documents parameter (expected []string)")
	}

	if len(documents) == 0 {
		return RerankResponse{}, fmt.Errorf("rerank: documents cannot be empty")
	}

	topN := len(documents)
	if n, ok := d["top_n"].(float64); ok && n > 0 {
		topN = int(n)
	}

	if n, ok := d["top_n"].(int); ok && n > 0 {
		topN = n
	}

	returnDocuments, _ := d["return_documents"].(bool)

	// -------------------------------------------------------------------------

	var results []RerankResult

	if m.batchSeq != nil {
		results, totalPromptTokens, err = m.processRerankBatchSeq(ctx, query, documents, returnDocuments)
	} else {
		// The fallback runtime processes concurrent requests on independent
		// single-sequence contexts.
		pc, acquireErr := m.pool.acquire(ctx)
		if acquireErr != nil {
			return RerankResponse{}, acquireErr
		}
		defer m.pool.release(pc)

		results, totalPromptTokens, err = m.processRerank(ctx, pc, query, documents, returnDocuments)
	}
	if err != nil {
		return RerankResponse{}, err
	}

	// -------------------------------------------------------------------------

	// Sort results by relevance score (descending).
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// Apply top_n limit.
	if topN < len(results) {
		results = results[:topN]
	}

	// -------------------------------------------------------------------------

	rr := RerankResponse{
		Object:  "list",
		Created: time.Now().Unix(),
		Model:   m.modelInfo.ID,
		Data:    results,
		Usage: RerankUsage{
			PromptTokens: totalPromptTokens,
			TotalTokens:  totalPromptTokens,
		},
	}

	return rr, nil
}

// processRerankBatchSeq processes query-document pairs as multi-sequence
// batches on one llama context.
func (m *Model) processRerankBatchSeq(ctx context.Context, query string, documents []string, returnDocuments bool) ([]RerankResult, int, error) {
	maxTokens := rerankTokenLimit(m.batchSeq.maxTokens, m.cfg.ContextWindow())

	nClsOut := int(llama.ModelNClsOut(m.model))
	if nClsOut == 0 {
		nClsOut = 1
	}

	items := make([]batchSeqItem, len(documents))
	totalTokens := 0
	for i, doc := range documents {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}

		pairText := formatRerankPair(query, doc)
		tokens := llama.Tokenize(m.vocab, pairText, m.addBOSToken, true)
		if len(tokens) > maxTokens {
			m.log(ctx, "rerank", "status", "truncating input", "index", i, "original_tokens", len(tokens), "max_tokens", maxTokens)
			tokens = tokens[:maxTokens]
		}

		items[i] = batchSeqItem{index: i, tokens: tokens}
		totalTokens += len(tokens)
	}

	outputs, err := m.batchSeq.run(ctx, items, nClsOut)
	if err != nil {
		return nil, 0, fmt.Errorf("rerank: batchseq inference: %w", err)
	}

	results := make([]RerankResult, len(documents))
	for i, output := range outputs {
		result := RerankResult{
			Index:          i,
			RelevanceScore: sigmoid(output[0]),
		}
		if returnDocuments {
			result.Document = documents[i]
		}
		results[i] = result
	}

	return results, totalTokens, nil
}

// processRerank processes all documents on a single context.
func (m *Model) processRerank(ctx context.Context, pc poolContext, query string, documents []string, returnDocuments bool) ([]RerankResult, int, error) {
	maxTokens := rerankTokenLimit(int(llama.NUBatch(pc.lctx)), m.cfg.ContextWindow())

	nClsOut := llama.ModelNClsOut(m.model)
	if nClsOut == 0 {
		nClsOut = 1
	}

	results := make([]RerankResult, len(documents))
	totalTokens := 0

	for i, doc := range documents {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()

		default:
		}

		// Format the query-document pair for the reranker model.
		pairText := formatRerankPair(query, doc)

		tokens := llama.Tokenize(m.vocab, pairText, m.addBOSToken, true)

		if len(tokens) > maxTokens {
			m.log(ctx, "rerank", "status", "truncating input", "index", i, "original_tokens", len(tokens), "max_tokens", maxTokens)
			tokens = tokens[:maxTokens]
		}

		totalTokens += len(tokens)

		batch := llama.BatchGetOne(tokens)

		ret, err := llama.Decode(pc.lctx, batch)
		if err != nil {
			return nil, 0, fmt.Errorf("rerank: decode failed for document[%d]: %w", i, err)
		}

		if ret != 0 {
			return nil, 0, fmt.Errorf("rerank: decode returned non-zero for document[%d]: %d", i, ret)
		}

		// Get the rank output.
		rawScore, err := llama.GetEmbeddingsSeq(pc.lctx, 0, int32(nClsOut))
		if err != nil {
			return nil, 0, fmt.Errorf("rerank: unable to get score for document[%d]: %w", i, err)
		}

		// Apply sigmoid to normalize score to [0, 1] range.
		var score float32
		if len(rawScore) > 0 {
			score = sigmoid(rawScore[0])
		}

		results[i] = RerankResult{
			Index:          i,
			RelevanceScore: score,
		}

		if returnDocuments {
			results[i].Document = doc
		}

		// Clear KV cache before next document.
		llama.MemoryClear(pc.mem, true)
	}

	return results, totalTokens, nil
}

// formatRerankPair formats a query-document pair for reranker models.
// Most BGE-style rerankers expect pairs without explicit prefixes.
func formatRerankPair(query, document string) string {
	return fmt.Sprintf("%s %s", query, document)
}

func rerankTokenLimit(batchTokens, contextTokens int) int {
	return min(batchTokens, contextTokens)
}

// sigmoid applies the sigmoid function to normalize a raw logit to [0, 1].
func sigmoid(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(x))))
}
