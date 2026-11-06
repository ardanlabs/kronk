package decision

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ardanlabs/jinja"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	lfm2OmniSeparator    = "<<d1omni:sep>>"
	lfm2OmniStateMark    = "<<d1omni:state>>"
	lfm2OmniQuestionMark = "<<d1omni:question>>"
	lfm2OmniOptionMark   = "<<d1omni:option>>"
	lfm2OmniMaxTokens    = 16384
)

type lfm2D1OmniProtocol struct {
	runtime  *Runtime
	template systemOneTemplate
	marker   llama.Token
	encode   func(string) []llama.Token
}

func newLFM2D1OmniProtocol(m *Runtime) (Protocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-lfm2-d1-omni: %w", err)
	}
	marker := llama.VocabMASK(m.Vocab)
	if marker == llama.TokenNull {
		return nil, fmt.Errorf("init-lfm2-d1-omni: model has no mask token")
	}
	if width := llama.ModelNEmbdOut(m.Model); width != 3 {
		return nil, fmt.Errorf("init-lfm2-d1-omni: invalid output embedding width %d", width)
	}
	p := lfm2D1OmniProtocol{
		runtime: m, template: tmpl, marker: marker,
		encode: func(text string) []llama.Token { return llama.Tokenize(m.Vocab, text, false, true) },
	}
	return &p, nil
}

func (p *lfm2D1OmniProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	work := make([]Work, len(req.Questions))
	for i, question := range req.Questions {
		options := question.Options
		switch question.Type {
		case DecisionQuestionTypeScore:
			options = make([]DecisionOption, len(question.Levels))
			for j, level := range question.Levels {
				options[j] = DecisionQuestionOpt(strconv.Itoa(j), level)
			}
		case DecisionQuestionTypeNoul:
			var falseValue, trueValue any
			if question.NoulCriteria != nil {
				falseValue, trueValue = question.NoulCriteria.False, question.NoulCriteria.True
			}
			options = []DecisionOption{DecisionQuestionOpt("false", falseValue), DecisionQuestionOpt("true", trueValue)}
		}
		raw := make([]D, len(options))
		optionsByQuestion[i] = make([]systemOneOption, len(options))
		for j, option := range options {
			raw[j] = D{"key": option.Name, "description": option.Description}
			optionsByQuestion[i][j] = systemOneOption{name: option.Name}
		}
		native, err := decisionJinjaValue(D{
			"id": question.ID, "type": question.Type.String(), "state": req.State,
			"instructions": question.Instructions, "options": raw,
		})
		if err != nil {
			return DecisionResponse{}, err
		}
		native = escapeLFM2OmniValue(native)
		values := D{
			"images": []any{}, "audio": false, "sep": lfm2OmniSeparator,
			"mark_state": lfm2OmniStateMark, "mark_question": lfm2OmniQuestionMark, "mark_option": lfm2OmniOptionMark,
		}
		for _, key := range native.AsDict().OrderedKeys() {
			values[key], _ = native.AsDict().Get(key)
		}
		prompt, err := p.template.renderValues(values)
		if err != nil {
			return DecisionResponse{}, err
		}
		work[i], err = p.prepare(prompt, len(options))
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
	}
	readouts, err := p.runtime.Run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: lfm2-d1-omni readout: %w", err)
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
		probabilities, err := calibratedDecisionProbabilities(p.runtime, question, scores)
		if err != nil {
			return DecisionResponse{}, err
		}
		answers[question.ID] = answerSystemOneQuestion(question, optionsByQuestion[i], probabilities)
	}
	return DecisionResponse{Model: p.runtime.ModelID, Answers: answers, Usage: DecisionUsage{InputTokens: decisionInputTokens(work)}}, nil
}

// Escape decoded strings, including object keys, before adding template markers.
// Ordered objects and scalar types stay intact for model-owned JSON rendering.
func escapeLFM2OmniValue(value jinja.Value) jinja.Value {
	clean := func(text string) string {
		return strings.ReplaceAll(escapeDecisionSpecial(text), "<<d1omni:", "<<d1omni ")
	}
	switch {
	case value.IsString():
		return jinja.NewString(clean(value.AsString()))
	case value.IsList():
		for i, item := range value.AsList().Items {
			value.AsList().Items[i] = escapeLFM2OmniValue(item)
		}
	case value.IsDict():
		out := jinja.NewDict()
		for _, key := range value.AsDict().OrderedKeys() {
			item, _ := value.AsDict().Get(key)
			out.AsDict().Set(clean(key), escapeLFM2OmniValue(item))
		}
		return out
	}
	return value
}

func (p *lfm2D1OmniProtocol) prepare(prompt string, optionCount int) (Work, error) {
	pieces := strings.Split(prompt, lfm2OmniSeparator)
	if pieces[0] != "" {
		return Work{}, fmt.Errorf("decision: unexpected media prefix in text-only lfm2-d1-omni prompt")
	}
	budget := max(96, min(24*optionCount+32, lfm2OmniMaxTokens/2))
	optionMax := max(2, (budget-3*optionCount)/optionCount)
	var tokens []llama.Token
	var states, questions, options int
	for _, piece := range pieces[1:] {
		limit := lfm2OmniMaxTokens
		switch {
		case strings.HasPrefix(piece, lfm2OmniStateMark):
			piece = strings.TrimPrefix(piece, lfm2OmniStateMark)
			states++
		case strings.HasPrefix(piece, lfm2OmniQuestionMark):
			piece = strings.TrimPrefix(piece, lfm2OmniQuestionMark)
			limit = max(16, budget)
			questions++
		case strings.HasPrefix(piece, lfm2OmniOptionMark):
			piece = strings.TrimPrefix(piece, lfm2OmniOptionMark)
			limit = optionMax
			options++
		}
		// Piece boundaries are tokenizer boundaries, not just template markers.
		// Unlike the native shim, Kronk rejects a piece rather than truncating it.
		part := p.encode(piece)
		if len(part) > limit {
			return Work{}, fmt.Errorf("%w: lfm2-d1-omni piece needs %d tokens, limit is %d", ErrDecisionBudget, len(part), limit)
		}
		tokens = append(tokens, part...)
	}
	if states != 1 || questions != 1 || options != optionCount {
		return Work{}, fmt.Errorf("decision: unexpected layout of the lfm2-d1-omni prompt")
	}
	limit := min(lfm2OmniMaxTokens, p.runtime.ContextWindow)
	if len(tokens) > limit {
		return Work{}, fmt.Errorf("%w: lfm2-d1-omni input needs %d tokens, limit is %d", ErrDecisionBudget, len(tokens), limit)
	}
	markers := tokenPositions(tokens, p.marker)
	if len(markers) != optionCount {
		return Work{}, fmt.Errorf("decision: lfm2-d1-omni prompt has %d mask tokens, want %d", len(markers), optionCount)
	}
	readouts := make([]Readout, len(markers))
	for i, marker := range markers {
		readouts[i] = Readout{Position: marker, EmbeddingWidth: 3}
	}
	return Work{Tokens: tokens, Readouts: readouts}, nil
}
