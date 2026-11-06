package decision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/jinja"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestDecisionLabelMetadataControlsOptionMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels []string
		err    error
		winner string
	}{
		{name: "model labels preserve order and leading spaces", labels: []string{" B", " A"}, winner: "second"},
		{name: "no metadata retains alphabet", winner: "first"},
		{name: "older library retains alphabet", err: llama.ErrModelNoDecisionLabel, winner: "first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encodeLabel := func(label string) []llama.Token {
				return map[string][]llama.Token{"A": {10}, "B": {11}, " A": {12}, " B": {13}}[label]
			}
			tokens, labels, err := decisionLabelTokens(func(index uint32) (string, error) {
				if tc.err != nil {
					return "", tc.err
				}
				if int(index) >= len(tc.labels) {
					return "", nil
				}
				return tc.labels[index], nil
			}, encodeLabel)
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{ModelID: "test-labels", Protocol: "pplx-decider", ContextWindow: 128}
			runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
				out := make([][][]float32, len(work))
				for i, item := range work {
					scores := make([]float32, len(item.Readouts[0].Candidates))
					for j, token := range item.Readouts[0].Candidates {
						if token == 10 || token == 12 {
							scores[j] = 3
						}
					}
					out[i] = [][]float32{scores}
				}
				return out, nil
			}
			tmpl, err := jinja.Compile(`{% for o in options %}{{ o.label }}:{{ o.key }};{% endfor %}`)
			if err != nil {
				t.Fatal(err)
			}
			p := &pplxDeciderProtocol{
				runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, encode: func(string, bool) []llama.Token { return []llama.Token{1, 2} },
				labels: labels, labelTokens: tokens}
			resp, err := validatedDeciderRequest(context.Background(), &runtime, p, DecisionRequest{State: "state", Questions: []DecisionQuestion{
				DecisionQuestionChoice("route", "Pick an option", DecisionQuestionOpt("first", nil), DecisionQuestionOpt("second", nil)),
			}})
			if err != nil {
				t.Fatal(err)
			}
			if answer := resp.Answers["route"]; answer.Choice != tc.winner || answer.Probabilities[tc.winner] != .9526 {
				t.Fatalf("Decision: got %+v, want %s with probability .9526", answer, tc.winner)
			}
		})
	}
}

