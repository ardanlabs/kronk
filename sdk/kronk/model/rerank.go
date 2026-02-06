package model

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

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
// When NSeqMax > 1, multiple concurrent requests can be processed in parallel,
// each using one context from the internal pool.
func (m *Model) Rerank(ctx context.Context, d D) (RerankResponse, error) {
	if !m.modelInfo.IsRerankModel {
		return RerankResponse{}, fmt.Errorf("rerank: model doesn't support reranking")
	}

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

	// Acquire a single context from the pool. This allows NSeqMax concurrent
	// requests to run in parallel, which is more important for server workloads
	// than parallelizing within a single request.
	pc, err := m.pool.acquire(ctx)
	if err != nil {
		return RerankResponse{}, err
	}
	defer m.pool.release(pc)

	results, totalPromptTokens, err := m.processRerank(ctx, pc, query, documents, returnDocuments)
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

// processRerank processes all documents on a single context.
func (m *Model) processRerank(ctx context.Context, pc poolContext, query string, documents []string, returnDocuments bool) ([]RerankResult, int, error) {
	maxTokens := int(llama.NUBatch(pc.lctx))
	ctxTokens := int(llama.NCtx(pc.lctx))
	if ctxTokens < maxTokens {
		maxTokens = ctxTokens
	}

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

// sigmoid applies the sigmoid function to normalize a raw logit to [0, 1].
func sigmoid(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(x))))
}
