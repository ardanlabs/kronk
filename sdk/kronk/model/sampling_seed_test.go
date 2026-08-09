package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestParseSeed(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    uint32
		wantErr bool
	}{
		{name: "zero", value: 0, want: 0},
		{name: "float64 integer", value: float64(42), want: 42},
		{name: "json number", value: json.Number("43"), want: 43},
		{name: "json number decimal", value: json.Number("44.0"), want: 44},
		{name: "json number exponent", value: json.Number("4.5e1"), want: 45},
		{name: "string", value: "44", want: 44},
		{name: "maximum", value: uint64(math.MaxUint32), want: math.MaxUint32},
		{name: "negative", value: -1, wantErr: true},
		{name: "fractional", value: 1.5, wantErr: true},
		{name: "too large", value: uint64(math.MaxUint32) + 1, wantErr: true},
		{name: "nan", value: math.NaN(), wantErr: true},
		{name: "positive infinity", value: math.Inf(1), wantErr: true},
		{name: "malformed", value: "abc", wantErr: true},
		{name: "boolean", value: true, wantErr: true},
		{name: "nil", value: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSeed(tt.value)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("parseSeed(%v): got %v, want ErrInvalidRequest", tt.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSeed(%v): %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseSeed(%v): got %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseParamsSeedPresence(t *testing.T) {
	defaultSeed := uint32(99)
	m := Model{
		cfg: Config{DefaultParams: Params{Seed: &defaultSeed}},
		log: noopLog,
	}

	omitted, err := m.parseParams(t.Context(), D{})
	if err != nil {
		t.Fatalf("parseParams omitted seed: %v", err)
	}
	if omitted.Seed == nil || *omitted.Seed != defaultSeed {
		t.Errorf("omitted Seed: got %v, want pointer to %d", omitted.Seed, defaultSeed)
	}

	explicit, err := m.parseParams(t.Context(), D{"seed": 0})
	if err != nil {
		t.Fatalf("parseParams explicit zero seed: %v", err)
	}
	if explicit.Seed == nil || *explicit.Seed != 0 {
		t.Errorf("explicit Seed: got %v, want pointer to 0", explicit.Seed)
	}

	d := D{}
	AddParams(explicit, d)
	if got, exists := d["seed"]; !exists || got != uint32(0) {
		t.Errorf("AddParams seed: got %v, exists %t, want uint32(0)", got, exists)
	}

	random, err := (&Model{log: noopLog}).parseParams(t.Context(), D{})
	if err != nil {
		t.Fatalf("parseParams random seed: %v", err)
	}
	if random.Seed != nil {
		t.Errorf("random Seed: got %v, want nil", *random.Seed)
	}
}

func TestAddParamsClonesStop(t *testing.T) {
	params := Params{Stop: []string{"END"}}
	d := D{}

	AddParams(params, d)
	got, ok := d["stop"].([]string)
	if !ok {
		t.Fatalf("stop: got %T, want []string", d["stop"])
	}
	params.Stop[0] = "changed"
	if got[0] != "END" {
		t.Errorf("stop: got %q after source mutation, want %q", got[0], "END")
	}
}

func TestResolveSamplingSeeds(t *testing.T) {
	seed := uint32(math.MaxUint32)
	got1, rng1, err := resolveSamplingSeedsFrom(&seed, nil)
	if err != nil {
		t.Fatalf("resolveSamplingSeedsFrom first: %v", err)
	}
	got2, rng2, err := resolveSamplingSeedsFrom(&seed, nil)
	if err != nil {
		t.Fatalf("resolveSamplingSeedsFrom second: %v", err)
	}
	if got1 != got2 {
		t.Errorf("sampling seeds: got %+v and %+v, want equal", got1, got2)
	}
	if got1.master != seed || got1.generated {
		t.Errorf("master seed: got %d generated[%t], want %d generated[false]", got1.master, got1.generated, seed)
	}
	for name, nativeSeed := range map[string]uint32{
		"target dist":       got1.targetDist,
		"target XTC":        got1.targetXTC,
		"target Adaptive-P": got1.targetAdaptiveP,
		"draft dist":        got1.draftDist,
	} {
		if nativeSeed == llama.DefaultSeed {
			t.Errorf("%s seed: got llama.DefaultSeed, want concrete seed", name)
		}
	}
	if rng1.Uint64() != rng2.Uint64() {
		t.Error("speculative RNG: got different first values for the same master seed")
	}
}

func TestResolveSamplingSeedsOmitted(t *testing.T) {
	entropy := []byte{1, 2, 3, 4}
	seeds, rng, err := resolveSamplingSeedsFrom(nil, bytes.NewReader(entropy))
	if err != nil {
		t.Fatalf("resolveSamplingSeedsFrom: %v", err)
	}
	if seeds.master != 0x04030201 || !seeds.generated {
		t.Errorf("master seed: got %d generated[%t], want %d generated[true]", seeds.master, seeds.generated, uint32(0x04030201))
	}

	replayed, replayRNG, err := resolveSamplingSeedsFrom(&seeds.master, nil)
	if err != nil {
		t.Fatalf("resolveSamplingSeedsFrom replay: %v", err)
	}
	want := seeds
	want.generated = false
	if replayed != want {
		t.Errorf("replayed seeds: got %+v, want %+v", replayed, want)
	}
	if got, want := rng.Uint64(), replayRNG.Uint64(); got != want {
		t.Errorf("replayed speculative RNG: got %d, want %d", got, want)
	}

	if _, _, err := resolveSamplingSeedsFrom(nil, strings.NewReader("")); err == nil {
		t.Fatal("empty entropy: got nil error, want error")
	}
}

func TestSeededProbabilitySamplingIsRepeatable(t *testing.T) {
	probs := []float32{0.1, 0.2, 0.3, 0.4}
	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))

	for range 20 {
		got := sampleFromProbs(rng1, probs)
		want := sampleFromProbs(rng2, probs)
		if got != want {
			t.Fatalf("sampleFromProbs: got %d, want %d", got, want)
		}
	}
}
