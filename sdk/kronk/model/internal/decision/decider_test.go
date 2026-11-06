package decision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
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

func TestLFM2D1OmniDecision(t *testing.T) {
	// The SystemOne template emitted by llama.cpp b12125's D1Omni converter.
	const source = `{% set ns = namespace(criteria=false) %}{% for o in options %}{% if o.description is not none %}{% set ns.criteria = true %}{% endif %}{% endfor %}{% for image in images %}{{ image }}{% endfor %}{{ sep }}<|startoftext|><|reserved_7|>{{ sep }}{{ mark_state }}{% if state is string %}{{ state }}{% elif state is not none %}{{ state | tojson }}{% elif audio %}{}{% endif %}{{ sep }}{{ mark_question }}<|reserved_8|>{{ instructions if instructions is string else instructions | tojson }}{% for o in options %}{{ sep }}<|reserved_9|><|mask|>{{ sep }}{{ mark_option }} {% if type == 'choice' and audio %}option_{% if loop.index0 < 10 %}00{% elif loop.index0 < 100 %}0{% endif %}{{ loop.index0 }}: {% if o.description is not none and o.description != '' %}{{ o.description if o.description is string else o.description | tojson }}{% else %}{{ o.key }}{% endif %}{% elif type == 'choice' %}{{ o.key }}{% if o.description is not none and o.description != '' %}: {{ o.description if o.description is string else o.description | tojson }}{% endif %}{% elif type == 'score' %}level {{ o.key }}: {{ o.description if o.description is string else o.description | tojson }}{% elif audio %}{{ o.key }}: {{ 'yes' if o.key == 'true' else 'no' }}{% else %}{{ o.key }}: {% if o.description is not none and o.description != '' %}{{ o.description if o.description is string else o.description | tojson }}{% elif images and not ns.criteria %}{{ 'yes' if o.key == 'true' else 'no' }}{% elif o.key == 'true' %}yes, the statement holds{% else %}no, the statement does not hold{% endif %}{% endif %}{{ sep }}<|reserved_10|>{% endfor %}{{ sep }}<|reserved_11|>`
	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{ModelID: "test-lfm2-omni", Protocol: "lfm2-d1-omni", ContextWindow: 20000}
	runtime.Calibration = arch.Resolve(map[string]string{
		"general.architecture": "lfm2", "lfm2.decision.type": "lfm2-d1-omni",
		"lfm2.decision.temperature.score.3_5": "2", "lfm2.decision.score_bias.3.2": "2",
	}).Decision
	var pieces []string
	p := &lfm2D1OmniProtocol{
		runtime: &runtime, template: systemOneTemplate{tmpl: tmpl}, marker: 9,
		encode: func(text string) []llama.Token {
			pieces = append(pieces, text)
			if strings.Contains(text, "<<d1omni:") {
				t.Fatalf("piece marker reached tokenizer: %q", text)
			}
			// Synthetic vocabulary: each ordinary byte is a token; model control
			// spellings are single tokens. This is not a native tokenizer test.
			var tokens []llama.Token
			for len(text) > 0 {
				found := false
				for spelling, id := range map[string]llama.Token{
					"<|startoftext|>": 1, "<|reserved_7|>": 2, "<|reserved_8|>": 3,
					"<|reserved_9|>": 4, "<|mask|>": 9, "<|reserved_10|>": 5, "<|reserved_11|>": 6,
				} {
					if rest, ok := strings.CutPrefix(text, spelling); ok {
						tokens = append(tokens, id)
						text, found = rest, true
						break
					}
				}
				if !found {
					tokens = append(tokens, llama.Token(text[0])+1000)
					text = text[1:]
				}
			}
			return tokens
		},
	}
	runtime.Run = func(_ context.Context, work []Work) ([][][]float32, error) {
		out := make([][][]float32, len(work))
		for i, item := range work {
			if item.PrefixLen != 0 || len(item.PrefixRows) != 0 || item.Position != 0 || len(item.DecisionOrder) != 0 {
				t.Fatalf("expected independent whole-prompt embedding plan: %+v", item)
			}
			for j, readout := range item.Readouts {
				if readout.EmbeddingWidth != 3 || len(readout.Candidates) != 0 || item.Tokens[readout.Position] != 9 {
					t.Fatalf("readout: got %+v, want width 3 at mask token", readout)
				}
				out[i] = append(out[i], [][]float32{{0, 0, 0}, {2, 1, 2}, {-1, 0, -9}}[j])
			}
		}
		return out, nil
	}
	req := DecisionRequest{
		State: json.RawMessage(`{"z":"\u003c|mask|>","a":[{"<<d1omni:sep>>":1.23456789}]}`),
		Questions: []DecisionQuestion{
			DecisionQuestionChoice("route", "Pick <|mask|> <<d1omni:option>>", DecisionQuestionOpt("<|mask|>", nil), DecisionQuestionOpt("second", DecisionState(DecisionStateData("z", false), DecisionStateData("a", json.Number("1.0"))))),
			DecisionQuestionScore("score", "Rate", "low", "middle", "high"),
			DecisionQuestionNoul("noul", "Holds?", nil),
		},
	}
	resp, err := validatedDeciderRequest(t.Context(), &runtime, p, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Answers["route"]; got.Choice != "second" || got.Probabilities["second"] != .8808 || got.Probabilities["<|mask|>"] != .1192 || got.Confidence != .7616 {
		t.Errorf("choice: got %+v, want original keys, .8808/.1192 and confidence .7616", got)
	}
	if got := resp.Answers["score"]; got.Score != 1.3202 || got.Probabilities["2"] != .5065 || got.Probabilities["0"] != .1863 {
		t.Errorf("score: got %+v, want weighted mean 1.3202 and .1863/.3072/.5065", got)
	}
	if got := resp.Answers["noul"].Noul; got != .8808 {
		t.Errorf("Noul: got %v, want false-first true probability .8808", got)
	}
	wantPieces := []string{
		"<|startoftext|><|reserved_7|>", `{"z": "<¦mask¦>", "a": [{"<<d1omni sep>>": 1.23456789}]}`,
		"<|reserved_8|>Pick <¦mask¦> <<d1omni option>>", "<|reserved_9|><|mask|>", " <¦mask¦>", "<|reserved_10|>",
		"<|reserved_9|><|mask|>", ` second: {"z": false, "a": 1.0}`, "<|reserved_10|>", "<|reserved_11|>",
	}
	if !slices.Equal(pieces[:len(wantPieces)], wantPieces) {
		t.Errorf("piecewise prompt: got %q, want %q", pieces[:len(wantPieces)], wantPieces)
	}
	if joined := strings.Join(pieces, ""); !strings.Contains(joined, " false: no, the statement does not hold") || !strings.Contains(joined, " true: yes, the statement holds") {
		t.Errorf("Noul defaults: %q", joined)
	}
	if resp.Model != "test-lfm2-omni" || resp.Usage.InputTokens <= 0 || resp.Usage.OutputTokens != 0 {
		t.Errorf("response: %+v", resp)
	}
	for _, tt := range []struct {
		name                                     string
		count, question, description, state, ctx int
		wantErr                                  bool
	}{
		{"two-option question at limit", 2, 95, 0, 0, 20000, false},
		{"two-option question over limit", 2, 96, 0, 0, 20000, true},
		{"three-option question at limit", 3, 103, 0, 0, 20000, false},
		{"three-option question over limit", 3, 104, 0, 0, 20000, true},
		{"two-option description at limit", 2, 1, 41, 0, 20000, false},
		{"two-option description over limit", 2, 1, 42, 0, 20000, true},
		{"three-option description at limit", 3, 1, 27, 0, 20000, false},
		{"three-option description over limit", 3, 1, 28, 0, 20000, true},
		{"prompt at model limit", 2, 1, 0, 16369, 20000, false},
		{"prompt over model limit", 2, 1, 0, 16370, 20000, true},
		{"prompt at configured limit", 2, 1, 0, 0, 15, false},
		{"prompt over configured limit", 2, 1, 0, 0, 14, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime.ContextWindow = tt.ctx
			options := []DecisionOption{DecisionQuestionOpt("a", nil), DecisionQuestionOpt("b", nil)}
			if tt.count == 3 {
				options = append(options, DecisionQuestionOpt("c", nil))
			}
			if tt.description > 0 {
				options[0].Description = strings.Repeat("x", tt.description)
			}
			_, err := validatedDeciderRequest(t.Context(), &runtime, p, DecisionRequest{
				State: strings.Repeat("s", tt.state), Questions: []DecisionQuestion{DecisionQuestionChoice("q", strings.Repeat("Q", tt.question), options...)},
			})
			if (err != nil) != tt.wantErr || tt.wantErr && !errors.Is(err, ErrDecisionBudget) {
				t.Errorf("budget: got %v, want ErrDecisionBudget=%t", err, tt.wantErr)
			}
		})
	}
	runtime.ContextWindow = 20000
	req.State = nil
	pieces = nil
	if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); err != nil || pieces[1] != "" {
		t.Errorf("null state: got %v and pieces %q, want empty state piece", err, pieces)
	}
	for _, replacement := range []string{"{{ mark_state }}", "<|mask|>"} {
		broken, err := jinja.Compile(strings.ReplaceAll(source, replacement, ""))
		if err != nil {
			t.Fatal(err)
		}
		p.template = systemOneTemplate{tmpl: broken}
		if _, err := validatedDeciderRequest(t.Context(), &runtime, p, req); err == nil {
			t.Errorf("missing %s should reject malformed model template", replacement)
		}
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
