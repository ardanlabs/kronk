package model

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// These values are published with Jev-Style-0.8B-Decision-v3 in its
// readout_config.json. A Jev-Style protocol profile owns them; the decision
// engine remains independent of model-specific tokens and calibration.
const (
	jevStyleYesToken    llama.Token = 9542
	jevStyleNoToken     llama.Token = 874
	jevStyleSlotToken   llama.Token = 1411
	jevStyleTemperature             = 0.8800546821789332
	jevStyleMaxLen                  = 25_600
	jevStyleHeadMax                 = 2_048
)

type jevStyleProtocol struct {
	model    *Model
	renderer jevStyleRenderer
}

type jevStyleRenderer struct {
	encode  func(string) []llama.Token
	maxLen  int
	headMax int
}

type jevStyleRendered struct {
	work       decisionWork
	names      []string
	headTokens int
}

func newJevStyleProtocol(m *Model) (*jevStyleProtocol, error) {
	expected := []struct {
		text  string
		token llama.Token
	}{
		{text: " yes", token: jevStyleYesToken},
		{text: " no", token: jevStyleNoToken},
		{text: " ->", token: jevStyleSlotToken},
	}
	for _, item := range expected {
		tokens := tokenize(m.vocab, item.text, false, false)
		if len(tokens) != 1 || tokens[0] != item.token {
			return nil, fmt.Errorf("init-jev-style: tokenizer mismatch for %q: got %v, want [%d]", item.text, tokens, item.token)
		}
	}

	return &jevStyleProtocol{
		model: m,
		renderer: jevStyleRenderer{
			encode: func(text string) []llama.Token {
				return tokenize(m.vocab, text, false, false)
			},
			maxLen:  min(jevStyleMaxLen, m.decision.contextWindow),
			headMax: jevStyleHeadMax,
		},
	}, nil
}

func (p *jevStyleProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionState(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	rendered := make([]jevStyleRendered, len(req.Questions))
	work := make([]decisionWork, len(req.Questions))
	for i, question := range req.Questions {
		rendered[i], err = p.renderer.render(state, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		work[i] = rendered[i].work
	}

	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: jev-style readout: %w", err)
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		scores := make([]float64, len(readouts[i]))
		for optionIndex, logits := range readouts[i] {
			if len(logits) != 2 {
				return DecisionResponse{}, fmt.Errorf("decision: question %q readout[%d] returned %d logits, want 2", question.ID, optionIndex, len(logits))
			}
			scores[optionIndex] = float64(logits[0] - logits[1])
		}
		probabilities, err := jevStyleSoftmax(scores)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		answers[question.ID] = answerJevStyleQuestion(question, rendered[i].names, probabilities)
	}

	return DecisionResponse{
		Model:   p.model.responseModelID(),
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: decisionInputTokens(work)},
	}, nil
}

func (r jevStyleRenderer) render(state string, question DecisionQuestion) (jevStyleRendered, error) {
	instructions, err := jevStyleText(question.Instructions)
	if err != nil {
		return jevStyleRendered{}, err
	}
	names, options, err := jevStyleOptions(question)
	if err != nil {
		return jevStyleRendered{}, err
	}

	optionTokens := make([][]llama.Token, len(options))
	for i, option := range options {
		optionTokens[i] = r.encode(option)
	}

	head := r.encode(fmt.Sprintf("Question [%s]: %s\nOptions:\n", question.Type, instructions))
	dash := r.encode("- ")
	newline := r.encode("\n")
	for _, option := range optionTokens {
		head = append(head, dash...)
		head = append(head, option...)
		head = append(head, newline...)
	}
	head = append(head, r.encode("Judge each option:\n")...)

	relativeSlots := make([]int, len(optionTokens))
	for i, option := range optionTokens {
		head = append(head, option...)
		head = append(head, jevStyleSlotToken)
		relativeSlots[i] = len(head) - 1
		head = append(head, newline...)
	}
	if len(head) > r.headMax {
		return jevStyleRendered{}, fmt.Errorf("%w: question and options need %d tokens, head limit is %d", ErrDecisionBudget, len(head), r.headMax)
	}

	tokens := r.encode("State:\n")
	tokens = append(tokens, r.encode(state)...)
	tokens = append(tokens, r.encode("\n\n")...)
	prefixLen := len(tokens)
	tokens = append(tokens, head...)
	if len(tokens) > r.maxLen {
		return jevStyleRendered{}, fmt.Errorf("%w: input needs %d tokens (state %d, head %d), limit is %d", ErrDecisionBudget, len(tokens), prefixLen, len(head), r.maxLen)
	}

	readouts := make([]decisionReadout, len(relativeSlots))
	for i, slot := range relativeSlots {
		readouts[i] = decisionReadout{
			position:   prefixLen + slot,
			candidates: []llama.Token{jevStyleYesToken, jevStyleNoToken},
		}
	}
	return jevStyleRendered{
		work: decisionWork{
			tokens:    tokens,
			prefixLen: prefixLen,
			readouts:  readouts,
		},
		names:      names,
		headTokens: len(head),
	}, nil
}

