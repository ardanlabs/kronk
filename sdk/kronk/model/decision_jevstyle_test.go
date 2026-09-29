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
