package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type nimbleProtocol struct {
	model        *Model
	template     systemOneTemplate
	labelTokens  []llama.Token
	labelStrings []string
}

func newNimbleProtocol(m *Model) (*nimbleProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-nimble: %w", err)
	}
	labelTokens, labelStrings := decisionLabelTokens(m.vocab)
	if len(labelTokens) < 2 {
		return nil, fmt.Errorf("init-nimble: tokenizer provides %d single-token labels, need at least 2", len(labelTokens))
	}
	return &nimbleProtocol{model: m, template: tmpl, labelTokens: labelTokens, labelStrings: labelStrings}, nil
}

func (p *nimbleProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionTemplateValue(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	questions := make([]D, len(req.Questions))
	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionRawSystemOneOptions(question, false)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > len(p.labelTokens) {
			return DecisionResponse{}, fmt.Errorf("decision: question %q has %d options, nimble supports at most %d", question.ID, len(options), len(p.labelTokens))
		}
		for j := range options {
			options[j].label = p.labelStrings[j]
		}
		instructions, err := decisionTemplateValue(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		questions[i] = decisionTemplateQuestion(question, instructions, options)
		optionsByQuestion[i] = make([]systemOneOption, len(options))
		for j, option := range options {
			optionsByQuestion[i][j] = systemOneOption{name: option.name, label: option.label}
		}
	}

	work := make([]decisionWork, len(req.Questions))
	for i, question := range req.Questions {
		prompt, err := p.template.renderValues(D{
			"id":           question.ID,
			"type":         string(question.Type),
			"instructions": questions[i]["instructions"],
			"state":        state,
			"options":      questions[i]["options"],
			"questions":    questions,
			"images":       []string{},
		})
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		tokens := llama.Tokenize(p.model.vocab, prompt, false, true)
		if len(tokens) == 0 {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: tokenize prompt: no tokens", question.ID)
		}
		if len(tokens) > p.model.decision.contextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
		}
		work[i] = decisionWork{
			tokens:    tokens,
			prefixLen: len(tokens) - 1,
			readouts: []decisionReadout{{
				position:   len(tokens) - 1,
				candidates: append([]llama.Token(nil), p.labelTokens[:len(optionsByQuestion[i])]...),
			}},
		}
	}

	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: nimble readout: %w", err)
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		temperature, err := decisionTemperature(p.model, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		probabilities, err := decisionSoftmax(readouts[i][0], temperature)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		answers[question.ID] = answerSystemOneQuestion(question, optionsByQuestion[i], probabilities)
	}

	return DecisionResponse{
		Model:   p.model.responseModelID(),
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: decisionInputTokens(work)},
	}, nil
}

type decisionRawOption struct {
	name        string
	description any
	label       string
}

func decisionTemplateQuestion(question DecisionQuestion, instructions any, options []decisionRawOption) D {
	values := make([]D, len(options))
	for i, option := range options {
		values[i] = D{"key": option.name, "description": option.description, "label": option.label}
	}
	return D{
		"id":           question.ID,
		"type":         string(question.Type),
		"instructions": instructions,
		"options":      values,
	}
}

func decisionRawSystemOneOptions(question DecisionQuestion, trueFirst bool) ([]decisionRawOption, error) {
	var values []struct {
		name        string
		description any
	}
	switch question.Type {
	case DecisionQuestionTypeChoice:
		for _, option := range question.Options {
			values = append(values, struct {
				name        string
				description any
			}{option.Name, option.Description})
		}
	case DecisionQuestionTypeScore:
		for i, level := range question.Levels {
			values = append(values, struct {
				name        string
				description any
			}{fmt.Sprint(i), level})
		}
	case DecisionQuestionTypeNoul:
		var falseValue, trueValue any
		if question.NoulCriteria != nil {
			falseValue = question.NoulCriteria.False
			trueValue = question.NoulCriteria.True
		}
		values = append(values,
			struct {
				name        string
				description any
			}{"false", falseValue},
			struct {
				name        string
				description any
			}{"true", trueValue},
		)
		if trueFirst {
			values[0], values[1] = values[1], values[0]
		}
	}

	options := make([]decisionRawOption, len(values))
	for i, value := range values {
		description, err := decisionTemplateValue(value.description)
		if err != nil {
			return nil, err
		}
		options[i] = decisionRawOption{name: value.name, description: description}
	}
	return options, nil
}
