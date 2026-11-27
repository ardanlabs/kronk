package speculation

import (
	"math"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestVerifyDistributionAcceptanceBoundaries(t *testing.T) {
	// p=(1/4,3/8,1/8,1/4), q=(3/4,1/4,0,0).
	// Candidate 0 accepts below 1/3; candidate 1 always accepts (p/q=3/2).
	tests := []struct {
		name      string
		candidate llama.Token
		draw      float64
		accepted  int
		calls     int
	}{
		{name: "zero", draw: 0, accepted: 1, calls: 1},
		{name: "below ratio", draw: math.Nextafter(1.0/3, 0), accepted: 1, calls: 1},
		{name: "at ratio", draw: 1.0 / 3, calls: 2},
		{name: "above ratio", draw: math.Nextafter(1.0/3, 1), calls: 2},
		{name: "near one", draw: math.Nextafter(1, 0), calls: 2},
		{name: "ratio exceeds one", candidate: 1, accepted: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			accepted := 0
			rows := 0
			got := VerifyDistribution(DistributionInput{
				Candidates:    []llama.Token{tt.candidate},
				Distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 0.75}, {Tok: 1, Prob: 0.25}}},
				Target: func(index int) DistributionTarget {
					if index != rows {
						t.Fatalf("Target index: got %d, want %d", index, rows)
					}
					rows++
					if index == 1 {
						return DistributionTarget{Token: 7, SamplerAccepted: true}
					}
					return DistributionTarget{Probabilities: []float32{0.25, 0.375, 0.125, 0.25}, SamplerAccepted: true}
				},
				Accept: func(index int, token llama.Token, samplerAccepted bool) bool {
					accepted++
					if index != 0 || token != tt.candidate || !samplerAccepted {
						t.Errorf("Accept: got (%d, %d, %t), want (0, %d, true)", index, token, samplerAccepted, tt.candidate)
					}
					return true
				},
				Random: func() float64 {
					calls++
					if calls == 1 {
						return tt.draw
					}
					return 0.75
				},
			})
			want := DistributionResult{Drafted: 1, Complete: true, Bonus: 3}
			if tt.accepted == 1 {
				want.Accepted = 1
				want.Bonus = 7
				want.SamplerAccepted = true
			}
			if got != want || accepted != tt.accepted || calls != tt.calls || rows != 1+tt.accepted {
				t.Errorf("got (%+v, accepted=%d, draws=%d, rows=%d), want (%+v, accepted=%d, draws=%d, rows=%d)", got, accepted, calls, rows, want, tt.accepted, tt.calls, 1+tt.accepted)
			}
		})
	}
}

func TestVerifyDistributionResidualBoundaries(t *testing.T) {
	// Positive p-q is (0,1/8,1/8,1/4), total 1/2. Its normalized
	// law is (0,1/4,1/4,1/2): both tokens 2 and 3 are outside q support.
	tests := []struct {
		name string
		draw float64
		want llama.Token
	}{
		{name: "start", draw: 0, want: 1},
		{name: "below sparse boundary", draw: math.Nextafter(0.25, 0), want: 1},
		{name: "at sparse boundary", draw: 0.25, want: 2},
		{name: "above sparse boundary", draw: math.Nextafter(0.25, 1), want: 2},
		{name: "below outside boundary", draw: math.Nextafter(0.5, 0), want: 2},
		{name: "at outside boundary", draw: 0.5, want: 3},
		{name: "above outside boundary", draw: math.Nextafter(0.5, 1), want: 3},
		{name: "end", draw: math.Nextafter(1, 0), want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draws := 0
			got := VerifyDistribution(DistributionInput{
				Candidates:    []llama.Token{0},
				Distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 0.75}, {Tok: 1, Prob: 0.25}}},
				Target: func(index int) DistributionTarget {
					if index != 0 {
						t.Fatalf("Target index: got %d, want 0", index)
					}
					return DistributionTarget{Probabilities: []float32{0.25, 0.375, 0.125, 0.25}, SamplerAccepted: true}
				},
				Accept: func(int, llama.Token, bool) bool { t.Fatal("unexpected Accept"); return false },
				Random: func() float64 {
					draws++
					if draws == 1 {
						return 0.9
					}
					return tt.draw
				},
				Random32: func() float32 { t.Fatal("unexpected target draw"); return 0 },
			})
			want := DistributionResult{Drafted: 1, Bonus: tt.want, Complete: true}
			if got != want || draws != 2 {
				t.Errorf("got (%+v, draws=%d), want (%+v, draws=2)", got, draws, want)
			}
		})
	}
}

