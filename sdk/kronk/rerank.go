package kronk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Rerank provides support to interact with a reranker model.
//
// Supported options in d:
//   - query (string): the query to rank documents against (required)
//   - documents ([]string): the documents to rank (required)
//   - top_n (int): return only the top N results (optional, default: all)
//   - return_documents (bool): include document text in results (default: false)
//
// Reranking calls are processed sequentially, with one forward pass per document.
func (krn *Kronk) Rerank(ctx context.Context, d model.D) (model.RerankResponse, error) {
	if !krn.ModelInfo().IsRerankModel {
		return model.RerankResponse{}, fmt.Errorf("rerank: model doesn't support reranking")
	}

	if _, exists := ctx.Deadline(); !exists {
		return model.RerankResponse{}, fmt.Errorf("rerank: context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.RerankResponse, error) {
		return m.Rerank(ctx, d)
	}

	return nonStreaming(ctx, krn, f)
}

// RerankHTTP provides http handler support for a rerank call.
func (krn *Kronk) RerankHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.RerankResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.RerankResponse{}, fmt.Errorf("rerank-http: context has no deadline, provide a reasonable timeout")
	}

	resp, err := krn.Rerank(ctx, d)
	if err != nil {
		return model.RerankResponse{}, fmt.Errorf("rerank-http: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return resp, fmt.Errorf("rerank-http: marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return resp, nil
}
