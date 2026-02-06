package kronk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Tokenize returns the token count for a text input.
//
// Supported options in d:
//   - input (string): the text to tokenize (required)
//   - apply_template (bool): if true, wrap input as a user message and apply
//     the model's chat template before tokenizing (default: false)
//   - add_generation_prompt (bool): when apply_template is true, controls whether
//     the assistant role prefix is appended to the prompt (default: true)
//
// When apply_template is true, the returned count includes all template
// overhead (role markers, separators, generation prompt). This reflects the
// actual number of tokens that would be fed to the model.
func (krn *Kronk) Tokenize(ctx context.Context, d model.D) (model.TokenizeResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.TokenizeResponse{}, fmt.Errorf("tokenize: context has no deadline, provide a reasonable timeout")
	}

	f := func(m *model.Model) (model.TokenizeResponse, error) {
		return m.Tokenize(ctx, d)
	}

	return nonStreaming(ctx, krn, f)
}

// TokenizeHTTP provides http handler support for a tokenize call.
func (krn *Kronk) TokenizeHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.TokenizeResponse, error) {
	if _, exists := ctx.Deadline(); !exists {
		return model.TokenizeResponse{}, fmt.Errorf("tokenize-http: context has no deadline, provide a reasonable timeout")
	}

	resp, err := krn.Tokenize(ctx, d)
	if err != nil {
		return model.TokenizeResponse{}, fmt.Errorf("tokenize-http: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return resp, fmt.Errorf("tokenize-http: marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return resp, nil
}
