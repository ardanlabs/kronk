package model

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// deciderProtocol implements the final-token protocols. D1 differs in candidate
// selection; D3 differs in execution and confidence, not in readout location.
type deciderProtocol struct {
	model       *Model
	template    systemOneTemplate
	encode      func(string, bool) []llama.Token
	labelTokens []llama.Token
	labels      []string
}

func newDeciderProtocol(m *Model) (decisionProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-decider: %w", err)
	}
	p := deciderProtocol{model: m, template: tmpl, encode: func(text string, special bool) []llama.Token {
		return llama.Tokenize(m.vocab, text, false, special)
	}}
	if m.modelInfo.decisionProtocol != DecisionProtocolLfm2D1 {
		p.labelTokens, p.labels = decisionLabelTokens(m.vocab)
		if len(p.labels) < 9 {
			return nil, fmt.Errorf("init-decider: tokenizer provides %d single-token labels, need at least 9", len(p.labels))
		}
	}
	return &p, nil
}

func (p *deciderProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	protocol := p.model.modelInfo.decisionProtocol
	var state any
	if req.State != nil || protocol == DecisionProtocolPplxDecider {
		text, err := decisionDeciderText(req.State, protocol == DecisionProtocolLfm2D1, false)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
		}
		state = escapeDecisionSpecial(text)
	}

	work := make([]decisionWork, len(req.Questions))
	optionsByQuestion := make([][]systemOneOption, len(work))
	groupsByQuestion := make([][]int, len(work))
	for i, question := range req.Questions {
		options, err := decisionDeciderOptions(question, protocol == DecisionProtocolLfm2D1)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > decisionMaxReadouts {
			return DecisionResponse{}, fmt.Errorf("%w: question %q has too many options", ErrDecisionRequest, question.ID)
		}
		var labels []string
		var groups [][]llama.Token
		if protocol == DecisionProtocolLfm2D1 {
			labels, groups, err = decisionD1Labels(question.Type, options, func(text string) []llama.Token { return p.encode(text, false) })
		} else {
			if len(options) > len(p.labels) {
				return DecisionResponse{}, fmt.Errorf("%w: question %q needs %d labels, model provides %d", ErrDecisionRequest, question.ID, len(options), len(p.labels))
			}
			labels = p.labels[:len(options)]
			for _, token := range p.labelTokens[:len(options)] {
				groups = append(groups, []llama.Token{token})
			}
		}
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		instructions := ""
		if question.Instructions != nil {
			instructions, err = decisionDeciderText(question.Instructions, false, false)
			if err != nil {
				return DecisionResponse{}, err
			}
		}
		values := decisionDeciderQuestion(question, instructions, options, labels)
		values["state"], values["images"] = state, []any{}
		prompt, err := p.template.renderValues(values)
		if err != nil {
			return DecisionResponse{}, err
		}
		tokens := p.encode(prompt, true)
		if len(tokens) > p.model.decision.contextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
		}
		var candidates []llama.Token
		for _, group := range groups {
			candidates = append(candidates, group...)
			groupsByQuestion[i] = append(groupsByQuestion[i], len(group))
		}
		work[i] = decisionWork{tokens: tokens, readouts: []decisionReadout{{position: len(tokens) - 1, candidates: candidates}}}
		optionsByQuestion[i] = options
	}
	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: decider readout: %w", err)
	}
	answers := make(map[string]DecisionAnswer, len(work))
	for i, question := range req.Questions {
		var scores []float32
		offset := 0
		for _, n := range groupsByQuestion[i] {
			scores = append(scores, slices.Max(readouts[i][0][offset:offset+n]))
			offset += n
		}
		answer, err := answerDeciderQuestion(p.model, question, optionsByQuestion[i], scores)
		if err != nil {
			return DecisionResponse{}, err
		}
		answers[question.ID] = answer
	}
	return DecisionResponse{Model: p.model.responseModelID(), Answers: answers, Usage: DecisionUsage{InputTokens: decisionInputTokens(work)}}, nil
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

func decisionD1Labels(typ DecisionQuestionType, options []systemOneOption, encode func(string) []llama.Token) ([]string, [][]llama.Token, error) {
	single := func(forms ...string) []llama.Token {
		var tokens []llama.Token
		for _, form := range forms {
			if ids := encode(form); len(ids) == 1 && !slices.Contains(tokens, ids[0]) {
				tokens = append(tokens, ids[0])
			}
		}
		return tokens
	}
	var labels []string
	var groups [][]llama.Token
	if typ != DecisionQuestionTypeChoice {
		for _, option := range options {
			group := single(option.name)
			if typ == DecisionQuestionTypeNoul {
				group = single("no", "No", "NO")
				if option.name == "true" {
					group = single("yes", "Yes", "YES")
				}
			}
			if len(group) == 0 {
				return nil, nil, fmt.Errorf("%w: label %q is not one token", ErrDecisionRequest, option.name)
			}
			labels = append(labels, option.name)
			groups = append(groups, group)
		}
		return labels, groups, nil
	}
	letters := true
	for _, option := range options {
		key := option.name
		letters = letters && len(key) == 1 && (key[0] >= 'a' && key[0] <= 'z' || key[0] >= 'A' && key[0] <= 'Z')
	}
	var used []llama.Token
	take := func(code string) bool {
		ids := encode(code)
		if len(ids) != 1 || slices.Contains(used, ids[0]) {
			return false
		}
		used = append(used, ids[0])
		labels = append(labels, code)
		groups = append(groups, single(code, " "+code))
		return true
	}
	var pool []string
	for c := 'A'; c <= 'Z'; c++ {
		pool = append(pool, string(c))
	}
	for i := range 100 {
		pool = append(pool, fmt.Sprintf("%02d", i))
	}
	for c := 'a'; c <= 'z'; c++ {
		pool = append(pool, string(c))
	}
	for i := range 200 {
		pool = append(pool, "#"+strconv.Itoa(i))
	}
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			pool = append(pool, string([]rune{a, b}))
		}
	}
	for i, option := range options {
		code := fmt.Sprintf("%02d", i)
		if letters {
			code = option.name
		} else if len(options) <= 26 {
			code = string(rune('A' + i))
		}
		if take(code) {
			continue
		}
		found := slices.ContainsFunc(pool, take)
		if !found {
			return nil, nil, fmt.Errorf("%w: no single-token label left for %d options", ErrDecisionRequest, len(options))
		}
	}
	return labels, groups, nil
}

func answerDeciderQuestion(m *Model, question DecisionQuestion, options []systemOneOption, scores []float32) (DecisionAnswer, error) {
	values := make([]float64, len(scores))
	for i, score := range scores {
		values[i] = float64(score)
	}
	if question.Type == DecisionQuestionTypeScore {
		bias, err := m.modelInfo.profile.Decision.ScoreBias(len(values))
		if err != nil {
			return DecisionAnswer{}, err
		}
		for i := range values {
			values[i] += bias[i]
		}
	}
	temperature, err := decisionTemperature(m, question)
	if err != nil {
		return DecisionAnswer{}, err
	}
	probabilities, err := decisionSoftmaxValues(values, temperature)
	if err != nil {
		return DecisionAnswer{}, err
	}
	answer := answerSystemOneQuestion(question, options, probabilities)
	protocol := m.modelInfo.decisionProtocol
	if question.Type != DecisionQuestionTypeNoul && (protocol == DecisionProtocolVllmD3 || protocol == DecisionProtocolVllmDecision2) {
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
	return answer, nil
}
