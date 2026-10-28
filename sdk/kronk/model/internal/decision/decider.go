package decision

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// finalTokenRunner executes prepared final-token questions without knowing
// their protocol. Protocols own template values, candidate groups and answers.
type finalTokenRunner struct {
	runtime  *Runtime
	template systemOneTemplate
	encode   func(string, bool) []llama.Token
}

type finalTokenQuestion struct {
	values  D
	options []systemOneOption
	groups  [][]llama.Token
}

func newFinalTokenRunner(m *Runtime) (finalTokenRunner, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return finalTokenRunner{}, fmt.Errorf("init-decider: %w", err)
	}
	r := finalTokenRunner{runtime: m, template: tmpl, encode: func(text string, special bool) []llama.Token {
		return llama.Tokenize(m.Vocab, text, false, special)
	}}
	return r, nil
}

func (r *finalTokenRunner) run(ctx context.Context, req DecisionRequest, questions []finalTokenQuestion, answer func(DecisionQuestion, []systemOneOption, []float64) DecisionAnswer) (DecisionResponse, error) {
	work := make([]Work, len(questions))
	for i, question := range questions {
		prompt, err := r.template.renderValues(question.values)
		if err != nil {
			return DecisionResponse{}, err
		}
		tokens := r.encode(prompt, true)
		if len(tokens) > r.runtime.ContextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", req.Questions[i].ID, ErrDecisionBudget, len(tokens), r.runtime.ContextWindow)
		}
		var candidates []llama.Token
		for _, group := range question.groups {
			candidates = append(candidates, group...)
		}
		work[i] = Work{Tokens: tokens, Readouts: []Readout{{Position: len(tokens) - 1, Candidates: candidates}}}
	}
	readouts, err := r.runtime.Run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: decider readout: %w", err)
	}
	answers := make(map[string]DecisionAnswer, len(work))
	for i, question := range questions {
		var scores []float32
		offset := 0
		for _, group := range question.groups {
			scores = append(scores, slices.Max(readouts[i][0][offset:offset+len(group)]))
			offset += len(group)
		}
		probabilities, err := calibratedDecisionProbabilities(r.runtime, req.Questions[i], scores)
		if err != nil {
			return DecisionResponse{}, err
		}
		answers[req.Questions[i].ID] = answer(req.Questions[i], question.options, probabilities)
	}
	return DecisionResponse{Model: r.runtime.ModelID, Answers: answers, Usage: DecisionUsage{InputTokens: decisionInputTokens(work)}}, nil
}

// labelDecider shares model-declared label preparation between PPLX and D3.
// It does not choose how either protocol serializes state or interprets answers.
type labelDecider struct {
	finalTokenRunner
	labelTokens []llama.Token
	labels      []string
}

func newLabelDecider(m *Runtime) (labelDecider, error) {
	runner, err := newFinalTokenRunner(m)
	if err != nil {
		return labelDecider{}, err
	}
	tokens, labels, err := m.decisionLabelTokens()
	if err != nil {
		return labelDecider{}, fmt.Errorf("init-decider: %w", err)
	}
	if len(labels) < 9 {
		return labelDecider{}, fmt.Errorf("init-decider: tokenizer provides %d single-token labels, need at least 9", len(labels))
	}
	d := labelDecider{finalTokenRunner: runner, labelTokens: tokens, labels: labels}
	return d, nil
}

