package model

import (
	"math"
	"testing"
)

func TestOpenJEVPrompt(t *testing.T) {
	state, err := decisionState(DecisionState(
		DecisionStateData("user", "Ada"),
		DecisionStateData("active", true),
	))
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := openJEVInstructions(DecisionState(
		DecisionStateData("goal", "route"),
		DecisionStateData("priority", 2),
	))
	if err != nil {
		t.Fatal(err)
	}
	description, err := decisionDescription(DecisionState(
		DecisionStateData("team", "technical"),
		DecisionStateData("urgent", false),
	))
	if err != nil {
		t.Fatal(err)
	}

	got := openJEVPrompt(state, instructions, []openJEVOption{
		{name: "billing", description: "Payment help"},
		{name: "support", description: description},
	})
	want := "State:\n{\"user\": \"Ada\", \"active\": true}\n\n" +
		"Question: {'goal': 'route', 'priority': 2}\n" +
		"Options:\n" +
		"[A] billing: Payment help\n" +
		"[B] support: {\"team\": \"technical\", \"urgent\": false}\n\n" +
		"Answer with the letter of the best option only."
	if got != want {
		t.Fatalf("prompt:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestOpenJEVReadoutMath(t *testing.T) {
	probabilities, err := openJEVSoftmax([]float32{2, 0, -1})
	if err != nil {
		t.Fatal(err)
	}
	if diff := math.Abs(probabilities[0] - 0.8893543288042062); diff > 1e-12 {
		t.Fatalf("top probability: got %.8f, want 0.88935433", probabilities[0])
	}
	if diff := math.Abs(openJEVChoiceConfidence([]float64{0.7, 0.1, 0.1, 0.1}) - 0.6); diff > 1e-12 {
		t.Fatalf("choice confidence: got %.8f, want 0.6", openJEVChoiceConfidence([]float64{0.7, 0.1, 0.1, 0.1}))
	}
	if got := openJEVScoreConfidence([]float64{0, 0, 1}); got != 1 {
		t.Fatalf("score confidence: got %.8f, want 1", got)
	}
	if got := openJEVNoul(0.8); math.Abs(got-0.6809022812912505) > 1e-12 {
		t.Fatalf("noul: got %.8f, want 0.68090228", got)
	}
}

func TestOpenJEVChunkComposition(t *testing.T) {
	options := make([]openJEVOption, 105)
	chunks := openJEVChunks(options)
	if len(chunks) != 3 || len(chunks[0]) != 35 || len(chunks[1]) != 35 || len(chunks[2]) != 35 {
		t.Fatalf("chunks: got lengths %d/%d/%d, want 35/35/35", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}

	got := openJEVComposeChunks(
		[][]float64{{0.75, 0.25}, {0.2, 0.8}},
		[]int{0, 1},
		[]float64{0.4, 0.6},
	)
	want := []float64{0.4, 0.4 / 3, 0.15, 0.6}
	var sum float64
	for i := range want {
		sum += want[i]
	}
	for i := range want {
		want[i] /= sum
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("probability[%d]: got %.12f, want %.12f", i, got[i], want[i])
		}
	}
}

func TestPythonRepresentation(t *testing.T) {
	got, err := decisionPythonRepr(DecisionState(
		DecisionStateData("text", "it's fine"),
		DecisionStateData("flags", []any{true, nil, "é"}),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := `{'text': "it's fine", 'flags': [True, None, 'é']}`
	if got != want {
		t.Fatalf("representation: got %s, want %s", got, want)
	}
}

func TestDecisionJSONMatchesPythonSeparators(t *testing.T) {
	line := "before" + string(rune(0x2028)) + "after"
	got, err := decisionJSON(DecisionState(
		DecisionStateData("line", line),
		DecisionStateData("items", []any{1, true}),
	))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"line\": \"" + line + "\", \"items\": [1, true]}"
	if got != want {
		t.Fatalf("json: got %q, want %q", got, want)
	}
}
