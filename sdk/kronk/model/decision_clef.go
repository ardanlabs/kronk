package model

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	clefMarker       = "<<clef:"
	clefSeparator    = "<<clef:sep>>"
	clefQuestionMark = "<<clef:question>>"
	clefOptionMark   = "<<clef:option>>"
)

type clefProtocol struct {
	model    *Model
	template systemOneTemplate
}

func newClefProtocol(m *Model) (*clefProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-clef: %w", err)
	}
	if width := llama.ModelNEmbdOut(m.model); width != 1 {
		return nil, fmt.Errorf("init-clef: invalid output embedding width %d", width)
	}
	return &clefProtocol{model: m, template: tmpl}, nil
}

func (p *clefProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionTemplateValue(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}
	state = replaceDecisionTemplateText(state, clefMarker, "<<clef ")

	questions := make([]D, len(req.Questions))
	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	totalOptions := 0
	for i, question := range req.Questions {
		options, err := decisionRawSystemOneOptions(question, true)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if question.Type == DecisionQuestionTypeChoice {
			sort.Slice(options, func(i, j int) bool { return options[i].name < options[j].name })
		}
		instructions, err := decisionTemplateValue(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		instructions = replaceDecisionTemplateText(instructions, clefMarker, "<<clef ")
		for j := range options {
			options[j].name = strings.ReplaceAll(options[j].name, clefMarker, "<<clef ")
			options[j].description = replaceDecisionTemplateText(options[j].description, clefMarker, "<<clef ")
		}
		questions[i] = decisionTemplateQuestion(question, instructions, options)
		optionsByQuestion[i] = make([]systemOneOption, len(options))
		for j, option := range options {
			optionsByQuestion[i][j] = systemOneOption{name: option.name}
		}
		totalOptions += len(options)
	}
	if totalOptions > decisionMaxReadouts {
		return DecisionResponse{}, fmt.Errorf("decision: clef needs %d option scores, limit is %d", totalOptions, decisionMaxReadouts)
	}

	prompt, err := p.template.renderValues(D{
		"state":         state,
		"questions":     questions,
		"sep":           clefSeparator,
		"mark_question": clefQuestionMark,
		"mark_option":   clefOptionMark,
	})
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: %w", err)
	}

	var tokens []llama.Token
	var orders []llama.DecisionOrder
	questionIndex := 0
	optionCount := 0
	for piece := range strings.SplitSeq(prompt, clefSeparator) {
		order := llama.DecisionOrderNone
		switch {
		case strings.HasPrefix(piece, clefQuestionMark):
			piece = strings.TrimPrefix(piece, clefQuestionMark)
			if questionIndex >= len(req.Questions) {
				return DecisionResponse{}, fmt.Errorf("decision: unexpected layout of the decision prompt")
			}
			order = clefQuestionOrder(req.Questions[questionIndex].Type)
			questionIndex++
		case strings.HasPrefix(piece, clefOptionMark):
			piece = strings.TrimPrefix(piece, clefOptionMark)
			order = llama.DecisionOrderOption
			optionCount++
		}

		pieceTokens := llama.Tokenize(p.model.vocab, piece, false, true)
		if order != llama.DecisionOrderNone && len(pieceTokens) == 0 {
			return DecisionResponse{}, fmt.Errorf("decision: instructions and options must not be empty")
		}
		tokens = append(tokens, pieceTokens...)
		for range pieceTokens {
			orders = append(orders, order)
		}
	}
	if questionIndex != len(req.Questions) || optionCount != totalOptions {
		return DecisionResponse{}, fmt.Errorf("decision: unexpected layout of the decision prompt")
	}
	if len(tokens) == 0 {
		return DecisionResponse{}, fmt.Errorf("decision: tokenize prompt: no tokens")
	}
	if len(tokens) > p.model.decision.contextWindow {
		return DecisionResponse{}, fmt.Errorf("decision: %w: input needs %d tokens, limit is %d", ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
	}

	work := []decisionWork{{tokens: tokens, decisionOrder: orders, jointScores: totalOptions}}
	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: clef readout: %w", err)
	}
	if len(readouts) != 1 || len(readouts[0]) != 1 || len(readouts[0][0]) != totalOptions {
		return DecisionResponse{}, fmt.Errorf("decision: clef returned an unexpected score layout")
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	offset := 0
	for i, question := range req.Questions {
		n := len(optionsByQuestion[i])
		temperature, err := decisionTemperature(p.model, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		probabilities, err := decisionSoftmax(readouts[0][0][offset:offset+n], temperature)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		answers[question.ID] = answerSystemOneQuestion(question, optionsByQuestion[i], probabilities)
		offset += n
	}

	return DecisionResponse{
		Model:   p.model.responseModelID(),
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: len(tokens)},
	}, nil
}

func clefQuestionOrder(questionType DecisionQuestionType) llama.DecisionOrder {
	switch questionType {
	case DecisionQuestionTypeNoul:
		return llama.DecisionOrderQuestionNoul
	case DecisionQuestionTypeChoice:
		return llama.DecisionOrderQuestionChoice
	case DecisionQuestionTypeScore:
		return llama.DecisionOrderQuestionScore
	default:
		panic("validated decision question has unknown type")
	}
}

func replaceDecisionTemplateText(value any, old, replacement string) any {
	switch value := value.(type) {
	case string:
		return strings.ReplaceAll(value, old, replacement)
	case []any:
		for i := range value {
			value[i] = replaceDecisionTemplateText(value[i], old, replacement)
		}
	case map[string]any:
		for key := range value {
			value[key] = replaceDecisionTemplateText(value[key], old, replacement)
		}
	}
	return value
}