func TestLFM3D1ChoiceCodes(t *testing.T) {
	runtime := Runtime{ModelID: "test-lfm3-d1", Protocol: "lfm3-d1", ContextWindow: 4096}
	runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
		out := make([][][]float32, len(work))
		for i, item := range work {
			scores := make([]float32, len(item.Readouts[0].Candidates))
			for j, token := range item.Readouts[0].Candidates {
				if token == 100 { // AA, the 27th label; not decimal 26 or a fallback.
					scores[j] = 10
				}
			}
			out[i] = [][]float32{scores}
		}
		return out, nil
	}
	tmpl, err := jinja.Compile(`{% for o in options %}{{ o.label }}:{{ o.key }};{% endfor %}`)
	if err != nil {
		t.Fatal(err)
	}
	p := &lfm3D1Protocol{breaker: strings.NewReplacer(), runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, encode: func(text string, special bool) []llama.Token {
		if special {
			return []llama.Token{1, 2}
		}
		if code, err := strconv.Atoi(text); err == nil && code >= 0 && code < 28 {
			return []llama.Token{llama.Token(200 + code)}
		}
		switch {
		case len(text) == 1 && text[0] >= 'A' && text[0] <= 'Z':
			return []llama.Token{llama.Token(10 + text[0] - 'A')}
		case text == "AA":
			return []llama.Token{100}
		case text == "AB":
			return []llama.Token{101}
		default:
			return nil
		}
	}}
	var options []DecisionOption
	for i := range 28 {
		options = append(options, DecisionQuestionOpt(fmt.Sprintf("route-%d", i), nil))
	}
	resp, err := validatedDeciderRequest(t.Context(), &runtime, p, DecisionRequest{State: nil, Questions: []DecisionQuestion{
		DecisionQuestionChoice("route", "Pick a route", options...),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Answers["route"].Choice; got != "route-26" {
		t.Errorf("choice: got %q, want route-26", got)
	}
}

func TestDeciderDecisionContracts(t *testing.T) {
	for _, protocol := range []string{"pplx-decider", "lfm2-d1", "lfm3-d1", "vllm-d3", "vllm-decision2"} {
		t.Run(protocol, func(t *testing.T) {
			runtime := Runtime{ModelID: "test-decider", Protocol: protocol, ContextWindow: 4096}
			runtime.Calibration = arch.Resolve(map[string]string{
				"general.architecture": "qwen35", "qwen35.decision.type": protocol,
				"qwen35.decision.score_bias.2.1":      "2",
				"qwen35.decision.temperature.score":   "9",
				"qwen35.decision.temperature.score.2": "2",
			}).Decision

			runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
				out := make([][][]float32, len(work))
				for i := range work {
					scores := []float32{2, 0}
					if i == 1 {
						scores = []float32{0, 0}
					}
					if i == 2 {
						scores = []float32{0, 2}
					}
					if protocol == "lfm2-d1" || protocol == "lfm3-d1" {
						if i == 0 {
							scores = []float32{2, -5, 0, -1}
						}
						if i == 2 {
							scores = []float32{2, -5, 0, -1}
						}
					}
					out[i] = [][]float32{scores}
				}
				return out, nil
			}
			var p Protocol
			var prompts []string
			encode := func(text string) []llama.Token {
				prompts = append(prompts, text)
				return []llama.Token{1, 2, 3}
			}
			if protocol == "vllm-decision2" {
				// The decision2 SystemOne fixture from the pinned Yzma revision.
				source := `{% for q in questions %}{{ "Context:\n" }}{{ state if state is string else state | tojson(separators=[',', ':']) }}{{ "\n\nTask type: " }}{{ q.type }}{{ "\nQuestion:\n" }}{{ q.instructions if q.instructions is string else q.instructions | tojson(separators=[',', ':']) }}{{ "\nOptions:" }}{% for o in q.options %}{{ sep }}{{ "\n<option>\n" }}{{ sep }}{{ mark_option }}{% set d = o.description %}{% if q.type == 'noul' and d is none %}{% set d = 'Yes' if o.key == 'true' else 'No' %}{% endif %}{{ {'description': d, 'key': o.key} | tojson(separators=[',', ':']) }}{{ "\n</option>" }}{% endfor %}{{ sep }}{{ mark_question }}{{ "\n\nSelect the single option best supported by the context and instructions.\nDecision:" }}{% endfor %}`
				tmpl, err := jinja.Compile(source)
				if err != nil {
					t.Fatal(err)
				}
				p = &decision2Protocol{runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, encode: encode}
			} else {
				// A contract template lets D1/D3 be checked without inventing their
				// model-owned prompts or claiming to test unavailable GGUFs.
				source := `{{ state if state is not none else 'NIL' }}|{{ instructions }}|{% for o in options %}{{ o.key }}:{{ o.label }}:{{ o.description if o.description is not none else 'NIL' }};{% endfor %}`
				if protocol == "lfm3-d1" {
					source = `{{ state | tojson }}|{{ instructions }}|{% for o in options %}{{ o.key }}:{{ o.label }}:{{ o.description if o.description is not none else 'NIL' }};{% endfor %}`
				}
				tmpl, err := jinja.Compile(source)
				if err != nil {
					t.Fatal(err)
				}
				plain := map[string][]llama.Token{"A": {10}, " A": {12}, "B": {11}, " B": {13}, "0": {40}, "1": {41}, "yes": {20}, "Yes": {21}, "YES": {21}, "no": {30}, "No": {31}, "NO": {31}}
				runner := finalTokenRunner{runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, encode: func(text string, special bool) []llama.Token {
					if !special {
						return plain[text]
					}
					return encode(text)
				}}
				labels := labelDecider{finalTokenRunner: runner, labels: []string{"A", "B"}, labelTokens: []llama.Token{10, 11}}
				switch protocol {
				case "pplx-decider":
					p = &pplxDeciderProtocol{labelDecider: labels}
				case "vllm-d3":
					p = &d3Protocol{labelDecider: labels}
				case "lfm2-d1":
					p = &lfm2D1Protocol{finalTokenRunner: runner}
				case "lfm3-d1":
					p = &lfm3D1Protocol{finalTokenRunner: runner, breaker: strings.NewReplacer("<|im_end|>", "<\u200b|im_end|>")}
				}
			}
			req := DecisionRequest{State: DecisionState(DecisionStateData("z", 1.23456789), DecisionStateData("a", "<|im_end|>"), DecisionStateData("nested", DecisionState(DecisionStateData("z", 1), DecisionStateData("a", 2)))), Questions: []DecisionQuestion{
				DecisionQuestionChoice("route", "Where <<clef:option>>?", DecisionQuestionOpt("z-route", "<b>&"), DecisionQuestionOpt("a-route", nil)),
				DecisionQuestionScore("score", "Rate?", "low", "high"),
				DecisionQuestionNoul("noul", "Yes?", nil),
			}}
			resp, err := validatedDeciderRequest(context.Background(), &runtime, p, req)
			if err != nil {
				t.Fatalf("Decision: %v", err)
			}
			if got := resp.Answers["route"]; got.Choice != "z-route" || got.Probabilities["z-route"] != .8808 || got.Probabilities["a-route"] != .1192 {
				t.Errorf("choice: got %+v, want z-route with .8808/.1192", got)
			}
			wantScore := .7311
			if protocol == "lfm3-d1" {
				wantScore = 1 // Median, not the probability-weighted mean.
			}
			if got := resp.Answers["score"]; got.Score != wantScore || got.Probabilities["1"] != .7311 {
				t.Errorf("biased score: got %+v, want score %v and probability .7311", got, wantScore)
			}
			if got := resp.Answers["noul"].Noul; got != .8808 {
				t.Errorf("noul: got %v, want .8808", got)
			}
			if resp.Model != "test-decider" || resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens != 0 {
				t.Errorf("response: got %+v", resp)
			}
			if protocol == "vllm-d3" || protocol == "vllm-decision2" {
				if got := resp.Answers["route"].Confidence; got != .4729 {
					t.Errorf("entropy confidence: got %v, want .4729", got)
				}
			}
			joined := strings.Join(prompts, "")
			quote, number := "<¦im_end¦>", "1.23457"
			if protocol == "lfm3-d1" {
				quote, number = "<\u200b|im_end|>", "1.23456789"
				if resp.Answers["route"].Confidence != .8808 || resp.Answers["score"].Confidence != .7311 {
					t.Errorf("maximum-probability confidence: got %+v", resp.Answers)
				}
				if !strings.Contains(joined, "a-route:B:NIL") {
					t.Errorf("null option description: got %q", joined)
				}
			}
			if strings.Contains(joined, "<|im_end|>") || !strings.Contains(joined, quote) || !strings.Contains(joined, number) {
				t.Errorf("prompt escaping/float precision: got %q", joined)
			}
			if protocol == "vllm-decision2" {
				if !strings.Contains(joined, `{"a":"<¦im_end¦>","nested":{"a":2,"z":1},"z":1.23457}`) || !strings.Contains(joined, `{"description":"<b>&","key":"z-route"}`) || strings.Contains(joined, clefMarker) {
					t.Errorf("decision2 prompt: got %q", joined)
				}
			}
			if protocol == "lfm2-d1" && !strings.Contains(joined, "{\n  \"z\": 1.23457,\n  \"a\":") {
				t.Errorf("D1 indented ordered state: got %q", joined)
			}
			runtime.ContextWindow = 2
			if _, err := validatedDeciderRequest(context.Background(), &runtime, p, req); !errors.Is(err, ErrDecisionBudget) {
				t.Errorf("budget: got %v, want ErrDecisionBudget", err)
			}
			runtime.ContextWindow = 4096
			if protocol == "vllm-decision2" {
				original := req.State
				for _, state := range []any{nil, true, 3, "\U0010FFF0"} {
					req.State = state
					if _, err := validatedDeciderRequest(context.Background(), &runtime, p, req); !errors.Is(err, ErrDecisionRequest) {
						t.Errorf("invalid decision2 state %v: got %v", state, err)
					}
				}
				req.State = original
			}
			// Missing instructions are allowed only for D3; nil state reaches
			// its template as nil instead of a JSON string.
			req.State = nil
			req.Questions = []DecisionQuestion{DecisionQuestionNoul("default", nil, nil)}
			_, err = validatedDeciderRequest(context.Background(), &runtime, p, req)
			if protocol == "vllm-d3" {
				if err != nil {
					t.Fatalf("D3 default question: %v", err)
				}
				if got := prompts[len(prompts)-1]; !strings.HasPrefix(got, "NIL||false:A:NIL;true:B:NIL;") {
					t.Errorf("D3 nil/default prompt: got %q", got)
				}
			} else if !errors.Is(err, ErrDecisionRequest) {
				t.Errorf("missing instructions: got %v, want ErrDecisionRequest", err)
			}
		})
	}
}

func TestH2OLightningDecision(t *testing.T) {
	// Emitted by llama.cpp b12125's H2O converter with a fixture system prompt.
	const source = `<|im_start|>system{{ '\n' }}Decision fixture<|im_end|>{{ '\n' }}<|im_start|>user{{ '\n' }}{% for image in images %}{{ image }}{% endfor %}{% set user %}record: {{ state }}{{ '\n' }}question: {{ instructions if instructions else 'Answer the question below.' }}{{ '\n' }}options:{% for o in options %}{{ '\n' }}{{ o.label | trim }}) {{ o.key }}: {% if type == 'noul' %}{% if o.description %}{{ o.description }}{% elif o.key == 'true' %}the statement holds{% else %}it does not{% endif %}{% elif o.description is none %}{{ o.key }}{% else %}{{ o.description }}{% endif %}{% endfor %}{% endset %}{{ user | trim }}<|im_end|>{{ '\n' }}<|im_start|>assistant{{ '\n' }}<think>{{ '\n\n' }}</think>{{ '\n\n' }}Answer:`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	serializer, err := jinja.Compile(`{{ value if value is string else value | tojson(indent=indent, separators=separators, ensure_ascii=ascii) }}`)
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{ModelID: "test-h2o", Protocol: "h2o-lightning", ContextWindow: 40000}
	runtime.Calibration = arch.Resolve(map[string]string{
		"general.architecture": "qwen35", "qwen35.decision.type": "h2o-lightning",
		"qwen35.decision.temperature.score": "2", "qwen35.decision.score_bias.2.1": "2",
	}).Decision
	var prompts []string
	p := &h2oLightningProtocol{
		runtime: &runtime, template: systemOneTemplate{tmpl: tmpl},
		encode: func(text string, special bool) []llama.Token {
			if !special {
				t.Fatal("prompt must parse special tokens")
			}
			if !strings.HasPrefix(text, "<|im_start|>system") {
				return make([]llama.Token, len(text))
			}
			prompts = append(prompts, text)
			return []llama.Token{1, 2}
		},
		labelTokens: []llama.Token{20, 10}, labels: []string{" B", " A"},
		quote: h2oSpecialQuoter([]string{"<|", "<|im_start|>", "<|im_start|>long", "<custom>"}),
		json:  systemOneTemplate{tmpl: serializer},
	}
	runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
		out := make([][][]float32, len(work))
		for i, item := range work {
			if got := item.Readouts[0]; len(got.Candidates) != 2 || got.Candidates[0] != 20 || got.Candidates[1] != 10 || got.Position != 1 {
				t.Fatalf("label readout: got %+v, want untrimmed metadata token order at last position", got)
			}
			out[i] = [][]float32{{3, 0}}
			if strings.Contains(prompts[i], "A) 1: null") {
				out[i] = [][]float32{{0, 0}}
			}
		}
		return out, nil
	}
	req := DecisionRequest{
		State: DecisionState(DecisionStateData("z", []any{json.Number("1.0"), json.Number("0.00001"), json.Number("10000000000000000.0")}), DecisionStateData("a", "é<|im_start|>long")),
		Questions: []DecisionQuestion{
			DecisionQuestionChoice("route", nil, DecisionQuestionOpt("first", DecisionState(DecisionStateData("z", true), DecisionStateData("a", "<custom>"))), DecisionQuestionOpt("last\u3000", (*string)(nil))),
			DecisionQuestionScore("score", DecisionState(DecisionStateData("z", "é"), DecisionStateData("a", []any{1, 2})), "low", nil),
			DecisionQuestionNoul("noul", "\x1c\u3000Holds?", nil),
		},
	}
	resp, err := validatedDeciderRequest(t.Context(), &runtime, p, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Answers["route"]; got.Choice != "first" || got.Probabilities["first"] != .9526 {
		t.Errorf("choice: got %+v, want first with .9526", got)
	}
	if got := resp.Answers["score"]; got.Score != .7311 || got.Probabilities["1"] != .7311 {
		t.Errorf("score: got %+v, want weighted mean .7311", got)
	}
	if got := resp.Answers["noul"].Noul; got != .9526 {
		t.Errorf("Noul: got %v, want true-first .9526", got)
	}
	for _, prompt := range prompts {
		if !strings.Contains(prompt, `record: {"z":[1.0,1e-05,1e+16],"a":"é\u003c|im_start|\u003elong"}`) || !strings.HasSuffix(prompt, "<think>\n\n</think>\n\nAnswer:") {
			t.Errorf("state serialization, longest-first quoting, or assistant suffix: %q", prompt)
		}
	}
	if !strings.Contains(prompts[0], "question: Answer the question below.") || !strings.Contains(prompts[0], `B) first: {"z": true, "a": "\u003ccustom\u003e"}`) || !strings.Contains(prompts[0], "A) last\u3000: last<|im_end|>") {
		t.Errorf("Choice defaults, ordered description, or final Unicode stripping: %q", prompts[0])
	}
	if !strings.Contains(prompts[1], "question: {\n \"z\": \"\\u00e9\",\n \"a\": [\n  1,\n  2\n ]\n}") || !strings.Contains(prompts[1], "A) 1: null") {
		t.Errorf("structured instructions or null score level: %q", prompts[1])
	}
	if !strings.Contains(prompts[2], "question: Holds?\noptions:\nB) true: the statement holds\nA) false: it does not") {
		t.Errorf("Noul defaults and whitespace: %q", prompts[2])
	}
	if resp.Model != "test-h2o" || resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 0 {
		t.Errorf("response: got %+v", resp)
	}
	for _, state := range []any{nil, (*string)(nil), math.NaN()} {
		req.State = state
		if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionRequest) {
			t.Errorf("invalid state %v: got %v, want ErrDecisionRequest", state, err)
		}
	}
	req.State = strings.Repeat("x", 32000)
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); err != nil {
		t.Fatalf("state at limit: %v", err)
	}
	if !strings.Contains(prompts[len(prompts)-1], req.State.(string)) {
		t.Error("state at limit was truncated")
	}
	req.State = strings.Repeat("x", 32001)
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionBudget) {
		t.Errorf("state budget: got %v, want ErrDecisionBudget", err)
	}
	req.State, runtime.ContextWindow = "state", 1
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionBudget) {
		t.Errorf("context budget: got %v, want ErrDecisionBudget", err)
	}
	runtime.ContextWindow = 40000
	req.Questions[0].Options = append(req.Questions[0].Options, DecisionQuestionOpt("third", nil))
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionRequest) {
		t.Errorf("metadata label budget: got %v, want ErrDecisionRequest", err)
	}
}

