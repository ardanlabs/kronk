package model

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	openJEVTemperature = 0.85
	openJEVNoulScale   = 1.829074
	openJEVMaxOnePass  = 52
)

var openJEVLetters = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

type openJEVOption struct {
	name        string
	description string
}

type openJEVProtocol struct {
	model        *Model
	letterTokens []llama.Token
}

func newOpenJEVProtocol(m *Model) (*openJEVProtocol, error) {
	for _, prefix := range []string{"", " "} {
		ids := make([]llama.Token, len(openJEVLetters))
		seen := make(map[llama.Token]bool, len(ids))
		valid := true
		for i, letter := range openJEVLetters {
			tokens := llama.Tokenize(m.vocab, prefix+string(letter), false, false)
			if len(tokens) != 1 || seen[tokens[0]] {
				valid = false
				break
			}
			ids[i] = tokens[0]
			seen[tokens[0]] = true
		}
		if valid {
			return &openJEVProtocol{model: m, letterTokens: ids}, nil
		}
	}
	return nil, fmt.Errorf("init-openjev: neither bare nor space-prefixed A-Z/a-z are unique single tokens")
}

type openJEVQuestionPlan struct {
	question      DecisionQuestion
	options       []openJEVOption
	instructions  string
	chunks        [][]openJEVOption
	probabilities [][]float64
}

type openJEVPass struct {
	plan  int
	chunk int
}

func (p *openJEVProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionState(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	plans := make([]openJEVQuestionPlan, len(req.Questions))
	var passes []openJEVPass
	var work []decisionWork
	for i, question := range req.Questions {
		plan, err := prepareOpenJEVQuestion(question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		plans[i] = plan
		for chunkIndex, chunk := range plan.chunks {
			item, err := p.work(ctx, state, plan.instructions, chunk)
			if err != nil {
				return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
			}
			work = append(work, item)
			passes = append(passes, openJEVPass{plan: i, chunk: chunkIndex})
		}
	}

	result, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: openjev readout: %w", err)
	}
	inputTokens := decisionInputTokens(work)
	for i, pass := range passes {
		probabilities, err := openJEVSoftmax(result[i][0])
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", plans[pass.plan].question.ID, err)
		}
		plans[pass.plan].probabilities[pass.chunk] = probabilities
	}

	var finalWork []decisionWork
	var finalPlans []int
	for i := range plans {
		if len(plans[i].chunks) == 1 {
			continue
		}
		winners := make([]openJEVOption, len(plans[i].chunks))
		for chunkIndex, chunk := range plans[i].chunks {
			winners[chunkIndex] = chunk[slicesMaxIndex(plans[i].probabilities[chunkIndex])]
		}
		item, err := p.work(ctx, state, plans[i].instructions, winners)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q final pass: %w", plans[i].question.ID, err)
		}
		finalWork = append(finalWork, item)
		finalPlans = append(finalPlans, i)
	}

	if len(finalWork) > 0 {
		finalResult, err := p.model.decision.run(ctx, finalWork)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: openjev final readout: %w", err)
		}
		inputTokens += decisionInputTokens(finalWork)
		for i, planIndex := range finalPlans {
			final, err := openJEVSoftmax(finalResult[i][0])
			if err != nil {
				return DecisionResponse{}, fmt.Errorf("decision: question %q final pass: %w", plans[planIndex].question.ID, err)
			}
			winners := make([]int, len(plans[planIndex].chunks))
			for chunkIndex := range winners {
				winners[chunkIndex] = slicesMaxIndex(plans[planIndex].probabilities[chunkIndex])
			}
			composed := openJEVComposeChunks(plans[planIndex].probabilities, winners, final)
			plans[planIndex].probabilities = [][]float64{composed}
		}
	}

	answers := make(map[string]DecisionAnswer, len(plans))
	for _, plan := range plans {
		answers[plan.question.ID] = answerOpenJEVQuestion(plan)
	}
	return DecisionResponse{
		Model:   p.model.responseModelID(),
		Answers: answers,
		Usage:   DecisionUsage{InputTokens: inputTokens},
	}, nil
}

