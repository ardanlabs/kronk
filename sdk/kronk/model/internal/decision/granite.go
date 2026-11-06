package decision

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type graniteSwitchProtocol struct {
	finalTokenRunner
	labelTokens []llama.Token
	labels      []string
}

func newGraniteSwitchProtocol(m *Runtime) (Protocol, error) {
	runner, err := newFinalTokenRunner(m)
	if err != nil {
		return nil, err
	}
	tokens, labels := decisionAlphabetLabels(func(text string) []llama.Token {
		return runner.encode(text, false)
	})
	if len(labels) == 0 {
		return nil, fmt.Errorf("init-granite-switch: tokenizer provides no single-token labels")
	}
	p := graniteSwitchProtocol{
		finalTokenRunner: runner, labelTokens: tokens, labels: labels,
	}
	return &p, nil
}

func (p *graniteSwitchProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionJinjaValue(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}
	if state.IsNone() {
		return DecisionResponse{}, fmt.Errorf("%w: granite-switch requires state", ErrDecisionRequest)
	}
	questions := make([]finalTokenQuestion, len(req.Questions))
	for i, question := range req.Questions {
		options := question.Options
		switch question.Type {
		case DecisionQuestionTypeScore:
			options = make([]DecisionOption, len(question.Levels))
			for j, level := range question.Levels {
				options[j] = DecisionQuestionOpt(strconv.Itoa(j), level)
			}
		case DecisionQuestionTypeNoul:
			var falseValue, trueValue any
			if question.NoulCriteria != nil {
				falseValue, trueValue = question.NoulCriteria.False, question.NoulCriteria.True
			}
			options = []DecisionOption{DecisionQuestionOpt("true", trueValue), DecisionQuestionOpt("false", falseValue)}
		}
		if len(options) > len(p.labels) {
			return DecisionResponse{}, fmt.Errorf("%w: question %q needs %d labels, model provides %d", ErrDecisionRequest, question.ID, len(options), len(p.labels))
		}
		raw := make([]D, len(options))
		prepared := finalTokenQuestion{options: make([]systemOneOption, len(options))}
		for j, option := range options {
			raw[j] = D{"key": option.Name, "description": option.Description, "label": p.labels[j]}
			prepared.options[j] = systemOneOption{name: option.Name}
			prepared.groups = append(prepared.groups, []llama.Token{p.labelTokens[j]})
		}
		values, err := decisionJinjaValue(D{
			"id": question.ID, "type": question.Type.String(),
			"instructions": question.Instructions, "options": raw,
		})
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		prepared.values = D{"state": state, "images": []any{}}
		for _, key := range values.AsDict().OrderedKeys() {
			prepared.values[key], _ = values.AsDict().Get(key)
		}
		questions[i] = prepared
	}
	return p.run(ctx, req, questions, answerSystemOneQuestion)
}
