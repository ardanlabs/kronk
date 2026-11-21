package decision

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

type lfm3D1Protocol struct {
	finalTokenRunner
	breaker *strings.Replacer
}

func newLFM3D1Protocol(m *Runtime) (*lfm3D1Protocol, error) {
	runner, err := newFinalTokenRunner(m)
	if err != nil {
		return nil, err
	}
	var spellings []string
	for id := range llama.VocabNTokens(m.Vocab) {
		token := llama.Token(id)
		if llama.VocabGetAttr(m.Vocab, token)&llama.TokenAttrControl != 0 {
			if text := llama.VocabGetText(m.Vocab, token); len(text) >= 2 {
				spellings = append(spellings, text)
			}
		}
	}
	slices.SortStableFunc(spellings, func(a, b string) int { return len(b) - len(a) })
	pairs := make([]string, 0, 2*len(spellings))
	for _, text := range spellings {
		_, n := utf8.DecodeRuneInString(text)
		pairs = append(pairs, text, text[:n]+"\u200b"+text[n:])
	}
	p := lfm3D1Protocol{finalTokenRunner: runner, breaker: strings.NewReplacer(pairs...)}
	return &p, nil
}

func (p *lfm3D1Protocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	questions, err := p.questions(req)
	if err != nil {
		return DecisionResponse{}, err
	}
	return p.runQuestions(ctx, req, questions)
}

// runQuestions follows D1's pack_set layout: shared state is group zero,
// and each question can attend only to the state and its own nonzero group.
// Groups isolate full attention, not KV sequences or unrelated requests.
func (p *lfm3D1Protocol) runQuestions(ctx context.Context, req DecisionRequest, questions []finalTokenQuestion) (DecisionResponse, error) {
	pack := p.runtime.DecisionGroups && len(questions) > 1
	for _, question := range req.Questions {
		if _, spoken := question.Instructions.(DecisionAudioInstructions); spoken {
			pack = false
		}
	}
	if !pack {
		return p.finalTokenRunner.run(ctx, req, questions, answerLFM3D1Question)
	}
	var work []Work
	for i, question := range questions {
		if err := ctx.Err(); err != nil {
			return DecisionResponse{}, err
		}
		values := maps.Clone(question.values)
		values["state"] = nil
		lone, err := p.template.renderValues(values)
		if err != nil {
			return DecisionResponse{}, err
		}
		head := "<|startoftext|><|im_start|>user\n"
		if len(question.files) > 0 {
			head += strings.Repeat(mtmd.DefaultMarker(), len(question.files))
		}
		suffix, ok := strings.CutPrefix(lone, head)
		if !ok {
			return DecisionResponse{}, fmt.Errorf("decision: unexpected LFM3 d1 prompt layout")
		}
		text := suffix
		if question.values["state"] != nil {
			text = "QUESTION:\n" + text
		}
		tokens := p.encode(text, true)
		if len(work) > 0 {
			set := &work[len(work)-1]
			if len(set.Readouts) < 16 && set.rowCount()+len(tokens) <= p.runtime.ContextWindow {
				group := int32(len(set.Readouts) + 1)
				set.Tokens = append(set.Tokens, tokens...)
				for range tokens {
					set.DecisionGroups = append(set.DecisionGroups, group)
				}
				var candidates []llama.Token
				for _, group := range question.groups {
					candidates = append(candidates, group...)
				}
				set.Readouts = append(set.Readouts, Readout{Position: set.Position + len(set.Tokens) - 1, Candidates: candidates})
				continue
			}
		}
		// A new set starts with the ordinary complete prompt. This also checks
		// standalone budgets and prepares shared media only once per set.
		item, err := p.prepareQuestion(ctx, question)
		if err != nil {
			return DecisionResponse{}, fmt.Errorf("decision: question %q: %w", req.Questions[i].ID, err)
		}
		first := item.rowCount() - len(p.encode(suffix, true))
		if first <= 0 || first >= item.rowCount() {
			return DecisionResponse{}, fmt.Errorf("decision: unexpected LFM3 d1 question boundary")
		}
		item.DecisionGroups = make([]int32, len(item.Tokens))
		for j := first; j < item.rowCount(); j++ {
			if j < len(item.PrefixRows) {
				item.PrefixRows[j].DecisionGroup = 1
				continue
			}
			item.DecisionGroups[j-len(item.PrefixRows)] = 1
		}
		work = append(work, item)
	}
	return p.runWork(ctx, req, questions, work, answerLFM3D1Question)
}