func (p *openJEVProtocol) work(ctx context.Context, state string, instructions string, options []openJEVOption) (decisionWork, error) {
	prompt := openJEVPrompt(state, instructions, options)
	rendered, err := p.model.applyJinjaTemplate(ctx, D{
		"messages": []D{{"role": RoleUser, "content": prompt}},
		"chat_template_kwargs": D{
			"enable_thinking": false,
		},
	})
	if err != nil {
		return decisionWork{}, fmt.Errorf("render prompt: %w", err)
	}

	tokens := llama.Tokenize(p.model.vocab, rendered, p.model.addBOSToken, true)
	if len(tokens) == 0 {
		return decisionWork{}, fmt.Errorf("tokenize prompt: no tokens")
	}
	if len(tokens) > p.model.decision.contextWindow {
		return decisionWork{}, fmt.Errorf("%w: input needs %d tokens, limit is %d", ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
	}
	candidates := append([]llama.Token(nil), p.letterTokens[:len(options)]...)
	return decisionWork{
		tokens:    tokens,
		prefixLen: len(tokens) - 1,
		readouts: []decisionReadout{{
			position:   len(tokens) - 1,
			candidates: candidates,
		}},
	}, nil
}

func prepareOpenJEVQuestion(question DecisionQuestion) (openJEVQuestionPlan, error) {
	instructions, err := openJEVInstructions(question.Instructions)
	if err != nil {
		return openJEVQuestionPlan{}, err
	}

	var options []openJEVOption
	switch question.Type {
	case DecisionQuestionTypeChoice:
		options = make([]openJEVOption, len(question.Options))
		for i, option := range question.Options {
			description, err := decisionDescription(option.Description)
			if err != nil {
				return openJEVQuestionPlan{}, err
			}
			options[i] = openJEVOption{name: option.Name, description: description}
		}

	case DecisionQuestionTypeScore:
		options, err = openJEVScoreOptions(question.Levels)
		if err != nil {
			return openJEVQuestionPlan{}, err
		}
		instructions += " Rate along the ordered levels below (lowest first)."

	case DecisionQuestionTypeNoul:
		yes := "The statement is true."
		no := "The statement is false."
		if question.NoulCriteria != nil {
			if description, err := decisionDescription(question.NoulCriteria.True); err != nil {
				return openJEVQuestionPlan{}, err
			} else if description != "" {
				yes = description
			}
			if description, err := decisionDescription(question.NoulCriteria.False); err != nil {
				return openJEVQuestionPlan{}, err
			} else if description != "" {
				no = description
			}
		}
		options = []openJEVOption{{name: "yes", description: yes}, {name: "no", description: no}}
	}

	chunks := openJEVChunks(options)
	return openJEVQuestionPlan{
		question:      question,
		options:       options,
		instructions:  instructions,
		chunks:        chunks,
		probabilities: make([][]float64, len(chunks)),
	}, nil
}

func answerOpenJEVQuestion(plan openJEVQuestionPlan) DecisionAnswer {
	probabilities := plan.probabilities[0]
	switch plan.question.Type {
	case DecisionQuestionTypeChoice:
		answer := DecisionAnswer{
			Type:          DecisionQuestionTypeChoice,
			Choice:        plan.options[slicesMaxIndex(probabilities)].name,
			Probabilities: make(map[string]float64, len(probabilities)),
			Confidence:    roundFour(openJEVChoiceConfidence(probabilities)),
		}
		for i, probability := range probabilities {
			answer.Probabilities[plan.options[i].name] = roundFour(probability)
		}
		return answer

	case DecisionQuestionTypeScore:
		answer := DecisionAnswer{
			Type:          DecisionQuestionTypeScore,
			Probabilities: make(map[string]float64, len(probabilities)),
			Legend:        make(map[string]any, len(probabilities)),
			Confidence:    roundFour(openJEVScoreConfidence(probabilities)),
		}
		for i, probability := range probabilities {
			name := strconv.Itoa(i)
			answer.Score += float64(i) * probability
			answer.Probabilities[name] = roundFour(probability)
			answer.Legend[name] = plan.question.Levels[i]
		}
		answer.Score = roundFour(answer.Score)
		return answer

	case DecisionQuestionTypeNoul:
		return DecisionAnswer{Type: DecisionQuestionTypeNoul, Noul: roundFour(openJEVNoul(probabilities[0]))}

	default:
		panic("validated decision question has unknown type")
	}
}

func decisionInputTokens(work []decisionWork) int {
	var total int
	for _, item := range work {
		total += len(item.tokens)
	}
	return total
}

func decisionState(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	return decisionJSON(value)
}

func openJEVInstructions(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	return decisionPythonRepr(value)
}

func decisionDescription(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	return decisionJSON(value)
}

func openJEVPrompt(state string, instructions string, options []openJEVOption) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "State:\n%s\n\nQuestion: %s\nOptions:\n", state, instructions)
	for i, option := range options {
		fmt.Fprintf(&buf, "[%c] %s: %s\n", openJEVLetters[i], option.name, option.description)
	}
	buf.WriteString("\nAnswer with the letter of the best option only.")
	return buf.String()
}

