package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ardanlabs/jinja"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
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
		{name: "older library retains alphabet", err: ErrModelDecisionLabelUnavailable, winner: "first"},
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
			m := Model{modelInfo: ModelInfo{ID: "test-labels", decisionProtocol: DecisionProtocolPplxDecider}}
			engine := decisionEngine{contextWindow: 128, maxTokens: 128, maxSequences: 1, maxOutputs: 128, nVocab: 32}
			engine.scheduler = newDecisionScheduler(&engine, 1)
			engine.scheduler.evaluate = func(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
				out := make([][][]float32, len(entries))
				for i, entry := range entries {
					scores := make([]float32, len(entry.work.readouts[0].candidates))
					for j, token := range entry.work.readouts[0].candidates {
						if token == 10 || token == 12 {
							scores[j] = 3
						}
					}
					out[i] = [][]float32{scores}
				}
				return out, false, nil
			}
			engine.scheduler.start()
			t.Cleanup(engine.scheduler.stop)
			m.decision = &engine
			tmpl, err := jinja.Compile(`{% for o in options %}{{ o.label }}:{{ o.key }};{% endfor %}`)
			if err != nil {
				t.Fatal(err)
			}
			m.protocol = &deciderProtocol{model: &m, template: systemOneTemplate{tmpl: tmpl}, labels: labels, labelTokens: tokens, encode: func(string, bool) []llama.Token { return []llama.Token{1, 2} }}
			resp, err := m.Decision(context.Background(), DecisionRequest{State: "state", Questions: []DecisionQuestion{
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

func TestDeciderDecisionContracts(t *testing.T) {
	for _, protocol := range []DecisionProtocol{DecisionProtocolPplxDecider, DecisionProtocolLfm2D1, DecisionProtocolVllmD3, DecisionProtocolVllmDecision2} {
		t.Run(protocol.String(), func(t *testing.T) {
			m := Model{modelInfo: ModelInfo{ID: "test-decider", decisionProtocol: protocol}}
			m.modelInfo.profile = modelprofile.Resolve(map[string]string{
				"general.architecture": "qwen35", "qwen35.decision.type": protocol.String(),
				"qwen35.decision.score_bias.2.1":      "2",
				"qwen35.decision.temperature.score":   "9",
				"qwen35.decision.temperature.score.2": "2",
			})
			engine := decisionEngine{contextWindow: 4096, maxTokens: 4096, maxSequences: 1, maxOutputs: 4096, nVocab: 128}
			engine.scheduler = newDecisionScheduler(&engine, 1)
			engine.scheduler.evaluate = func(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
				out := make([][][]float32, len(entries))
				for i, entry := range entries {
					scores := []float32{2, 0}
					if entry.workOffset == 1 {
						scores = []float32{0, 0}
					}
					if entry.workOffset == 2 {
						scores = []float32{0, 2}
					}
					if protocol == DecisionProtocolLfm2D1 {
						if entry.workOffset == 0 {
							scores = []float32{2, -5, 0, -1}
						}
						if entry.workOffset == 2 {
							scores = []float32{2, -5, 0, -1}
						}
					}
					out[i] = [][]float32{scores}
				}
				return out, false, nil
			}
			engine.scheduler.start()
			t.Cleanup(engine.scheduler.stop)
			m.decision = &engine
			var prompts []string
			encode := func(text string) []llama.Token {
				prompts = append(prompts, text)
				return []llama.Token{1, 2, 3}
			}
			if protocol == DecisionProtocolVllmDecision2 {
				// The decision2 SystemOne fixture from the pinned Yzma revision.
				source := `{% for q in questions %}{{ "Context:\n" }}{{ state if state is string else state | tojson(separators=[',', ':']) }}{{ "\n\nTask type: " }}{{ q.type }}{{ "\nQuestion:\n" }}{{ q.instructions if q.instructions is string else q.instructions | tojson(separators=[',', ':']) }}{{ "\nOptions:" }}{% for o in q.options %}{{ sep }}{{ "\n<option>\n" }}{{ sep }}{{ mark_option }}{% set d = o.description %}{% if q.type == 'noul' and d is none %}{% set d = 'Yes' if o.key == 'true' else 'No' %}{% endif %}{{ {'description': d, 'key': o.key} | tojson(separators=[',', ':']) }}{{ "\n</option>" }}{% endfor %}{{ sep }}{{ mark_question }}{{ "\n\nSelect the single option best supported by the context and instructions.\nDecision:" }}{% endfor %}`
				tmpl, err := jinja.Compile(source)
				if err != nil {
					t.Fatal(err)
				}
				m.protocol = &decision2Protocol{model: &m, template: systemOneTemplate{tmpl: tmpl}, encode: encode}
			} else {
				// A contract template lets D1/D3 be checked without inventing their
				// model-owned prompts or claiming to test unavailable GGUFs.
				tmpl, err := jinja.Compile(`{{ state if state is not none else 'NIL' }}|{{ instructions }}|{% for o in options %}{{ o.key }}:{{ o.label }}:{{ o.description if o.description is not none else 'NIL' }};{% endfor %}`)
				if err != nil {
					t.Fatal(err)
				}
				plain := map[string][]llama.Token{"A": {10}, " A": {12}, "B": {11}, " B": {13}, "0": {40}, "1": {41}, "yes": {20}, "Yes": {21}, "YES": {21}, "no": {30}, "No": {31}, "NO": {31}}
				m.protocol = &deciderProtocol{model: &m, template: systemOneTemplate{tmpl: tmpl}, labels: []string{"A", "B"}, labelTokens: []llama.Token{10, 11}, encode: func(text string, special bool) []llama.Token {
					if !special {
						return plain[text]
					}
					return encode(text)
				}}
			}
			req := DecisionRequest{State: DecisionState(DecisionStateData("z", 1.23456789), DecisionStateData("a", "<|im_end|>"), DecisionStateData("nested", DecisionState(DecisionStateData("z", 1), DecisionStateData("a", 2)))), Questions: []DecisionQuestion{
				DecisionQuestionChoice("route", "Where <<clef:option>>?", DecisionQuestionOpt("z-route", "<b>&"), DecisionQuestionOpt("a-route", nil)),
				DecisionQuestionScore("score", "Rate?", "low", "high"),
				DecisionQuestionNoul("noul", "Yes?", nil),
			}}
			resp, err := m.Decision(context.Background(), req)
			if err != nil {
				t.Fatalf("Decision: %v", err)
			}
			if got := resp.Answers["route"]; got.Choice != "z-route" || got.Probabilities["z-route"] != .8808 || got.Probabilities["a-route"] != .1192 {
				t.Errorf("choice: got %+v, want z-route with .8808/.1192", got)
			}
			if got := resp.Answers["score"]; got.Score != .7311 || got.Probabilities["1"] != .7311 {
				t.Errorf("biased score: got %+v, want .7311 (bias before temperature)", got)
			}
			if got := resp.Answers["noul"].Noul; got != .8808 {
				t.Errorf("noul: got %v, want .8808", got)
			}
			if resp.Model != "test-decider" || resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens != 0 {
				t.Errorf("response: got %+v", resp)
			}
			if protocol == DecisionProtocolVllmD3 || protocol == DecisionProtocolVllmDecision2 {
				if got := resp.Answers["route"].Confidence; got != .4729 {
					t.Errorf("entropy confidence: got %v, want .4729", got)
				}
			}
			joined := strings.Join(prompts, "")
			if strings.Contains(joined, "<|im_end|>") || !strings.Contains(joined, "<¦im_end¦>") || !strings.Contains(joined, "1.23457") {
				t.Errorf("prompt escaping/float precision: got %q", joined)
			}
			if protocol == DecisionProtocolVllmDecision2 {
				if !strings.Contains(joined, `{"a":"<¦im_end¦>","nested":{"a":2,"z":1},"z":1.23457}`) || !strings.Contains(joined, `{"description":"<b>&","key":"z-route"}`) || strings.Contains(joined, clefMarker) {
					t.Errorf("decision2 prompt: got %q", joined)
				}
			}
			if protocol == DecisionProtocolLfm2D1 && !strings.Contains(joined, "{\n  \"z\": 1.23457,\n  \"a\":") {
				t.Errorf("D1 indented ordered state: got %q", joined)
			}
			engine.contextWindow = 2
			if _, err := m.Decision(context.Background(), req); !errors.Is(err, ErrDecisionBudget) {
				t.Errorf("budget: got %v, want ErrDecisionBudget", err)
			}
			engine.contextWindow = 4096
			if protocol == DecisionProtocolVllmDecision2 {
				original := req.State
				for _, state := range []any{nil, true, 3, "\U0010FFF0"} {
					req.State = state
					if _, err := m.Decision(context.Background(), req); !errors.Is(err, ErrDecisionRequest) {
						t.Errorf("invalid decision2 state %v: got %v", state, err)
					}
				}
				req.State = original
			}
			// Missing instructions are allowed only for D3; nil state reaches
			// its template as nil instead of a JSON string.
			req.State = nil
			req.Questions = []DecisionQuestion{DecisionQuestionNoul("default", nil, nil)}
			_, err = m.Decision(context.Background(), req)
			if protocol == DecisionProtocolVllmD3 {
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