func (d *labelDecider) questions(req DecisionRequest, state any) ([]finalTokenQuestion, error) {
	questions := make([]finalTokenQuestion, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionDeciderOptions(question, false)
		if err != nil {
			return nil, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > MaxReadouts {
			return nil, fmt.Errorf("%w: question %q has too many options", ErrDecisionRequest, question.ID)
		}
		if len(options) > len(d.labels) {
			return nil, fmt.Errorf("%w: question %q needs %d labels, model provides %d", ErrDecisionRequest, question.ID, len(options), len(d.labels))
		}
		values, err := deciderQuestionValues(state, question, options, d.labels[:len(options)])
		if err != nil {
			return nil, err
		}
		var groups [][]llama.Token
		for _, token := range d.labelTokens[:len(options)] {
			groups = append(groups, []llama.Token{token})
		}
		questions[i] = finalTokenQuestion{values: values, options: options, groups: groups}
	}
	return questions, nil
}

func decisionDeciderOptions(question DecisionQuestion, trueFirst bool) ([]systemOneOption, error) {
	raw := question.Options
	switch question.Type {
	case DecisionQuestionTypeScore:
		raw = make([]DecisionOption, len(question.Levels))
		for i, level := range question.Levels {
			raw[i] = DecisionQuestionOpt(strconv.Itoa(i), level)
		}
	case DecisionQuestionTypeNoul:
		var falseValue, trueValue any
		if question.NoulCriteria != nil {
			falseValue, trueValue = question.NoulCriteria.False, question.NoulCriteria.True
		}
		raw = []DecisionOption{DecisionQuestionOpt("false", falseValue), DecisionQuestionOpt("true", trueValue)}
		if trueFirst {
			slices.Reverse(raw)
		}
	}
	options := make([]systemOneOption, len(raw))
	for i, option := range raw {
		text := ""
		if option.Description != nil {
			var err error
			text, err = decisionDeciderText(option.Description, false, false)
			if err != nil {
				return nil, err
			}
		}
		options[i] = systemOneOption{name: option.Name, description: text}
	}
	return options, nil
}

func deciderQuestionValues(state any, question DecisionQuestion, options []systemOneOption, labels []string) (D, error) {
	instructions := ""
	if question.Instructions != nil {
		var err error
		instructions, err = decisionDeciderText(question.Instructions, false, false)
		if err != nil {
			return nil, err
		}
	}
	values := decisionDeciderQuestion(question, instructions, options, labels)
	values["state"], values["images"] = state, []any{}
	return values, nil
}

func decisionDeciderQuestion(question DecisionQuestion, instructions string, options []systemOneOption, labels []string) D {
	values := make([]decisionRawOption, len(options))
	for i, option := range options {
		var description any
		if option.description != "" {
			description = escapeDecisionSpecial(option.description)
		}
		label := ""
		if labels != nil {
			label = labels[i]
		}
		values[i] = decisionRawOption{name: escapeDecisionSpecial(option.name), description: description, label: label}
	}
	return decisionTemplateQuestion(question, escapeDecisionSpecial(instructions), values)
}

var decisionSpecialPattern = regexp.MustCompile(`<\|([A-Za-z0-9_]+)\|>`)

func escapeDecisionSpecial(text string) string {
	return decisionSpecialPattern.ReplaceAllString(text, "<¦$1¦>")
}

func calibratedDecisionProbabilities(m *Runtime, question DecisionQuestion, scores []float32) ([]float64, error) {
	values := make([]float64, len(scores))
	for i, score := range scores {
		values[i] = float64(score)
	}
	if question.Type == DecisionQuestionTypeScore {
		bias, err := m.Calibration.ScoreBias(len(values))
		if err != nil {
			return nil, err
		}
		for i := range values {
			values[i] += bias[i]
		}
	}
	temperature, err := decisionTemperature(m, question)
	if err != nil {
		return nil, err
	}
	return decisionSoftmaxValues(values, temperature)
}

// answerEntropyQuestion is shared by D3 and Decision2, which report normalized
// entropy concentration rather than maximum answer probability.
func answerEntropyQuestion(question DecisionQuestion, options []systemOneOption, probabilities []float64) DecisionAnswer {
	answer := answerSystemOneQuestion(question, options, probabilities)
	if question.Type != DecisionQuestionTypeNoul {
		confidence := 1.0
		if len(probabilities) > 1 {
			for _, probability := range probabilities {
				if probability > 0 {
					confidence += probability * math.Log(probability) / math.Log(float64(len(probabilities)))
				}
			}
		}
		answer.Confidence = roundFour(max(0, min(1, confidence)))
	}
	return answer
}
