package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
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
