package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const decisionLevRatings = 9

type levProtocol struct {
	model        *Model
	template     systemOneTemplate
	labelTokens  []llama.Token
	labelStrings []string
}

type levPass struct {
	question int
	reversed bool
}

func newLevProtocol(m *Model) (*levProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-lev: %w", err)
	}

	labelTokens, labelStrings := decisionLabelTokens(m.vocab)
	if len(labelTokens) < decisionLevRatings {
		return nil, fmt.Errorf("init-lev: tokenizer provides %d single-token labels, need at least %d", len(labelTokens), decisionLevRatings)
	}

	return &levProtocol{
		model:        m,
		template:     tmpl,
		labelTokens:  labelTokens,
		labelStrings: labelStrings,
	}, nil
}

func (p *levProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionSortedText(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	var passes []levPass
	var work []decisionWork
	for i, question := range req.Questions {
		options, err := decisionSystemOneOptions(question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > len(p.labelTokens) {
			return DecisionResponse{}, fmt.Errorf("decision: question %q has %d options, lev supports at most %d", question.ID, len(options), len(p.labelTokens))
		}
		instructions, err := decisionSortedText(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		optionsByQuestion[i] = options

		variants := 1
		if question.Type == DecisionQuestionTypeChoice && len(options) > 1 {
			variants = 2
		}
		for variant := range variants {
			variantOptions := append([]systemOneOption(nil), options...)
			if variant == 1 {
				reverseSystemOneOptions(variantOptions)
			}
			for j := range variantOptions {
				variantOptions[j].label = p.labelStrings[j]
			}
			prompt, err := p.template.render(question, state, instructions, variantOptions)
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

			outputs := len(options)
			if question.Type == DecisionQuestionTypeNoul {
				outputs = decisionLevRatings
			}
			work = append(work, decisionWork{
				tokens:    tokens,
				prefixLen: len(tokens) - 1,
				readouts: []decisionReadout{{
					position:   len(tokens) - 1,
					candidates: append([]llama.Token(nil), p.labelTokens[:outputs]...),
				}},
			})
			passes = append(passes, levPass{question: i, reversed: variant == 1})
		}
	}

	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: lev readout: %w", err)
	}

	probabilitySums := make([][]float64, len(req.Questions))
	variantCounts := make([]int, len(req.Questions))
	for i, pass := range passes {
		question := req.Questions[pass.question]
		temperature, err := decisionTemperature(p.model, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		probabilities, err := decisionSoftmax(readouts[i][0], temperature)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if pass.reversed {
			reverseFloat64s(probabilities)
		}
		if probabilitySums[pass.question] == nil {
			probabilitySums[pass.question] = make([]float64, len(probabilities))
		}
		for j, probability := range probabilities {
			probabilitySums[pass.question][j] += probability
		}
		variantCounts[pass.question]++
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		probabilities := probabilitySums[i]
		for j := range probabilities {
			probabilities[j] /= float64(variantCounts[i])
		}
		if question.Type == DecisionQuestionTypeNoul {
			var expected float64
			for j, probability := range probabilities {
				expected += probability * float64(j) / float64(len(probabilities)-1)
			}
			answers[question.ID] = DecisionAnswer{Type: DecisionQuestionTypeNoul, Noul: roundFour(expected)}
			continue
		}
		answers[question.ID] = answerSystemOneQuestion(question, optionsByQuestion[i], probabilities)
	}

	return DecisionResponse{
		Model:   p.model.responseModelID(),
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: decisionInputTokens(work)},
	}, nil
}

func reverseSystemOneOptions(options []systemOneOption) {
	for left, right := 0, len(options)-1; left < right; left, right = left+1, right-1 {
		options[left], options[right] = options[right], options[left]
	}
}

func reverseFloat64s(values []float64) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
