package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/decision"
)

func initDecisionProtocol(m *Model) error {
	protocol, err := decision.New(decision.Runtime{
		Model: m.model, Vocab: m.vocab,
		ModelID: m.responseModelID(), Protocol: m.modelInfo.decisionProtocol.String(),
		AddBOSToken: m.addBOSToken, ContextWindow: m.decision.contextWindow,
		Calibration: m.modelInfo.profile.Decision,
		Run:         m.runDecisionWork,
		RenderChat: func(ctx context.Context, prompt string) (string, error) {
			return m.applyJinjaTemplate(ctx, D{
				"messages":             []D{{"role": RoleUser, "content": prompt}},
				"chat_template_kwargs": D{"enable_thinking": false},
			})
		},
		PrepareMedia: m.prepareProtocolMedia,
	})
	if err != nil {
		return err
	}
	m.protocol = protocol
	return nil
}

// Protocol plans contain only request-owned data. Native staging, scheduling,
// and context ownership remain in the model engine.
func (m *Model) runDecisionWork(ctx context.Context, plans []decision.Work) ([][][]float32, error) {
	work := make([]decisionWork, len(plans))
	for i, plan := range plans {
		readouts := make([]decisionReadout, len(plan.Readouts))
		for j, readout := range plan.Readouts {
			readouts[j] = decisionReadout{position: readout.Position, candidates: readout.Candidates, embeddingWidth: readout.EmbeddingWidth}
		}
		prefix := make([]extendedBatchEntry, len(plan.PrefixRows))
		for j, row := range plan.PrefixRows {
			prefix[j] = extendedBatchEntry{
				token: row.Token, hasToken: row.HasToken,
				embedding: row.Embedding, embdWidth: len(row.Embedding),
				positions: row.Positions, positionCount: row.PositionCount,
			}
		}
		work[i] = decisionWork{
			tokens: plan.Tokens, prefixRows: prefix, position: plan.Position,
			prefixLen: plan.PrefixLen, readouts: readouts,
			decisionOrder: plan.DecisionOrder, jointScores: plan.JointScores,
		}
	}
	return m.decision.run(ctx, work)
}

func (m *Model) prepareProtocolMedia(ctx context.Context, prompt string, images [][]byte) ([]decision.PrefixRow, int, error) {
	rows, position, err := m.prepareDecisionMedia(ctx, prompt, images)
	if err != nil {
		return nil, 0, err
	}
	prefix := make([]decision.PrefixRow, len(rows))
	for i, row := range rows {
		prefix[i] = decision.PrefixRow{
			Token: row.token, HasToken: row.hasToken, Embedding: row.embedding,
			Positions: row.positions, PositionCount: row.positionCount,
		}
	}
	return prefix, position, nil
}

// Decision evaluates a set of decision questions against shared state.
func (m *Model) Decision(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	if m.protocol == nil {
		return DecisionResponse{}, fmt.Errorf("decision: protocol was not detected for model %q; configure model.WithDecisionProtocol(...) with the model's protocol", m.modelInfo.ID)
	}
	if err := validateDecisionRequest(req, m.modelInfo.decisionProtocol); err != nil {
		return DecisionResponse{}, err
	}
	if len(req.Images) > 0 {
		media, ok := m.protocol.(decision.ImageProtocol)
		if !ok || !media.SupportsImages() {
			return DecisionResponse{}, fmt.Errorf("%w: decision protocol %q does not support images", ErrDecisionRequest, m.modelInfo.decisionProtocol)
		}
		if m.mtmdMetaCtx == 0 || !m.mtmdCapabilities.Vision {
			return DecisionResponse{}, fmt.Errorf("%w: decision images require a compatible vision projector", ErrDecisionRequest)
		}
	}
	return m.protocol.Decide(ctx, req)
}
