package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const decisionLayaMaxOptionTokens = 48

type layaProtocol struct {
	model         *Model
	template      systemOneTemplate
	marker        llama.Token
	separator     llama.Token
	markerText    string
	maxHeadTokens int
}

func newLayaProtocol(m *Model) (*layaProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-laya: %w", err)
	}

	marker := llama.VocabMASK(m.vocab)
	separator := llama.VocabSEP(m.vocab)
	if marker == llama.TokenNull || separator == llama.TokenNull {
		return nil, fmt.Errorf("init-laya: model has no mask or separator token")
	}
	if outputWidth := llama.ModelNEmbdOut(m.model); outputWidth != 3 {
		return nil, fmt.Errorf("init-laya: invalid output embedding width %d", outputWidth)
	}

	architecture := m.modelInfo.Metadata["general.architecture"]
	value := m.modelInfo.Metadata[architecture+".decision.max_head_tokens"]
	maxHeadTokens, err := strconv.Atoi(value)
	if err != nil || maxHeadTokens <= 0 {
		return nil, fmt.Errorf("init-laya: invalid max_head_tokens %q", value)
	}

	return &layaProtocol{
		model:         m,
		template:      tmpl,
		marker:        marker,
		separator:     separator,
		markerText:    tokenText(m.vocab, marker),
		maxHeadTokens: maxHeadTokens,
	}, nil
}

func (p *layaProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionSystemOneText(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}
	state = strings.ReplaceAll(state, p.markerText, " ")

	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	work := make([]decisionWork, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionSystemOneOptions(question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		for j := range options {
			options[j].name = strings.ReplaceAll(options[j].name, p.markerText, " ")
			options[j].description = strings.ReplaceAll(options[j].description, p.markerText, " ")
		}
		instructions, err := decisionSystemOneText(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		instructions = strings.ReplaceAll(instructions, p.markerText, " ")

		prompt, err := p.template.render(question, state, instructions, options)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		tokens := llama.Tokenize(p.model.vocab, prompt, false, true)
		tokens, markers, err := p.fit(tokens, len(options))
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(tokens) > p.model.decision.contextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
		}

		readouts := make([]decisionReadout, len(markers))
		for j, marker := range markers {
			readouts[j] = decisionReadout{position: marker, embeddingWidth: 3}
		}
		optionsByQuestion[i] = options
		work[i] = decisionWork{tokens: tokens, readouts: readouts}
	}

	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: laya readout: %w", err)
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		column := int(questionTypeColumn(question.Type))
		scores := make([]float32, len(readouts[i]))
		for j, embedding := range readouts[i] {
			if len(embedding) != 3 {
				return DecisionResponse{}, fmt.Errorf("decision: question %q marker[%d] returned %d values, want 3", question.ID, j, len(embedding))
			}
			scores[j] = embedding[column]
		}
		temperature, err := decisionTemperature(p.model, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		probabilities, err := decisionSoftmax(scores, temperature)
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

func (p *layaProtocol) fit(tokens []llama.Token, optionCount int) ([]llama.Token, []int, error) {
	markers := tokenPositions(tokens, p.marker)
	if len(markers) != optionCount || len(markers) == 0 || markers[0] < 2 || tokens[markers[0]-1] != p.separator || tokens[len(tokens)-1] != p.separator {
		return nil, nil, fmt.Errorf("unexpected layout of the decision prompt")
	}

	headEnd := markers[0] - 1
	optionsEnd := -1
	for i := markers[len(markers)-1]; i < len(tokens); i++ {
		if tokens[i] == p.separator {
			optionsEnd = i
			break
		}
	}
	if optionsEnd < 0 || optionsEnd+1 >= len(tokens) {
		return nil, nil, fmt.Errorf("unexpected layout of the decision prompt")
	}

	options := make([][]llama.Token, optionCount)
	for i := range optionCount {
		end := optionsEnd
		if i+1 < optionCount {
			end = markers[i+1]
		}
		options[i] = append([]llama.Token(nil), tokens[markers[i]:end]...)
	}
	trimLayaOptions(options, decisionLayaMaxOptionTokens+1)
	optionTokens := layaOptionTokenCount(options)
	if optionTokens+16 > p.maxHeadTokens {
		trimLayaOptions(options, max(4, (p.maxHeadTokens-min(p.maxHeadTokens, 16))/optionCount))
		optionTokens = layaOptionTokenCount(options)
	}
	questionMax := max(8, p.maxHeadTokens-min(p.maxHeadTokens, optionTokens))

	out := make([]llama.Token, 0, len(tokens))
	out = append(out, tokens[0])
	out = append(out, tokens[1:min(headEnd, 1+questionMax)]...)
	out = append(out, p.separator)
	markers = markers[:0]
	for _, option := range options {
		markers = append(markers, len(out))
		out = append(out, option...)
	}
	out = append(out, tokens[optionsEnd:]...)
	return out, markers, nil
}

func tokenPositions(tokens []llama.Token, target llama.Token) []int {
	var positions []int
	for i, token := range tokens {
		if token == target {
			positions = append(positions, i)
		}
	}
	return positions
}

func trimLayaOptions(options [][]llama.Token, limit int) {
	for i := range options {
		options[i] = options[i][:min(len(options[i]), limit)]
	}
}

func layaOptionTokenCount(options [][]llama.Token) int {
	var count int
	for _, option := range options {
		count += len(option)
	}
	return count
}

func questionTypeColumn(questionType DecisionQuestionType) int32 {
	switch questionType {
	case DecisionQuestionTypeChoice:
		return 0
	case DecisionQuestionTypeScore:
		return 1
	case DecisionQuestionTypeNoul:
		return 2
	default:
		panic("validated decision question has unknown type")
	}
}
