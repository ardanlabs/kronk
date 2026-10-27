package model

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/trace"
)

func TestNextPrefillSlotStartsAtCursorAndSkipsIneligibleSlots(t *testing.T) {
	e := batchEngine{
		prefillNext: 2,
		slots: []*slot{
			{id: 0, active: true, prefillTokens: []llama.Token{1}},
			{id: 1, active: false, prefillTokens: []llama.Token{1}},
			{id: 2, active: true},
			{id: 3, active: true, prefillTokens: []llama.Token{1}},
		},
	}

	s, idx := e.nextPrefillSlot()
	if s == nil {
		t.Fatal("nextPrefillSlot() slot = nil, want slot 3")
	}
	if idx != 3 || s.id != 3 {
		t.Errorf("nextPrefillSlot() = slot %d at %d, want slot 3 at 3", s.id, idx)
	}

	e.prefillNext = 0
	s, idx = e.nextPrefillSlot()
	if s == nil {
		t.Fatal("nextPrefillSlot() after wrap slot = nil, want slot 0")
	}
	if idx != 0 || s.id != 0 {
		t.Errorf("nextPrefillSlot() after wrap = slot %d at %d, want slot 0 at 0", s.id, idx)
	}
}

func TestNextPrefillSlotReturnsNone(t *testing.T) {
	e := batchEngine{slots: []*slot{{active: true}, {active: false, prefillTokens: []llama.Token{1}}}}

	s, idx := e.nextPrefillSlot()
	if s != nil || idx != -1 {
		t.Errorf("nextPrefillSlot() = (%v, %d), want (nil, -1)", s, idx)
	}
}

func TestPrefillSlotIDsReturnsEveryEligibleSlot(t *testing.T) {
	e := batchEngine{slots: []*slot{
		{id: 0, active: true, prefillTokens: []llama.Token{1}},
		{id: 1, active: true},
		{id: 2, active: false, prefillTokens: []llama.Token{1}},
		{id: 3, active: true, prefillTokens: []llama.Token{1}},
	}}

	got := e.prefillSlotIDs()
	if len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Errorf("prefillSlotIDs() = %v, want [0 3]", got)
	}
}

func TestPrefillContributionSizeUsesSpaceRemainingAfterGenerationRows(t *testing.T) {
	tests := []struct {
		name               string
		remaining          int
		availableAfterRows int
		chunkLimit         int
		want               int
	}{
		{"non-MTP padded tray", 4096, 2050 - 1, 2048, 2048},
		{"MTP padded tray", 4096, 2056 - 4, 2048, 2048},
		{"explicit tray reduces prefill", 4096, 2048 - 4, 2048, 2044},
		{"remaining prompt limits prefill", 512, 2056 - 4, 2048, 512},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prefillContributionSize(tt.remaining, tt.availableAfterRows, tt.chunkLimit)
			if got != tt.want {
				t.Errorf("prefill contribution: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAddPrefillChunkStagesOnlyFinalPromptLogits(t *testing.T) {
	s := &slot{
		id:            0,
		active:        true,
		seqIDs:        []llama.SeqId{0},
		job:           &chatJob{ctx: t.Context()},
		span:          trace.SpanFromContext(t.Context()),
		prefillTokens: []llama.Token{11, 22, 33},
		nPast:         17,
		iBatch:        -1,
	}
	e := batchEngine{
		model:      &Model{cfg: Config{nBatch: 4}},
		batch:      &extendedBatch{capacity: 4},
		slots:      []*slot{s},
		shutdownCh: make(chan struct{}),
	}
	e.speculation = speculation.NewDisabled(&e)

	pending, ok := e.addPrefillChunk(s, 2)
	if !ok {
		t.Fatal("first addPrefillChunk returned false")
	}
	if e.batch.len() != 2 || s.nPrefilled != 0 || s.nPast != 17 || s.iBatch != -1 || len(s.prefillTokens) != 3 {
		t.Fatalf("staged prefill = rows %d, prefilled %d, past %d, index %d; want 2, 0, 17, -1",
			e.batch.len(), s.nPrefilled, s.nPast, s.iBatch)
	}
	for i, entry := range e.batch.entries {
		if entry.output != extendedBatchOutputNone {
			t.Fatalf("partial prefill entry %d output = %d, want none", i, entry.output.value)
		}
		if entry.positions[0] != llama.Pos(17+i) {
			t.Fatalf("partial prefill entry %d position = %d, want %d", i, entry.positions[0], 17+i)
		}
	}
	pending.commit()
	if s.nPrefilled != 2 || s.nPast != 19 || s.iBatch != -1 || len(s.prefillTokens) != 3 {
		t.Fatalf("committed partial prefill = prefilled %d, past %d, index %d, tokens %v; want 2, 19, -1, [11 22 33]",
			s.nPrefilled, s.nPast, s.iBatch, s.prefillTokens)
	}

	e.batch.clear()
	// Another slot's generation row precedes the final prefill row.
	if _, err := e.batch.addToken(44, 9, []llama.SeqId{1}, extendedBatchOutputLogits); err != nil {
		t.Fatal(err)
	}
	pending, ok = e.addPrefillChunk(s, 2)
	if !ok {
		t.Fatal("final addPrefillChunk returned false")
	}
	if !pending.complete || pending.outputIndex != 1 || pending.nextPast != 20 {
		t.Fatalf("mixed tray pending progress = complete %t, index %d, past %d; want true, 1, 20",
			pending.complete, pending.outputIndex, pending.nextPast)
	}
	if s.nPrefilled != 2 || s.nPast != 19 || s.iBatch != -1 || len(s.prefillTokens) != 3 {
		t.Fatalf("final staging advanced slot: prefilled %d, past %d, index %d, tokens %v",
			s.nPrefilled, s.nPast, s.iBatch, s.prefillTokens)
	}
	// Discarding an unsubmitted tray must allow the same suffix to be staged
	// again, without skipping tokens or losing its logits row.
	e.batch.clear()
	pending, ok = e.addPrefillChunk(s, 2)
	if !ok {
		t.Fatal("restaging final addPrefillChunk returned false")
	}
	if got := e.batch.entries[0]; got.token != 33 || got.positions[0] != 19 || got.output != extendedBatchOutputLogits {
		t.Fatalf("restaged suffix = token %d, position %d, output %v; want 33, 19, logits", got.token, got.positions[0], got.output)
	}
	pending.commit()
	if e.batch.len() != 1 || s.nPrefilled != 3 || s.nPast != 20 || s.iBatch != 0 || s.prefillTokens != nil {
		t.Fatalf("completed prefill = rows %d, prefilled %d, past %d, index %d, tokens %v; want 1, 3, 20, 0, nil",
			e.batch.len(), s.nPrefilled, s.nPast, s.iBatch, s.prefillTokens)
	}
	if got := e.batch.entries[0].output; got != extendedBatchOutputLogits {
		t.Fatalf("final prefill output = %d, want logits", got.value)
	}
}
