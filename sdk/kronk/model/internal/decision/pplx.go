package decision

import (
	"context"
	"fmt"
)

type pplxDeciderProtocol struct {
	labelDecider
}

func newPplxDeciderProtocol(m *Runtime) (Protocol, error) {
	decider, err := newLabelDecider(m)
	if err != nil {
		return nil, err
	}
	p := pplxDeciderProtocol{labelDecider: decider}
	return &p, nil
}

func (p *pplxDeciderProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	// PPLX serializes even a nil state as JSON text.
	text, err := decisionDeciderText(req.State, false, false)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}
	questions, err := p.questions(req, escapeDecisionSpecial(text))
	if err != nil {
		return DecisionResponse{}, err
	}
	return p.run(ctx, req, questions, answerSystemOneQuestion)
}
