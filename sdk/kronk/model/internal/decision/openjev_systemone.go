package decision

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type systemOneOpenJEVProtocol struct {
	runtime      *Runtime
	template     systemOneTemplate
	letterTokens []llama.Token
}

func newSystemOneOpenJEVProtocol(m *Runtime) (*systemOneOpenJEVProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-openjev: %w", err)
	}

	letterTokens := make([]llama.Token, len(openJEVLetters))
	for i, letter := range openJEVLetters {
		tokens := llama.Tokenize(m.Vocab, string(letter), false, false)
		if len(tokens) != 1 {
			return nil, fmt.Errorf("init-openjev: label %q is not a single token", letter)
		}
		letterTokens[i] = tokens[0]
	}

	return &systemOneOpenJEVProtocol{
		runtime:      m,
		template:     tmpl,
		letterTokens: letterTokens,
	}, nil
}

func (p *systemOneOpenJEVProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionSystemOneText(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	work := make([]Work, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionSystemOneOptions(question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if question.Type == DecisionQuestionTypeNoul {
			options[0], options[1] = options[1], options[0]
		}
		if len(options) > len(p.letterTokens) {
			return DecisionResponse{}, fmt.Errorf("decision: question %q has %d options, openjev supports at most %d", question.ID, len(options), len(p.letterTokens))
		}

		instructions, err := decisionSystemOneText(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		prompt, err := p.template.render(question, state, instructions, options)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		tokens := llama.Tokenize(p.runtime.Vocab, prompt, false, true)
		if len(tokens) == 0 {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: tokenize prompt: no tokens", question.ID)
		}
		if len(tokens) > p.runtime.ContextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(tokens), p.runtime.ContextWindow)
		}

		optionsByQuestion[i] = options
		work[i] = Work{
			Tokens:    tokens,
			PrefixLen: len(tokens) - 1,
			Readouts: []Readout{{
				Position:   len(tokens) - 1,
				Candidates: append([]llama.Token(nil), p.letterTokens[:len(options)]...),
			}},
		}
	}

	readouts, err := p.runtime.Run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: openjev readout: %w", err)
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		temperature, err := decisionTemperature(p.runtime, question)
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
		Model:   p.runtime.ModelID,
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: decisionInputTokens(work)},
	}, nil
}
