package decision

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/ardanlabs/jinja"
	"github.com/hybridgroup/yzma/pkg/llama"
)

type h2oLightningProtocol struct {
	finalTokenRunner
	labelTokens []llama.Token
	labels      []string
	quote       *strings.Replacer
	json        systemOneTemplate
}

func newH2OLightningProtocol(m *Runtime) (Protocol, error) {
	runner, err := newFinalTokenRunner(m)
	if err != nil {
		return nil, err
	}
	tokens, labels, err := decisionMetadataLabels(func(index uint32) (string, error) {
		return llama.ModelDecisionLabel(m.Model, index)
	}, func(text string) []llama.Token { return runner.encode(text, false) })
	if err != nil {
		return nil, fmt.Errorf("init-h2o-lightning: %w", err)
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("init-h2o-lightning: model has no decision labels")
	}
	var spellings []string
	for id := range llama.VocabNTokens(m.Vocab) {
		token := llama.Token(id)
		if llama.VocabGetAttr(m.Vocab, token)&(llama.TokenAttrControl|llama.TokenAttrUserDef) != 0 {
			if text := llama.VocabGetText(m.Vocab, token); text != "" {
				spellings = append(spellings, text)
			}
		}
	}
	serializer, err := jinja.Compile(`{{ value if value is string else value | tojson(indent=indent, separators=separators, ensure_ascii=ascii) }}`)
	if err != nil {
		return nil, fmt.Errorf("init-h2o-lightning: compile JSON formatter: %w", err)
	}
	p := h2oLightningProtocol{
		finalTokenRunner: runner, labelTokens: tokens, labels: labels,
		quote: h2oSpecialQuoter(spellings), json: systemOneTemplate{tmpl: serializer},
	}
	return &p, nil
}

func h2oSpecialQuoter(spellings []string) *strings.Replacer {
	slices.SortStableFunc(spellings, func(a, b string) int { return len(b) - len(a) })
	angles := strings.NewReplacer("<", `\u003c`, ">", `\u003e`)
	pairs := make([]string, 0, 2*len(spellings))
	for _, text := range spellings {
		pairs = append(pairs, text, angles.Replace(text))
	}
	return strings.NewReplacer(pairs...)
}

// text follows the three Python JSON layouts used by the H2O shim. Formatting
// stays in the existing Jinja engine so float representation is not duplicated.
func (p *h2oLightningProtocol) text(value any, layout string) (string, error) {
	native, ok := value.(jinja.Value)
	if !ok {
		var err error
		native, err = decisionJinjaValue(value)
		if err != nil {
			return "", err
		}
	}
	var indent any
	separators := []string{", ", ": "}
	switch layout {
	case "state":
		if native.IsNone() {
			return "", fmt.Errorf("%w: h2o-lightning requires state", ErrDecisionRequest)
		}
		separators = []string{",", ":"}
	case "instructions":
		indent = 1
		separators = []string{",", ": "}
		if native.IsNone() {
			native = jinja.NewString("")
		}
	}
	return p.json.renderValues(D{"value": native, "indent": indent, "separators": separators, "ascii": layout == "instructions"})
}

func h2oWhitespace(ch rune) bool {
	return unicode.IsSpace(ch) || ch >= 0x1c && ch <= 0x1f
}

func (p *h2oLightningProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := p.text(req.State, "state")
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}
	// Kronk rejects oversized inputs instead of silently dropping state. Count
	// before quoting, matching the model's 32,000-token state budget.
	if len(state) > 32000 && len(p.encode(state, true)) > 32000 {
		return DecisionResponse{}, fmt.Errorf("%w: h2o-lightning state exceeds 32000 tokens", ErrDecisionBudget)
	}
	state = p.quote.Replace(state)
	questions := make([]finalTokenQuestion, len(req.Questions))
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
			options = []DecisionOption{DecisionQuestionOpt("true", trueValue), DecisionQuestionOpt("false", falseValue)}
		}
		if len(options) > len(p.labels) {
			return DecisionResponse{}, fmt.Errorf("%w: question %q needs %d labels, model provides %d", ErrDecisionRequest, question.ID, len(options), len(p.labels))
		}
		instructions, err := p.text(question.Instructions, "instructions")
		if err != nil {
			return DecisionResponse{}, err
		}
		values := make([]D, len(options))
		prepared := finalTokenQuestion{options: make([]systemOneOption, len(options))}
		for j, option := range options {
			native, err := decisionJinjaValue(option.Description)
			if err != nil {
				return DecisionResponse{}, err
			}
			var description any
			if !native.IsNone() || question.Type == DecisionQuestionTypeScore {
				text, err := p.text(native, "description")
				if err != nil {
					return DecisionResponse{}, err
				}
				description = p.quote.Replace(text)
			}
			key := p.quote.Replace(option.Name)
			if j == len(options)-1 {
				text, isText := description.(string)
				if question.Type == DecisionQuestionTypeChoice && description == nil {
					text, isText = key, true
				}
				if stripped := strings.TrimRightFunc(text, h2oWhitespace); isText && stripped != text {
					description = stripped
					if stripped == "" {
						description = " "
					}
				}
			}
			values[j] = D{"key": key, "description": description, "label": p.labels[j]}
			prepared.options[j] = systemOneOption{name: option.Name}
			prepared.groups = append(prepared.groups, []llama.Token{p.labelTokens[j]})
		}
		prepared.values = D{
			"type": question.Type.String(), "state": state, "images": []any{}, "options": values,
			"instructions": p.quote.Replace(strings.TrimLeftFunc(instructions, h2oWhitespace)),
		}
		questions[i] = prepared
	}
	return p.run(ctx, req, questions, answerSystemOneQuestion)
}