func openJEVSoftmax(logits []float32) ([]float64, error) {
	if len(logits) == 0 {
		return nil, fmt.Errorf("openjev readout has no candidate logits")
	}

	maxLogit := math.Inf(-1)
	for _, logit := range logits {
		value := float64(logit) / openJEVTemperature
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("openjev readout contains a non-finite candidate logit")
		}
		maxLogit = max(maxLogit, value)
	}

	probabilities := make([]float64, len(logits))
	var sum float64
	for i, logit := range logits {
		probabilities[i] = math.Exp(float64(logit)/openJEVTemperature - maxLogit)
		sum += probabilities[i]
	}
	for i := range probabilities {
		probabilities[i] /= sum
	}
	return probabilities, nil
}

func openJEVChoiceConfidence(probabilities []float64) float64 {
	if len(probabilities) == 1 {
		return 1
	}
	uniform := 1 / float64(len(probabilities))
	return max(0, (slicesMax(probabilities)-uniform)/(1-uniform))
}

func openJEVScoreConfidence(probabilities []float64) float64 {
	if len(probabilities) == 1 {
		return 1
	}
	mode := slicesMaxIndex(probabilities)
	var distance float64
	for i, probability := range probabilities {
		distance += probability * math.Abs(float64(i-mode))
	}
	center := float64(len(probabilities)-1) / 2
	var uniformMeanAbsoluteDeviation float64
	for i := range probabilities {
		uniformMeanAbsoluteDeviation += math.Abs(float64(i) - center)
	}
	uniformMeanAbsoluteDeviation /= float64(len(probabilities))
	return max(0, 1-distance/uniformMeanAbsoluteDeviation)
}

func openJEVNoul(probability float64) float64 {
	probability = min(max(probability, 1e-4), 1-1e-4)
	logit := math.Log(probability/(1-probability)) / openJEVNoulScale
	return 1 / (1 + math.Exp(-logit))
}

func openJEVChunks(options []openJEVOption) [][]openJEVOption {
	if len(options) <= openJEVMaxOnePass {
		return [][]openJEVOption{options}
	}
	nChunks := (len(options) + openJEVMaxOnePass - 1) / openJEVMaxOnePass
	chunkSize := (len(options) + nChunks - 1) / nChunks
	chunks := make([][]openJEVOption, 0, nChunks)
	for start := 0; start < len(options); start += chunkSize {
		chunks = append(chunks, options[start:min(start+chunkSize, len(options))])
	}
	return chunks
}

func openJEVComposeChunks(parts [][]float64, winners []int, final []float64) []float64 {
	var totalOptions int
	for _, part := range parts {
		totalOptions += len(part)
	}

	probabilities := make([]float64, 0, totalOptions)
	var sum float64
	for chunk, part := range parts {
		for _, probability := range part {
			composed := final[chunk] * probability / part[winners[chunk]]
			probabilities = append(probabilities, composed)
			sum += composed
		}
	}
	for i := range probabilities {
		probabilities[i] /= sum
	}
	return probabilities
}

func openJEVScoreOptions(levels []any) ([]openJEVOption, error) {
	options := make([]openJEVOption, len(levels))
	for i, level := range levels {
		description, err := decisionDescription(level)
		if err != nil {
			return nil, err
		}
		options[i] = openJEVOption{name: strconv.Itoa(i), description: description}
	}
	return options, nil
}

func slicesMax(values []float64) float64 {
	return values[slicesMaxIndex(values)]
}

func slicesMaxIndex(values []float64) int {
	winner := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[winner] {
			winner = i
		}
	}
	return winner
}

func roundFour(value float64) float64 {
	return math.Round(value*1e4) / 1e4
}
