package kronk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Embeddings provides support to interact with an embedding model.
//
// Supported options in d:
//   - input (string): the text to embed (required)
//   - truncate (bool): if true, truncate input to fit context window (default: false)
//   - truncate_direction (string): "right" (default) or "left"
//   - dimensions (int): reduce output to first N dimensions (for Matryoshka models)
//
// Embedding calls are processed sequentially (llama.cpp only supports sequence 0
// for embedding extraction). Batch multiple texts in the input parameter for
// better performance.
func (krn *Kronk) Embeddings(ctx context.Context, d model.D) (model.EmbedReponse, error) {
	if !krn.ModelInfo().IsEmbedModel {
		return model.EmbedReponse{}, fmt.Errorf("embeddings: model doesn't support embedding")
	}

	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embeddings: context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.EmbedReponse, error) {
		return m.Embeddings(ctx, d)
	}

	return nonStreaming(ctx, krn, f)
}

// EmbeddingsHTTP provides http handler support for an embeddings call.
func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embeddings-http: context has no deadline, provide a reasonable timeout")
	}

	resp, err := krn.Embeddings(ctx, d)
	if err != nil {
		return model.EmbedReponse{}, fmt.Errorf("embeddings-http: stream-response: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return resp, fmt.Errorf("embeddings-http: marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return resp, nil
}
