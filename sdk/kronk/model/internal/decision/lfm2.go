package decision

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type lfm2D1Protocol struct {
	finalTokenRunner
}

func newLFM2D1Protocol(m *Runtime) (Protocol, error) {
	runner, err := newFinalTokenRunner(m)
	if err != nil {
		return nil, err
	}
	p := lfm2D1Protocol{finalTokenRunner: runner}
	return &p, nil
}

func (p *lfm2D1Protocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	var state any
	if req.State != nil {
		text, err := decisionDeciderText(req.State, true, false)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
		}
		state = escapeDecisionSpecial(text)
	}
	questions := make([]finalTokenQuestion, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionDeciderOptions(question, true)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > MaxReadouts {
			return DecisionResponse{}, fmt.Errorf("%w: question %q has too many options", ErrDecisionRequest, question.ID)
		}
		labels, groups, err := decisionD1Labels(question.Type, options, func(index int) string {
			return fmt.Sprintf("%02d", index)
		}, func(text string) []llama.Token { return p.encode(text, false) })
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		values, err := deciderQuestionValues(state, question, options, labels)
		if err != nil {
			return DecisionResponse{}, err
		}
		questions[i] = finalTokenQuestion{values: values, options: options, groups: groups}
	}
	return p.run(ctx, req, questions, answerSystemOneQuestion)
}