func (p *lfm3D1Protocol) questions(req DecisionRequest) ([]finalTokenQuestion, error) {
	questions := make([]finalTokenQuestion, len(req.Questions))
	for i, question := range req.Questions {
		options, err := decisionDeciderOptions(question, true)
		if err != nil {
			return nil, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if len(options) > MaxReadouts {
			return nil, fmt.Errorf("%w: question %q has too many options", ErrDecisionRequest, question.ID)
		}
		labels, groups, err := decisionD1Labels(question.Type, options, lfm3D1ChoiceCode, func(text string) []llama.Token { return p.encode(text, false) })
		if err != nil {
			return nil, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		if question.Instructions != nil {
			// Retain numeric input validation without using its lossy text
			// representation in the model-owned JSON template.
			if _, err := decisionDeciderText(question.Instructions, false, false); err != nil {
				return nil, err
			}
		}
		values, err := p.values(req.State, question, labels)
		if err != nil {
			return nil, fmt.Errorf("decision: question %q: %w", question.ID, err)
		}
		questions[i] = finalTokenQuestion{values: values, options: options, groups: groups}
	}
	return questions, nil
}

func lfm3D1ChoiceCode(index int) string {
	if index < 26 {
		return string(rune('A' + index))
	}
	return string([]rune{'A' + rune((index-26)/26), 'A' + rune((index-26)%26)})
}

// values preserves JSON values for the model-owned template. In particular,
// null descriptions and structured state must not become pre-serialized text.
func (p *lfm3D1Protocol) values(state any, question DecisionQuestion, labels []string) (D, error) {
	options, err := decisionRawSystemOneOptions(question, true)
	if err != nil {
		return nil, err
	}
	for i := range options {
		options[i].label = labels[i]
	}
	values := decisionTemplateQuestion(question, question.Instructions, options)
	values["state"], values["images"] = state, []any{}
	value, err := decisionTemplateValue(values)
	if err != nil {
		return nil, err
	}
	if instructions, ok := value.(map[string]any)["instructions"].(map[string]any); ok && instructions["type"] == "input_audio" {
		return nil, fmt.Errorf("%w: lfm3-d1 supports text instructions, not spoken instructions", ErrDecisionRequest)
	}

	// Repeating the longest-first replacement also breaks overlapping control
	// spellings. Insert after the first UTF-8 character, as the native d1 shim does.
	breakText := func(text string) string {
		for {
			next := p.breaker.Replace(text)
			if next == text {
				return text
			}
			text = next
		}
	}
	var escape func(any) any
	escape = func(value any) any {
		switch value := value.(type) {
		case string:
			return breakText(value)
		case []any:
			for i := range value {
				value[i] = escape(value[i])
			}
			return value
		case map[string]any:
			out := make(map[string]any, len(value))
			for key, item := range value {
				out[breakText(key)] = escape(item)
			}
			return out
		default:
			return value
		}
	}
	return D(escape(value).(map[string]any)), nil
}

func answerLFM3D1Question(question DecisionQuestion, options []systemOneOption, probabilities []float64) DecisionAnswer {
	answer := answerSystemOneQuestion(question, options, probabilities)
	if question.Type == DecisionQuestionTypeNoul {
		return answer
	}
	answer.Confidence = roundFour(slices.Max(probabilities))
	if question.Type == DecisionQuestionTypeScore {
		var mass float64
		for i, probability := range probabilities {
			mass += probability
			answer.Score = float64(i)
			if mass >= .5 {
				break
			}
		}
	}
	return answer
}
