package model

import (
	"math"
	"reflect"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestJevStyleRenderer(t *testing.T) {
	next := llama.Token(10)
	encode := func(text string) []llama.Token {
		result := make([]llama.Token, len([]rune(text)))
		for i := range result {
			result[i] = next
			next++
		}
		return result
	}
	renderer := jevStyleRenderer{encode: encode, maxLen: 1_000, headMax: 500}

	got, err := renderer.render("customer is angry", DecisionQuestionChoice(
		"route",
		"Where should this go?",
		DecisionQuestionOpt("billing", "money"),
		DecisionQuestionOpt("support", nil),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got.names[0] != "billing" || got.names[1] != "support" {
		t.Fatalf("names: got %v", got.names)
	}
	if len(got.work.readouts) != 2 {
		t.Fatalf("readouts: got %d, want 2", len(got.work.readouts))
	}
	for i, readout := range got.work.readouts {
		if got.work.tokens[readout.position] != jevStyleSlotToken {
			t.Fatalf("readout[%d] token: got %d, want %d", i, got.work.tokens[readout.position], jevStyleSlotToken)
		}
		if !reflect.DeepEqual(readout.candidates, []llama.Token{jevStyleYesToken, jevStyleNoToken}) {
			t.Fatalf("readout[%d] candidates: got %v", i, readout.candidates)
		}
	}
	if got.work.prefixLen <= 0 || got.headTokens <= 0 || got.work.prefixLen+got.headTokens != len(got.work.tokens) {
		t.Fatalf("token accounting: prefix=%d head=%d total=%d", got.work.prefixLen, got.headTokens, len(got.work.tokens))
	}
}

func TestJevStyleAnswers(t *testing.T) {
	probabilities, err := jevStyleSoftmax([]float64{-1, 2, 0})
	if err != nil {
		t.Fatal(err)
	}
	if slicesMaxIndex(probabilities) != 1 {
		t.Fatalf("winner: got %d, want 1", slicesMaxIndex(probabilities))
	}

	noul := answerJevStyleQuestion(DecisionQuestionNoul("q", "true?", nil), []string{"false", "true"}, []float64{0.2, 0.8})
	if math.Abs(noul.Noul-0.8) > 1e-12 {
		t.Fatalf("noul: got %v, want 0.8", noul.Noul)
	}
}

func TestKevTextRenderingAndEscaping(t *testing.T) {
	got, err := decisionKevText(DecisionState(
		DecisionStateData("user", "Ada"),
		DecisionStateData("flags", []any{true, "<|box_end|>"}),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := "user: Ada\nflags:\n  - True\n  - <¦box_end¦>"
	if got != want {
		t.Fatalf("Kev text:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestLayaFitTruncatesHeadAndPreservesState(t *testing.T) {
	p := layaProtocol{
		marker:        99,
		separator:     2,
		maxHeadTokens: 20,
	}
	tokens := []llama.Token{
		1,
		10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24,
		2,
		99, 30, 31, 32, 33, 34, 35,
		99, 40, 41, 42, 43, 44, 45,
		2, 50, 2,
	}

	got, markers, err := p.fit(tokens, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []llama.Token{
		1,
		10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
		2,
		99, 30, 31, 32,
		99, 40, 41, 42,
		2, 50, 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens: got %v, want %v", got, want)
	}
	if wantMarkers := []int{14, 18}; !reflect.DeepEqual(markers, wantMarkers) {
		t.Fatalf("markers: got %v, want %v", markers, wantMarkers)
	}
}

func TestDecisionTemperatureBuckets(t *testing.T) {
	tests := []struct {
		name     string
		protocol DecisionProtocol
		options  int
		metadata map[string]string
		want     float64
	}{
		{
			name:    "generic three to five",
			options: 4,
			metadata: map[string]string{
				"general.architecture":                   "qwen35",
				"qwen35.decision.temperature.choice.3_5": "0.25",
			},
			want: 0.25,
		},
		{
			name:     "lev mid",
			protocol: DecisionProtocolLev,
			options:  9,
			metadata: map[string]string{
				"general.architecture":                   "qwen35",
				"qwen35.decision.temperature.choice.mid": "0.75",
			},
			want: 0.75,
		},
		{
			name:    "unbucketed fallback",
			options: 2,
			metadata: map[string]string{
				"general.architecture":               "qwen35",
				"qwen35.decision.temperature.choice": "1.25",
			},
			want: 1.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{modelInfo: ModelInfo{Metadata: tt.metadata, decisionProtocol: tt.protocol}}
			question := DecisionQuestion{Type: DecisionQuestionTypeChoice, Options: make([]DecisionOption, tt.options)}
			got, err := decisionTemperature(&m, question)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("temperature: got %v, want %v", got, tt.want)
			}
		})
	}
}