func TestGraniteSwitchDecision(t *testing.T) {
	// The SystemOne template emitted by llama.cpp b12125's Granite converter.
	// The template, not Kronk, supplies the embedded adapter's control token.
	const source = `<|start_of_role|>user<|end_of_role|>State:
{{ state if state is string else state | tojson }}<|end_of_text|>
<|start_of_role|>user<|end_of_role|><|decision|> DECISION
Question:
{{ instructions if instructions is string else instructions | tojson }}{% if type == 'score' %} Rate along the ordered levels below (lowest first).{% endif %}{{ '\n\nOptions:\n' }}{% for o in options %}{% if type != 'noul' %}{{ o.label }}: {% if o.description %}{{ o.description if o.description is string else o.description | tojson }}{% endif %}{% elif o.key == 'true' %}A: {% if o.description %}{{ o.description if o.description is string else o.description | tojson }}{% else %}The statement is true.{% endif %}{% else %}B: {% if o.description %}{{ o.description if o.description is string else o.description | tojson }}{% else %}The statement is false.{% endif %}{% endif %}{{ '\n' }}{% endfor %}{{ '\nReply with exactly one option label.<|end_of_text|>\n<|start_of_role|>assistant<|end_of_role|>' }}`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{ModelID: "test-granite", Protocol: "granite-switch", ContextWindow: 4096}
	runtime.Calibration = arch.Resolve(map[string]string{
		"general.architecture": "graniteswitch", "graniteswitch.decision.type": "granite-switch",
		"graniteswitch.decision.score_bias.2.1":      "2",
		"graniteswitch.decision.temperature.score":   "9",
		"graniteswitch.decision.temperature.score.2": "2",
	}).Decision
	var prompts []string
	encode := func(text string, special bool) []llama.Token {
		if !special {
			return map[string][]llama.Token{"A": {10}, "B": {11}, "C": {12, 13}, "AA": {40}}[text]
		}
		prompts = append(prompts, text)
		last := llama.Token(1)
		switch {
		case strings.Contains(text, "Rate along the ordered levels"):
			last = 2
		case strings.Contains(text, "The statement is true."):
			last = 3
		}
		return []llama.Token{0, last}
	}
	tokens, labels := decisionAlphabetLabels(func(text string) []llama.Token { return encode(text, false) })
	p := &graniteSwitchProtocol{
		runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, encode: encode,
		labelTokens: tokens, labels: labels,
	}
	runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
		out := make([][][]float32, len(work))
		for i, item := range work {
			scores := make([]float32, len(item.Readouts[0].Candidates))
			for j, token := range item.Readouts[0].Candidates {
				switch item.Tokens[len(item.Tokens)-1] {
				case 1:
					if token == 40 {
						scores[j] = 2
					}
				case 3:
					if token == 10 {
						scores[j] = 2
					}
				}
			}
			out[i] = [][]float32{scores}
		}
		return out, nil
	}
	req := DecisionRequest{
		State: DecisionState(DecisionStateData("z", 1.23456789), DecisionStateData("a", []any{
			json.Number("1.0"), true, nil, DecisionState(DecisionStateData("z", 2), DecisionStateData("a", 1)),
		})),
		Questions: []DecisionQuestion{
			DecisionQuestionChoice("route", DecisionState(DecisionStateData("task", "Choose")),
				DecisionQuestionOpt("first", nil), DecisionQuestionOpt("second", []any{}),
				DecisionQuestionOpt("third", DecisionState(DecisionStateData("z", false), DecisionStateData("a", 1.23456789)))),
			DecisionQuestionScore("score", "Rate", "low", "high"),
			DecisionQuestionNoul("noul", "Holds?", nil),
		},
	}
	resp, err := validatedDeciderRequest(t.Context(), &runtime, p, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Answers["route"]; got.Choice != "third" || got.Probabilities["third"] != .787 || got.Probabilities["first"] != .1065 {
		t.Errorf("choice: got %+v, want third with .787/.1065", got)
	}
	if got := resp.Answers["score"]; got.Score != .7311 || got.Probabilities["1"] != .7311 {
		t.Errorf("score: got %+v, want .7311", got)
	}
	if got := resp.Answers["noul"].Noul; got != .8808 {
		t.Errorf("noul: got %v, want .8808", got)
	}
	if resp.Model != "test-granite" || resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 0 {
		t.Errorf("response: got %+v", resp)
	}
	for _, prompt := range prompts {
		if !strings.Contains(prompt, `State:
{"z": 1.23456789, "a": [1.0, true, null, {"z": 2, "a": 1}]}`) || !strings.Contains(prompt, "<|decision|> DECISION") {
			t.Errorf("structured state or adapter control token lost: %q", prompt)
		}
	}
	if !strings.Contains(prompts[0], "Question:\n{\"task\": \"Choose\"}") || !strings.Contains(prompts[0], "A: \nB: \nAA: {\"z\": false, \"a\": 1.23456789}\n") {
		t.Errorf("structured instructions/options or label selection lost: %q", prompts[0])
	}
	if !strings.Contains(prompts[2], "A: The statement is true.\nB: The statement is false.\n") {
		t.Errorf("Noul defaults/order: %q", prompts[2])
	}
	for _, state := range []any{nil, (*string)(nil), math.NaN()} {
		req.State = state
		if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionRequest) {
			t.Errorf("invalid state %v: got %v, want ErrDecisionRequest", state, err)
		}
	}
	req.State = "<|decision|>"
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompts[len(prompts)-1], "State:\n<|decision|><|end_of_text|>") {
		t.Errorf("string state should reach the model-owned template unchanged: %q", prompts[len(prompts)-1])
	}
	runtime.ContextWindow = 1
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionBudget) {
		t.Errorf("budget: got %v, want ErrDecisionBudget", err)
	}
	runtime.ContextWindow = 4096
	req.Questions[0].Options = append(req.Questions[0].Options, DecisionQuestionOpt("fourth", nil))
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); !errors.Is(err, ErrDecisionRequest) {
		t.Errorf("label limit: got %v, want ErrDecisionRequest", err)
	}
}

// validatedDeciderRequest retains the public facade's question validation while
// protocol tests supply inference results without native engine or scheduler state.
func validatedDeciderRequest(ctx context.Context, runtime *Runtime, p Protocol, req DecisionRequest) (DecisionResponse, error) {
	if err := ValidateRequest(req, runtime.Protocol); err != nil {
		return DecisionResponse{}, err
	}
	return p.Decide(ctx, req)
}
