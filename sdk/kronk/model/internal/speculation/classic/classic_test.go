package classic

import (
	"math"
	"math/rand"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestGenerateAdaptiveSizing(t *testing.T) {
	tests := []struct {
		name     string
		ema      float64
		probe    int
		want     int
		wantTick int
	}{
		{name: "full draft", ema: 1, want: 4},
		{name: "three drafts", ema: 0.85, want: 4},
		{name: "two drafts", ema: 0.70, want: 3},
		{name: "one draft", ema: 0.30, want: 1},
		{name: "throttled", ema: 0.29, wantTick: 1},
		{name: "recovery probe", ema: 0.29, probe: probeInterval - 1, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := SlotState{AcceptanceEMA: tt.ema, ProbeTick: tt.probe}
			gotCount := -1
			_, err := Generate(GenerationInput{State: &state, MaxDraft: 4, Generate: func(count int) (GenerationResult, error) {
				gotCount = count
				return GenerationResult{}, nil
			}})
			if err != nil {
				t.Fatalf("Generate() error = %v, want nil", err)
			}
			if tt.want == 0 {
				gotCount = 0
			}
			if gotCount != tt.want {
				t.Errorf("draft count = %d, want %d", gotCount, tt.want)
			}
			if state.ProbeTick != tt.wantTick {
				t.Errorf("ProbeTick = %d, want %d", state.ProbeTick, tt.wantTick)
			}
		})
	}
}

func TestVerifyGreedy(t *testing.T) {
	state := SlotState{AcceptanceEMA: 1}
	result := Verify(VerifyInput{
		State:      &state,
		Candidates: []llama.Token{1, 2},
		Greedy:     true,
		Target: func(index int) Target {
			return []Target{{Token: 1}, {Token: 3}}[index]
		},
		Accept: func(int, llama.Token, bool) bool { return true },
	})
	if result.Accepted != 1 || result.Bonus != 3 || !result.Complete {
		t.Errorf("Verify() = %+v, want one accepted token and bonus 3", result)
	}
	if math.Abs(state.AcceptanceEMA-0.95) > 1e-9 {
		t.Errorf("AcceptanceEMA = %f, want 0.95", state.AcceptanceEMA)
	}
}

func TestVerifyProbabilisticAcceptance(t *testing.T) {
	state := SlotState{AcceptanceEMA: 1}
	random := []float64{0.25, 0.9}
	result := Verify(VerifyInput{
		State:         &state,
		Candidates:    []llama.Token{1},
		Distributions: [][]Candidate{{{Token: 1, Probability: 0.4}}},
		Target: func(int) Target {
			return Target{Token: 2, Probabilities: []float32{0.1, 0.2, 0.7}}
		},
		Accept: func(int, llama.Token, bool) bool { return true },
		Random: func() float64 {
			value := random[0]
			random = random[1:]
			return value
		},
	})
	if result.Accepted != 1 || result.Bonus != 2 || !result.Complete {
		t.Errorf("Verify() = %+v, want accepted draft and bonus 2", result)
	}
}

func TestVerifyAdjustedRejectionSampling(t *testing.T) {
	state := SlotState{AcceptanceEMA: 1}
	random := []float64{0.9, 0.5}
	result := Verify(VerifyInput{
		State:         &state,
		Candidates:    []llama.Token{0},
		Distributions: [][]Candidate{{{Token: 0, Probability: 0.8}}},
		Target: func(int) Target {
			return Target{Probabilities: []float32{0.2, 0.8}}
		},
		Accept: func(int, llama.Token, bool) bool { return true },
		Random: func() float64 {
			value := random[0]
			random = random[1:]
			return value
		},
	})
	if result.Accepted != 0 || result.Bonus != 1 || !result.Complete {
		t.Errorf("Verify() = %+v, want adjusted bonus 1", result)
	}
}

func TestSlotStateReset(t *testing.T) {
	state := SlotState{AcceptanceEMA: 0.2, ProbeTick: 7, Rounds: 4, DraftStartPosition: 12, DraftDistributions: [][]Candidate{{{Token: 1}}}, AdjustedScratch: []Candidate{{Token: 2}}}
	state.Reset()
	if state.AcceptanceEMA != 1 || state.ProbeTick != 0 || state.Rounds != 0 || state.DraftStartPosition != 0 || state.DraftDistributions != nil || len(state.AdjustedScratch) != 0 {
		t.Errorf("Reset() state = %+v, want clean request state", state)
	}
}

func TestDraftKeepPosition(t *testing.T) {
	tests := []struct {
		name     string
		start    llama.Pos
		end      llama.Pos
		accepted int
		want     llama.Pos
	}{
		{name: "partial acceptance", start: 100, end: 104, accepted: 1, want: 102},
		{name: "full acceptance", start: 100, end: 104, accepted: 4, want: 104},
		{name: "truncated draft", start: 100, end: 103, accepted: 2, want: 103},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DraftKeepPosition(tt.start, tt.end, tt.accepted); got != tt.want {
				t.Errorf("DraftKeepPosition() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProbabilitySamplingIsRepeatable(t *testing.T) {
	probabilities := []float32{0.1, 0.2, 0.3, 0.4}
	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))
	input1 := VerifyInput{Random32: rng1.Float32}
	input2 := VerifyInput{Random32: rng2.Float32}

	for range 20 {
		got := sampleProbabilities(input1, probabilities)
		want := sampleProbabilities(input2, probabilities)
		if got != want {
			t.Fatalf("sampleProbabilities() = %d, want %d", got, want)
		}
	}
}
