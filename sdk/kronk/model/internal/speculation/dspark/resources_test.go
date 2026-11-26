package dspark

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestContextStatePositionAndSequenceIsolation(t *testing.T) {
	r := Resources{width: 2, sequences: []sequence{newSequence(2, 2), newSequence(2, 2)}}
	r.sequences[0].store(0, []float32{11, 13})
	r.sequences[1].store(0, []float32{71, 73})
	for pos := llama.Pos(1); pos <= 3; pos++ {
		r.sequences[0].store(pos, []float32{float32(pos), -float32(pos)})
	}
	if got := r.ContextState(0, 0); got != nil {
		t.Errorf("overwritten position: got %v, want nil", got)
	}
	if got := r.ContextState(0, 3); !slices.Equal(got, []float32{3, -3}) {
		t.Errorf("latest state: got %v, want [3 -3]", got)
	}
	if got := r.ContextState(1, 0); !slices.Equal(got, []float32{71, 73}) {
		t.Errorf("other sequence: got %v, want [71 73]", got)
	}
	r.sequences[0].reset()
	if r.ContextState(0, 3) != nil || r.NextPosition(0) != 0 || r.ContextState(1, 0) == nil {
		t.Fatal("reset retained stale state or changed another sequence")
	}
}

func TestSynchronizeRejectsMissingAcceptedPrefix(t *testing.T) {
	for _, rows := range [][]FeatureRow{
		{{Row: 0, Sequence: 0, Position: 3}},
		{{Row: 0, Sequence: 0, Position: 0}, {Row: 1, Sequence: 0, Position: 2}},
		{{Row: 0, Sequence: 0, Position: 0}, {Row: 1, Sequence: 1, Position: 1}},
		{{Row: 2, Sequence: 0, Position: 0}},
	} {
		r := Resources{rows: 2, sequences: []sequence{newSequence(2, 2)}}
		if err := r.Synchronize(rows); err == nil {
			t.Fatalf("invalid accepted prefix %v was permitted", rows)
		}
		if r.NextPosition(0) != 0 || r.ContextState(0, 0) != nil {
			t.Fatalf("rejected prefix %v changed state", rows)
		}
	}
}
