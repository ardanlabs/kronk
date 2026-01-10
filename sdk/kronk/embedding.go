package kronk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Embeddings provides support to interact with an embedding model.
func (krn *Kronk) Embeddings(ctx context.Context, input string) (model.EmbedReponse, error) {
	if !krn.ModelInfo().IsEmbedModel {
		return model.EmbedReponse{}, fmt.Errorf("embed:model doesn't support embedding")
	}

	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embed:context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.EmbedReponse, error) {
		return m.Embeddings(ctx, input)
	}

	return nonStreaming(ctx, krn, f)
}

// EmbeddingsHTTP provides http handler support for an embeddings call.
func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.EmbedReponse{}, fmt.Errorf("embeddings:context has no deadline, provide a reasonable timeout")
	}

	var input string
	inputReq, ok := d["input"].(string)
	if ok {
		input = inputReq
	}

	if input == "" {
		return model.EmbedReponse{}, fmt.Errorf("embeddings:missing input parameter")
	}

	resp, err := krn.Embeddings(ctx, input)
	if err != nil {
		return model.EmbedReponse{}, fmt.Errorf("chat-streaming-http:stream-response: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return resp, fmt.Errorf("chat-streaming-http:marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return resp, nil
}
