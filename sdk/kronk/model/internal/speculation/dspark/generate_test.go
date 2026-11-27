package dspark

import (
	"math"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestGreedyCount(t *testing.T) {
	for _, tt := range []struct {
		name                                     string
		requested, limit, batch, ubatch, context int
		position                                 llama.Pos
		want                                     int
	}{
		{"trained", 99, 7, 256, 256, 1024, 24, 7},
		{"configured", 7, 3, 256, 256, 1024, 24, 3},
		{"requested", 2, 7, 256, 256, 1024, 24, 2},
		{"logical", 7, 7, 4, 256, 1024, 24, 3},
		{"physical", 7, 7, 256, 4, 1024, 24, 3},
		{"context", 7, 7, 256, 256, 28, 24, 3},
		{"anchor only", 7, 7, 256, 256, 25, 24, 0},
		{"no physical room", 7, 7, 256, 1, 1024, 24, 0},
		{"negative", -1, 7, 256, 256, 1024, 24, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := greedyCount(tt.requested, tt.limit, tt.batch, tt.ubatch, tt.context, tt.position); got != tt.want {
				t.Errorf("count: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPackedToken(t *testing.T) {
	for _, value := range []float32{-1, 200058, 1.5, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if _, err := packedToken(value, 200058); err == nil {
			t.Errorf("value %g: got nil error, want invalid token", value)
		}
	}
	for _, value := range []float32{0, 1, 200057} {
		got, err := packedToken(value, 200058)
		if err != nil || got != llama.Token(value) {
			t.Errorf("value %g: got %d/%v, want %d/nil", value, got, err, llama.Token(value))
		}
	}
}

func TestGenerateRejectsUnusablePrefix(t *testing.T) {
	r := Resources{width: 2, sequences: []sequence{newSequence(8, 2)}}
	if _, err := r.Generate(-1, 0, 1, 7, 0, nil); err == nil {
		t.Fatal("invalid sequence accepted")
	}
	r.sequences[0].store(0, []float32{1, 2})
	if _, err := r.Generate(0, 0, 2, 7, 0, nil); err == nil || !r.sequences[0].disabled {
		t.Fatal("missing prefix did not disable sequence")
	}
	r.sequences[0].reset()
	r.sequences[0].store(0, []float32{float32(math.NaN()), 2})
	if _, err := r.Generate(0, 0, 1, 7, 0, nil); err == nil || !r.sequences[0].disabled {
		t.Fatal("nonfinite state did not disable sequence")
	}
}

func TestPackedDistribution(t *testing.T) {
	row := []float32{3, 3, 0, 3, 7, 0.2, 0.5, 0.3, 999}
	got, err := packedDistribution(row, 3, 8)
	want := []llama.DraftCandidate{{Tok: 0, Prob: 0.2}, {Tok: 3, Prob: 0.5}, {Tok: 7, Prob: 0.3}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("law: got %v/%v, want %v", got, err, want)
	}
	zeroMass := []float32{3, 3, 0, 3, 7, 0, 1, 0}
	got, err = packedDistribution(zeroMass, 3, 8)
	if err != nil || !slices.Equal(got, []llama.DraftCandidate{{Tok: 3, Prob: 1}}) {
		t.Fatalf("zero support: got %v/%v, want only selected token with mass one", got, err)
	}
	for _, tt := range []struct {
		name  string
		index int
		value float32
	}{
		{"count zero", 1, 0}, {"count fractional", 1, 2.5}, {"count out of row", 1, 5},
		{"count NaN", 1, float32(math.NaN())}, {"count infinite", 1, float32(math.Inf(1))},
		{"duplicate", 4, 3}, {"invalid token", 4, 8}, {"fractional token", 4, 6.5},
		{"negative mass", 5, -0.2}, {"NaN mass", 5, float32(math.NaN())},
		{"infinite mass", 5, float32(math.Inf(1))}, {"unnormalized", 5, 0.1},
		{"selected token unsupported", 6, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bad := slices.Clone(row)
			bad[tt.index] = tt.value
			if _, err := packedDistribution(bad, 3, 8); err == nil {
				t.Fatal("invalid graph law accepted")
			}
		})
	}
	if _, err := packedDistribution(row, 2, 8); err == nil {
		t.Fatal("selected token absent from sparse support accepted")
	}
	if _, err := packedDistribution(row[:1], 3, 8); err == nil {
		t.Fatal("truncated header accepted")
	}
}
