package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ardanlabs/jinja"
	"github.com/hybridgroup/yzma/pkg/llama"
)

type systemOneTemplate struct {
	tmpl *jinja.Template
}

type systemOneOption struct {
	name        string
	description string
	label       string
}

func newSystemOneTemplate(m *Model) (systemOneTemplate, error) {
	source := llama.ModelChatTemplate(m.model, "systemone")
	if source == "" {
		return systemOneTemplate{}, fmt.Errorf("decision model has no %q template", "systemone")
	}

	tmpl, err := jinja.Compile(source)
	if err != nil {
		return systemOneTemplate{}, fmt.Errorf("compile systemone template: %w", err)
	}

	return systemOneTemplate{tmpl: tmpl}, nil
}

func (t systemOneTemplate) render(question DecisionQuestion, state, instructions string, options []systemOneOption) (string, error) {
	values := make([]D, len(options))
	for i, option := range options {
		values[i] = D{
			"key":         option.name,
			"description": option.description,
			"label":       option.label,
		}
	}

	rendered, err := t.tmpl.Render(D{
		"id":           question.ID,
		"type":         string(question.Type),
		"instructions": instructions,
		"state":        state,
		"options":      values,
		"images":       []string{},
	})
	if err != nil {
		return "", fmt.Errorf("render systemone template: %w", err)
	}

	return rendered, nil
}

func (t systemOneTemplate) renderValues(values D) (string, error) {
	rendered, err := t.tmpl.Render(values)
	if err != nil {
		return "", fmt.Errorf("render systemone template: %w", err)
	}
	return rendered, nil
}

func decisionUsesEmbeddings(protocol DecisionProtocol) bool {
	return protocol == DecisionProtocolLaya || protocol == DecisionProtocolKev || protocol == DecisionProtocolClef
}

func decisionLabelTokens(vocab llama.Vocab) ([]llama.Token, []string) {
	var tokens []llama.Token
	var labels []string
	appendLabel := func(label string) {
		encoded := llama.Tokenize(vocab, label, false, false)
		if len(encoded) == 1 {
			tokens = append(tokens, encoded[0])
			labels = append(labels, label)
		}
	}

	for first := 'A'; first <= 'Z' && len(tokens) < decisionMaxReadouts; first++ {
		appendLabel(string(first))
	}
	for first := 'A'; first <= 'Z' && len(tokens) < decisionMaxReadouts; first++ {
		for second := 'A'; second <= 'Z' && len(tokens) < decisionMaxReadouts; second++ {
			appendLabel(string([]rune{first, second}))
		}
	}
	return tokens, labels
}

func decisionTemplateValue(value any) (any, error) {
	data, err := marshalDecisionJSON(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func decisionSystemOneText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	return decisionJSON(value)
}

func decisionSystemOneOptions(question DecisionQuestion) ([]systemOneOption, error) {
	switch question.Type {
	case DecisionQuestionTypeChoice:
		options := make([]systemOneOption, len(question.Options))
		for i, option := range question.Options {
			description, err := decisionDescription(option.Description)
			if err != nil {
				return nil, err
			}
			options[i] = systemOneOption{name: option.Name, description: description}
		}
		return options, nil

	case DecisionQuestionTypeScore:
		options := make([]systemOneOption, len(question.Levels))
		for i, level := range question.Levels {
			description, err := decisionDescription(level)
			if err != nil {
				return nil, err
			}
			options[i] = systemOneOption{name: strconv.Itoa(i), description: description}
		}
		return options, nil

	case DecisionQuestionTypeNoul:
		falseDescription := ""
		trueDescription := ""
		if question.NoulCriteria != nil {
			var err error
			falseDescription, err = decisionDescription(question.NoulCriteria.False)
			if err != nil {
				return nil, err
			}
			trueDescription, err = decisionDescription(question.NoulCriteria.True)
			if err != nil {
				return nil, err
			}
		}
		return []systemOneOption{
			{name: "false", description: falseDescription},
			{name: "true", description: trueDescription},
		}, nil

	default:
		return nil, fmt.Errorf("unknown question type %q", question.Type)
	}
}

func decisionTemperature(m *Model, question DecisionQuestion) (float64, error) {
	n := decisionOptionCount(question)
	bucket := ""
	if m.modelInfo.decisionProtocol == DecisionProtocolLev {
		switch {
		case n <= 8:
			bucket = "small"
		case n <= 26:
			bucket = "mid"
		default:
			bucket = "large"
		}
	} else {
		switch {
		case n <= 2:
			bucket = "2"
		case n <= 5:
			bucket = "3_5"
		case n <= 10:
			bucket = "6_10"
		default:
			bucket = "11"
		}
	}

	for _, suffix := range []string{string(question.Type) + "." + bucket, string(question.Type)} {
		temperature, exists, err := m.modelInfo.profile.Decision.Temperature(suffix)
		if !exists {
			continue
		}
		if err != nil {
			return 0, err
		}
		return temperature, nil
	}

	return 1, nil
}

func decisionOptionCount(question DecisionQuestion) int {
	if question.Type == DecisionQuestionTypeScore {
		return len(question.Levels)
	}
	if question.Type == DecisionQuestionTypeNoul {
		return 2
	}
	return len(question.Options)
}

func decisionSoftmax(scores []float32, temperature float64) ([]float64, error) {
	if len(scores) == 0 {
		return nil, fmt.Errorf("decision readout has no scores")
	}

	maxScore := math.Inf(-1)
	for _, score := range scores {
		scaled := float64(score) / temperature
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return nil, fmt.Errorf("decision readout contains a non-finite score")
		}
		maxScore = max(maxScore, scaled)
	}

	probabilities := make([]float64, len(scores))
	var sum float64
	for i, score := range scores {
		probabilities[i] = math.Exp(float64(score)/temperature - maxScore)
		sum += probabilities[i]
	}
	for i := range probabilities {
		probabilities[i] /= sum
	}
	return probabilities, nil
}

func answerSystemOneQuestion(question DecisionQuestion, options []systemOneOption, probabilities []float64) DecisionAnswer {
	if question.Type == DecisionQuestionTypeNoul {
		for i, option := range options {
			if option.name == "true" {
				return DecisionAnswer{Type: DecisionQuestionTypeNoul, Noul: roundFour(probabilities[i])}
			}
		}
		panic("validated noul question has no true option")
	}

	names := make([]string, len(options))
	for i, option := range options {
		names[i] = option.name
	}
	return answerJevStyleQuestion(question, names, probabilities)
}

func decisionSortedText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}

	data, err := marshalDecisionJSON(value)
	if err != nil {
		return "", err
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return "", err
	}
	return decisionJSON(decoded)
}