func jevStyleText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	return decisionJSON(value)
}

func jevStyleOptions(question DecisionQuestion) ([]string, []string, error) {
	switch question.Type {
	case DecisionQuestionTypeChoice:
		names := make([]string, len(question.Options))
		options := make([]string, len(question.Options))
		for i, option := range question.Options {
			description, err := decisionDescription(option.Description)
			if err != nil {
				return nil, nil, err
			}
			names[i] = option.Name
			options[i] = option.Name
			if description != "" {
				options[i] += ": " + description
			}
		}
		return names, options, nil

	case DecisionQuestionTypeScore:
		names := make([]string, len(question.Levels))
		options := make([]string, len(question.Levels))
		for i, level := range question.Levels {
			description, err := decisionDescription(level)
			if err != nil {
				return nil, nil, err
			}
			names[i] = strconv.Itoa(i)
			options[i] = "level " + names[i] + ": " + description
		}
		return names, options, nil

	case DecisionQuestionTypeNoul:
		falseDescription := "no, the statement does not hold"
		trueDescription := "yes, the statement holds"
		if question.NoulCriteria != nil {
			if description, err := decisionDescription(question.NoulCriteria.False); err != nil {
				return nil, nil, err
			} else if description != "" {
				falseDescription = description
			}
			if description, err := decisionDescription(question.NoulCriteria.True); err != nil {
				return nil, nil, err
			} else if description != "" {
				trueDescription = description
			}
		}
		return []string{"false", "true"}, []string{
			"false: " + falseDescription,
			"true: " + trueDescription,
		}, nil

	default:
		return nil, nil, fmt.Errorf("unknown question type %q", question.Type)
	}
}

func jevStyleSoftmax(scores []float64) ([]float64, error) {
	maxScore := math.Inf(-1)
	for _, score := range scores {
		scaled := score / jevStyleTemperature
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return nil, fmt.Errorf("jev-style readout contains a non-finite score")
		}
		maxScore = max(maxScore, scaled)
	}

	probabilities := make([]float64, len(scores))
	var sum float64
	for i, score := range scores {
		probabilities[i] = math.Exp(score/jevStyleTemperature - maxScore)
		sum += probabilities[i]
	}
	for i := range probabilities {
		probabilities[i] /= sum
	}
	return probabilities, nil
}

func answerJevStyleQuestion(question DecisionQuestion, names []string, probabilities []float64) DecisionAnswer {
	switch question.Type {
	case DecisionQuestionTypeChoice:
		answer := DecisionAnswer{
			Type:          DecisionQuestionTypeChoice,
			Choice:        names[slicesMaxIndex(probabilities)],
			Probabilities: make(map[string]float64, len(names)),
			Confidence:    roundFour(openJEVChoiceConfidence(probabilities)),
		}
		for i, name := range names {
			answer.Probabilities[name] = roundFour(probabilities[i])
		}
		return answer

	case DecisionQuestionTypeScore:
		answer := DecisionAnswer{
			Type:          DecisionQuestionTypeScore,
			Probabilities: make(map[string]float64, len(names)),
			Legend:        make(map[string]any, len(names)),
			Confidence:    roundFour(openJEVScoreConfidence(probabilities)),
		}
		for i, name := range names {
			answer.Score += float64(i) * probabilities[i]
			answer.Probabilities[name] = roundFour(probabilities[i])
			answer.Legend[name] = question.Levels[i]
		}
		answer.Score = roundFour(answer.Score)
		return answer

	case DecisionQuestionTypeNoul:
		return DecisionAnswer{Type: DecisionQuestionTypeNoul, Noul: roundFour(probabilities[1])}

	default:
		panic("validated decision question has unknown type")
	}
}
