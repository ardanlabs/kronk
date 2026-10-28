package decision

import (
	"context"
	"fmt"
)

type d3Protocol struct {
	labelDecider
}

func newD3Protocol(m *Runtime) (Protocol, error) {
	decider, err := newLabelDecider(m)
	if err != nil {
		return nil, err
	}
	p := d3Protocol{labelDecider: decider}
	return &p, nil
}

func (p *d3Protocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	// D3 leaves nil state and omitted instructions for its template defaults.
	var state any
	if req.State != nil {
		text, err := decisionDeciderText(req.State, false, false)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
		}
		state = escapeDecisionSpecial(text)
	}
	questions, err := p.questions(req, state)
	if err != nil {
		return DecisionResponse{}, err
	}
	return p.run(ctx, req, questions, answerEntropyQuestion)
}
