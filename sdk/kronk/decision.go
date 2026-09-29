package kronk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Decision evaluates typed questions against shared state using a supported
// decision model.
func (krn *Kronk) Decision(ctx context.Context, req model.DecisionRequest) (model.DecisionResponse, error) {
	if !krn.ModelInfo().IsDecisionModel {
		return model.DecisionResponse{}, fmt.Errorf("decision: protocol was not detected for model %q; configure model.WithDecisionProtocol(...) with the model's protocol", krn.ModelID())
	}

	call := func(m *model.Model) (model.DecisionResponse, error) {
		return m.Decision(ctx, req)
	}

	return nonStreaming(ctx, krn, call)
}

// DecisionHTTP provides HTTP handler support for a decision call.
func (krn *Kronk) DecisionHTTP(ctx context.Context, log Logger, w http.ResponseWriter, req model.DecisionRequest) (model.DecisionResponse, error) {
	resp, err := krn.Decision(ctx, req)
	if err != nil {
		return model.DecisionResponse{}, fmt.Errorf("decision-http: %w", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return resp, fmt.Errorf("decision-http: marshal: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	return resp, nil
}