func TestVerifyDistributionAsymmetricMass(t *testing.T) {
	// Enumerate equal-mass cells, not seeded samples. Proposal cells have mass
	// 1/4, acceptance cells 1/3, residual cells 1/4. Independent calculation:
	// accepted mass=(1/4,1/4,0,0), rejected mass=1/2;
	// residual=(0,1/4,1/4,1/2), yielding p=(1/4,3/8,1/8,1/4).
	// This checks this exact law; it is not a general distributional proof.
	var counts [4]int
	for _, candidate := range []llama.Token{0, 0, 0, 1} {
		for _, acceptance := range []float64{1.0 / 6, 0.5, 5.0 / 6} {
			for _, residual := range []float64{0.125, 0.375, 0.625, 0.875} {
				draws := 0
				got := VerifyDistribution(DistributionInput{
					Candidates:    []llama.Token{candidate},
					Distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 0.75}, {Tok: 1, Prob: 0.25}}},
					Target: func(index int) DistributionTarget {
						if index == 1 {
							return DistributionTarget{Token: 7}
						}
						return DistributionTarget{Probabilities: []float32{0.25, 0.375, 0.125, 0.25}}
					},
					Accept: func(int, llama.Token, bool) bool { return true },
					Random: func() float64 {
						draws++
						if draws == 1 {
							return acceptance
						}
						return residual
					},
				})
				token := got.Bonus
				if got.Accepted == 1 {
					token = candidate
				}
				counts[token]++
			}
		}
	}
	if want := [4]int{12, 18, 6, 12}; counts != want {
		t.Errorf("counts: got %v, want %v", counts, want)
	}
}

