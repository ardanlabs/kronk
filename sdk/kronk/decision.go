package kronk

import (
	"context"
	"fmt"

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
