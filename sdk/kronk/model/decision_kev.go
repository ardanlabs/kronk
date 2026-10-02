package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

var kevSpecialToken = regexp.MustCompile(`<\|([A-Za-z0-9_]+)\|>`)

type kevProtocol struct {
	model       *Model
	template    systemOneTemplate
	marker      llama.Token
	outputWidth int
}

func newKevProtocol(m *Model) (*kevProtocol, error) {
	tmpl, err := newSystemOneTemplate(m)
	if err != nil {
		return nil, fmt.Errorf("init-kev: %w", err)
	}

	tokens := llama.Tokenize(m.vocab, "<|box_end|>", false, true)
	if len(tokens) != 1 {
		return nil, fmt.Errorf("init-kev: model has no single-token <|box_end|> marker")
	}
	outputWidth := int(llama.ModelNEmbdOut(m.model))
	if outputWidth <= 0 || outputWidth%2 != 0 {
		return nil, fmt.Errorf("init-kev: invalid output embedding width %d", outputWidth)
	}

	return &kevProtocol{
		model:       m,
		template:    tmpl,
		marker:      tokens[0],
		outputWidth: outputWidth,
	}, nil
}

func (p *kevProtocol) decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	state, err := decisionKevText(req.State)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: serialize state: %w", err)
	}

	optionsByQuestion := make([][]systemOneOption, len(req.Questions))
	work := make([]decisionWork, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionSystemOneOptions(question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		for j := range options {
			options[j].name, err = decisionKevText(options[j].name)
			if err != nil {
				return DecisionResponse{}, fmt.Errorf("decision: question %q option[%d]: %w", question.ID, j, err)
			}
			options[j].description, err = decisionKevText(options[j].description)
			if err != nil {
				return DecisionResponse{}, fmt.Errorf("decision: question %q option[%d]: %w", question.ID, j, err)
			}
		}
		instructions, err := decisionKevText(question.Instructions)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		prompt, err := p.template.render(question, state, instructions, options)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		tokens := llama.Tokenize(p.model.vocab, prompt, false, true)
		if len(tokens) == 0 {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: tokenize prompt: no tokens", question.ID)
		}
		if len(tokens) > p.model.decision.contextWindow {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w: input needs %d tokens, limit is %d", question.ID, ErrDecisionBudget, len(tokens), p.model.decision.contextWindow)
		}
		markers := tokenPositions(tokens, p.marker)
		if len(markers) != len(options) || markers[len(markers)-1] >= len(tokens)-1 {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: unexpected layout of the decision prompt", question.ID)
		}

		readouts := make([]decisionReadout, 0, len(markers)+1)
		for _, marker := range markers {
			readouts = append(readouts, decisionReadout{position: marker, embeddingWidth: p.outputWidth})
		}
		readouts = append(readouts, decisionReadout{position: len(tokens) - 1, embeddingWidth: p.outputWidth})
		optionsByQuestion[i] = options
		work[i] = decisionWork{tokens: tokens, readouts: readouts}
	}

	readouts, err := p.model.decision.run(ctx, work)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("decision: kev readout: %w", err)
	}

	answers := make(map[string]DecisionAnswer, len(req.Questions))
	for i, question := range req.Questions {
		headWidth := p.outputWidth / 2
		pointer := readouts[i][len(readouts[i])-1]
		if len(pointer) != p.outputWidth {
			return DecisionResponse{}, fmt.Errorf("decision: question %q pointer returned %d values, want %d", question.ID, len(pointer), p.outputWidth)
		}
		scores := make([]float32, len(readouts[i])-1)
		for j, embedding := range readouts[i][:len(readouts[i])-1] {
			if len(embedding) != p.outputWidth {
				return DecisionResponse{}, fmt.Errorf("decision: question %q marker[%d] returned %d values, want %d", question.ID, j, len(embedding), p.outputWidth)
			}
			var dot float64
			for k := range headWidth {
				dot += float64(pointer[k] * embedding[headWidth+k])
			}
			scores[j] = float32(dot / math.Sqrt(float64(headWidth)))
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

func decisionKevText(value any) (string, error) {
	text, err := decisionKevRender(value, 0)
	if err != nil {
		return "", err
	}
	return kevSpecialToken.ReplaceAllString(text, `<¦$1¦>`), nil
}

func decisionKevRender(value any, indent int) (string, error) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	if boolean, ok := value.(bool); ok {
		if boolean {
			return "True", nil
		}
		return "False", nil
	}
	if ordered, ok := value.(DecisionStateValue); ok {
		var buf strings.Builder
		for i, field := range ordered {
			if i > 0 {
				buf.WriteByte('\n')
			}
			nested := decisionKevNested(field.Value)
			nextIndent := 0
			separator := ": "
			if nested {
				nextIndent = indent + 1
				separator = ":\n"
			}
			text, err := decisionKevRender(field.Value, nextIndent)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&buf, "%s%s%s%s", strings.Repeat("  ", indent), field.Name, separator, text)
		}
		return buf.String(), nil
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
	return decisionKevRenderJSON(decoded, indent), nil
}

func decisionKevNested(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Array || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Struct
}

func decisionKevRenderJSON(value any, indent int) string {
	pad := strings.Repeat("  ", indent)
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		if value {
			return "True"
		}
		return "False"
	case []any:
		parts := make([]string, len(value))
		for i, item := range value {
			parts[i] = pad + "- " + strings.TrimLeft(decisionKevRenderJSON(item, indent+1), " \t\n\r")
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, key := range keys {
			item := value[key]
			_, object := item.(map[string]any)
			_, array := item.([]any)
			separator := ": "
			nextIndent := 0
			if object || array {
				separator = ":\n"
				nextIndent = indent + 1
			}
			parts[i] = pad + key + separator + decisionKevRenderJSON(item, nextIndent)
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(value)
	}
}