func TestVerifyDistributionZeroSupport(t *testing.T) {
	tests := []struct {
		name          string
		distributions [][]llama.DraftCandidate
		probabilities []float32
		want          llama.Token
	}{
		{name: "missing draft", probabilities: []float32{0.25, 0, 0.75}, want: 2},
		{name: "missing row", distributions: [][]llama.DraftCandidate{}, probabilities: []float32{0.25, 0, 0.75}, want: 2},
		{name: "empty row", distributions: [][]llama.DraftCandidate{{}}, probabilities: []float32{0.25, 0, 0.75}, want: 2},
		{name: "candidate absent", distributions: [][]llama.DraftCandidate{{{Tok: 1, Prob: 1}}}, probabilities: []float32{0.25, 0, 0.75}, want: 2},
		{name: "candidate zero", distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 0}, {Tok: 1, Prob: 1}}}, probabilities: []float32{0.25, 0, 0.75}, want: 2},
		{name: "missing target", distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 1}}}, want: 7},
		{name: "target zero at candidate", distributions: [][]llama.DraftCandidate{{{Tok: 0, Prob: 1}}}, probabilities: []float32{0, 0.25, 0.75}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyDistribution(DistributionInput{
				Candidates:    []llama.Token{0},
				Distributions: tt.distributions,
				Target: func(index int) DistributionTarget {
					if index != 0 {
						t.Fatalf("Target index: got %d, want 0", index)
					}
					return DistributionTarget{Token: 7, Probabilities: tt.probabilities, SamplerAccepted: true}
				},
				Accept:   func(int, llama.Token, bool) bool { t.Fatal("unexpected Accept"); return false },
				Random:   func() float64 { return 0.5 },
				Random32: func() float32 { return 0.5 },
			})
			want := DistributionResult{Drafted: 1, Bonus: tt.want, Complete: true, SamplerAccepted: tt.name != "target zero at candidate"}
			if got != want {
				t.Errorf("VerifyDistribution: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestVerifyDistributionFullAcceptanceBonus(t *testing.T) {
	// All candidate ratios exceed one, so only Random32 may be called for the
	// bonus. Enumerate every bonus CDF boundary of (1/8,0,3/8,1/2).
	tests := []struct {
		name string
		draw float32
		want llama.Token
	}{
		{name: "zero", draw: 0, want: 0},
		{name: "below first", draw: math.Nextafter32(0.125, 0), want: 0},
		{name: "at first skips zero", draw: 0.125, want: 2},
		{name: "above first", draw: math.Nextafter32(0.125, 1), want: 2},
		{name: "below second", draw: math.Nextafter32(0.5, 0), want: 2},
		{name: "at second", draw: 0.5, want: 3},
		{name: "above second", draw: math.Nextafter32(0.5, 1), want: 3},
		{name: "near one", draw: math.Nextafter32(1, 0), want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, accepts, draws := 0, 0, 0
			got := VerifyDistribution(DistributionInput{
				Candidates: []llama.Token{0, 1},
				Distributions: [][]llama.DraftCandidate{
					{{Tok: 0, Prob: 0.25}, {Tok: 1, Prob: 0.75}},
					{{Tok: 0, Prob: 0.75}, {Tok: 1, Prob: 0.25}},
				},
				Target: func(index int) DistributionTarget {
					if index != rows {
						t.Fatalf("Target index: got %d, want %d", index, rows)
					}
					rows++
					if index == 2 {
						return DistributionTarget{Probabilities: []float32{0.125, 0, 0.375, 0.5}, SamplerAccepted: true}
					}
					return DistributionTarget{Probabilities: []float32{0.5, 0.5}, SamplerAccepted: index == 0}
				},
				Accept: func(index int, token llama.Token, samplerAccepted bool) bool {
					if index != accepts || token != llama.Token(index) || samplerAccepted != (index == 0) {
						t.Errorf("Accept: got (%d, %d, %t), want (%d, %d, %t)", index, token, samplerAccepted, accepts, accepts, index == 0)
					}
					accepts++
					return true
				},
				Random:   func() float64 { t.Fatal("unexpected acceptance draw"); return 0 },
				Random32: func() float32 { draws++; return tt.draw },
			})
			want := DistributionResult{Accepted: 2, Drafted: 2, Bonus: tt.want, SamplerAccepted: true, Complete: true}
			if got != want || rows != 3 || accepts != 2 || draws != 1 {
				t.Errorf("got (%+v, rows=%d, accepts=%d, draws=%d), want (%+v, rows=3, accepts=2, draws=1)", got, rows, accepts, draws, want)
			}
		})
	}
}

func TestVerifyDistributionOutputStop(t *testing.T) {
	got := VerifyDistribution(DistributionInput{
		Candidates: []llama.Token{1, 2},
		Greedy:     true,
		Target: func(index int) DistributionTarget {
			if index != 0 {
				t.Fatalf("Target index: got %d, want 0", index)
			}
			return DistributionTarget{Token: 1, SamplerAccepted: true}
		},
		Accept: func(index int, token llama.Token, samplerAccepted bool) bool {
			if index != 0 || token != 1 || !samplerAccepted {
				t.Errorf("Accept: got (%d, %d, %t), want (0, 1, true)", index, token, samplerAccepted)
			}
			return false
		},
	})
	if want := (DistributionResult{Accepted: 1, Drafted: 2}); got != want {
		t.Errorf("VerifyDistribution: got %+v, want %+v", got, want)
	}
}
