package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type decision2Protocol struct {
	model    *Model
	template systemOneTemplate
	encode   func(string) []llama.Token
}

func newDecision2Protocol(m *Model) (decisionProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-decision2: %w", err)
	}
	if width := llama.ModelNEmbdOut(m.model); width != 1 {
		return nil, fmt.Errorf("init-decision2: invalid output embedding width %d", width)
	}
	p := decision2Protocol{model: m, template: tmpl, encode: func(text string) []llama.Token {
		return llama.Tokenize(m.vocab, text, false, true)
	}}
	return &p, nil
}

func (p *decision2Protocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	if _, ok := req.State.(string); !ok {
		data, err := marshalDecisionJSON(req.State)
		if err != nil {
			return DecisionResponse{}, err
		}
		if len(data) == 0 || data[0] != '{' && data[0] != '[' {
			return DecisionResponse{}, fmt.Errorf("%w: decision2 state must be text, an object or an array", ErrDecisionRequest)
		}
	}
	state, err := decisionDeciderText(req.State, false, true)
	if err != nil {
		return DecisionResponse{}, err
	}
	state, err = decision2Text(state)
	if err != nil {
		return DecisionResponse{}, err
	}
	work := make([]decisionWork, len(req.Questions))
	optionsByQuestion := make([][]systemOneOption, len(work))
	for i, question := range req.Questions {
		options, err := decisionDeciderOptions(question, false)
		if err != nil {
			return DecisionResponse{}, err
		}
		if len(options) < 2 || len(options) > decisionMaxReadouts {
			return DecisionResponse{}, fmt.Errorf("%w: decision2 question %q needs 2 to %d options", ErrDecisionRequest, question.ID, decisionMaxReadouts)
		}
		instructions, err := decisionDeciderText(question.Instructions, false, false)
		if err != nil {
			return DecisionResponse{}, err
		}
		instructions, err = decision2Text(instructions)
		if err != nil {
			return DecisionResponse{}, err
		}
		values := decisionDeciderQuestion(question, instructions, options, nil)
		// Hide HTML-sensitive characters from Jinja's tojson filter, whose
		// escaping differs from llama-server's. Keep response keys unchanged.
		for _, option := range values["options"].([]D) {
			for _, key := range []string{"key", "description"} {
				if text, ok := option[key].(string); ok {
					text, err = decision2Text(text)
					if err != nil {
						return DecisionResponse{}, err
					}
					option[key] = decision2Hide.Replace(text)
				}
			}
		}
		prompt, err := p.template.renderValues(D{
			"state": state, "questions": []D{values}, "images": []any{},
			"sep": clefSeparator, "mark_question": clefQuestionMark, "mark_option": clefOptionMark,
		})
		if err != nil {
			return DecisionResponse{}, err
		}
		item, err := decision2Work(decision2Show.Replace(prompt), question.Type, len(options), p.encode)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(item.tokens) > p.model.decision.contextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(item.tokens), p.model.decision.contextWindow)
		}
		work[i], optionsByQuestion[i] = item, options
	}
	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: decision2 readout: %w", err)
	}
	answers := make(map[string]DecisionAnswer, len(work))
	for i, question := range req.Questions {
		answer, err := answerDeciderQuestion(p.model, question, optionsByQuestion[i], readouts[i][0])
		if err != nil {
			return DecisionResponse{}, err
		}
		answers[question.ID] = answer
	}
	return DecisionResponse{Model: p.model.responseModelID(), Answers: answers, Usage: DecisionUsage{InputTokens: decisionInputTokens(work)}}, nil
}

var (
	decision2Hide = strings.NewReplacer("<", "\U0010FFF0", ">", "\U0010FFF1", "&", "\U0010FFF2", "\u2028", "\U0010FFF3", "\u2029", "\U0010FFF4")
	decision2Show = strings.NewReplacer("\U0010FFF0", "<", "\U0010FFF1", ">", "\U0010FFF2", "&", "\U0010FFF3", "\u2028", "\U0010FFF4", "\u2029")
)

func decision2Text(text string) (string, error) {
	if strings.ContainsAny(text, "\U0010FFF0\U0010FFF1\U0010FFF2\U0010FFF3\U0010FFF4") {
		return "", fmt.Errorf("%w: decision2 text contains reserved characters U+10FFF0 to U+10FFF4", ErrDecisionRequest)
	}
	return strings.ReplaceAll(escapeDecisionSpecial(text), clefMarker, "<<clef "), nil
}

func decision2Work(prompt string, typ DecisionQuestionType, n int, encode func(string) []llama.Token) (decisionWork, error) {
	item := decisionWork{jointScores: n}
	questions, options := 0, 0
	for piece := range strings.SplitSeq(prompt, clefSeparator) {
		order := llama.DecisionOrderNone
		switch {
		case strings.HasPrefix(piece, clefQuestionMark):
			piece = strings.TrimPrefix(piece, clefQuestionMark)
			order = clefQuestionOrder(typ)
			questions++
			if questions != 1 || options != n {
				return decisionWork{}, fmt.Errorf("unexpected decision2 question marker layout")
			}
		case strings.HasPrefix(piece, clefOptionMark):
			piece = strings.TrimPrefix(piece, clefOptionMark)
			order = llama.DecisionOrderOption
			options++
			if questions != 0 || options > n {
				return decisionWork{}, fmt.Errorf("unexpected decision2 option marker layout")
			}
		}
		tokens := encode(piece)
		if order != llama.DecisionOrderNone && len(tokens) == 0 {
			return decisionWork{}, fmt.Errorf("empty decision2 marked span")
		}
		item.tokens = append(item.tokens, tokens...)
		for range tokens {
			item.decisionOrder = append(item.decisionOrder, order)
		}
	}
	if questions != 1 || options != n {
		return decisionWork{}, fmt.Errorf("unexpected decision2 marker count")
	}
	return item, nil
}
