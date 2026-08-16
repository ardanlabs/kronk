package speculation_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestDisabledController(t *testing.T) {
	host := fakeHost{active: []bool{true, false, true}}
	controller := speculation.NewDisabled(&host)

	if controller.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if got := controller.Mode(); got != speculation.ModeDisabled {
		t.Errorf("Mode() = %q, want %q", got, speculation.ModeDisabled)
	}

	controller.BeginBatch()
	controller.Prepare()
	controller.AfterTargetDecode(nil)

	want := []string{"ordinary:0", "ordinary:2"}
	if !slices.Equal(host.events, want) {
		t.Errorf("events = %v, want %v", host.events, want)
	}
}

func TestClassicController(t *testing.T) {
	host := fakeHost{
		active:       []bool{true, true, true},
		needsPrefill: []bool{true, true, false},
		canSpeculate: []bool{true, false, false},
		hasRound:     []bool{true, false, false},
		pending:      []bool{true, false, false},
		candidates:   map[int][]llama.Token{0: {11, 12}},
		prefillErr:   map[int]error{1: errors.New("prefill")},
	}
	controller := classic.New(&host)

	controller.Prepare()
	generation, err := controller.PlanGeneration(0)
	if err != nil {
		t.Fatalf("PlanGeneration() error = %v, want nil", err)
	}
	if want := []llama.Token{11, 12}; !slices.Equal(generation.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", generation.Candidates, want)
	}
	if generation.Mode != "classic" {
		t.Errorf("Mode = %q, want classic", generation.Mode)
	}

	targetRange := speculation.TargetRange{Start: 3, Count: 3, BasePos: 8}
	if err := controller.CommitGeneration(0, generation.Candidates, targetRange); err != nil {
		t.Fatalf("CommitGeneration() error = %v, want nil", err)
	}
	controller.AfterTargetDecode(nil)

	wantEvents := []string{
		"prefill:0", "prefill:1", "fail:1", "generate:0", "commit:0", "ordinary:1", "ordinary:2", "verify:0", "finalize:0",
	}
	if !slices.Equal(host.events, wantEvents) {
		t.Errorf("events = %v, want %v", host.events, wantEvents)
	}
}

func TestMTPController(t *testing.T) {
	host := fakeHost{
		active:       []bool{true, true},
		canSpeculate: []bool{true, true},
		hasRound:     []bool{false, true},
		pending:      []bool{false, true},
		candidates:   map[int][]llama.Token{1: {21, 22}},
	}
	controller := mtp.New(&host)

	controller.BeginBatch()
	generation, err := controller.PlanGeneration(1)
	if err != nil {
		t.Fatalf("PlanGeneration() error = %v, want nil", err)
	}
	targetRange := speculation.TargetRange{Start: 4, Count: 3, BasePos: 9}
	if err := controller.CommitGeneration(1, generation.Candidates, targetRange); err != nil {
		t.Fatalf("CommitGeneration() error = %v, want nil", err)
	}
	controller.TargetRowsStaged(0, speculation.TargetRange{Start: 0, Count: 1, BasePos: 7})
	controller.AfterTargetDecode(nil)

	wantEvents := []string{
		"reset:0", "reset:1", "generate:1", "commit:1", "track:1", "track:0", "sync:0", "ordinary:0", "verify:1", "sync:1", "finalize:1",
	}
	if !slices.Equal(host.events, wantEvents) {
		t.Errorf("events = %v, want %v", host.events, wantEvents)
	}
}

type fakeHost struct {
	active       []bool
	needsPrefill []bool
	canSpeculate []bool
	hasRound     []bool
	pending      []bool
	candidates   map[int][]llama.Token
	prefillErr   map[int]error
	events       []string
}

func (fh *fakeHost) SlotCount() int                      { return len(fh.active) }
func (fh *fakeHost) SlotActive(slot int) bool            { return fh.active[slot] }
func (fh *fakeHost) SlotNeedsDraftPrefill(slot int) bool { return fh.needsPrefill[slot] }
func (fh *fakeHost) CanSpeculate(slot int) bool          { return fh.canSpeculate[slot] }
func (fh *fakeHost) HasSpeculativeRound(slot int) bool   { return fh.hasRound[slot] }
func (fh *fakeHost) HasPendingFinalize(slot int) bool    { return fh.pending[slot] }
func (fh *fakeHost) ProcessOrdinary(slot int, _ []byte)  { fh.event("ordinary", slot) }
func (fh *fakeHost) ResetTargetRange(slot int)           { fh.event("reset", slot) }
func (fh *fakeHost) TrackTargetRange(slot int, _ speculation.TargetRange) {
	fh.event("track", slot)
}

func (fh *fakeHost) PrefillDraft(slot int) error {
	fh.event("prefill", slot)
	return fh.prefillErr[slot]
}

func (fh *fakeHost) ClassicGenerationInput(slot int) (classic.GenerationInput, error) {
	fh.event("generate", slot)
	state := classic.SlotState{AcceptanceEMA: 1}
	return classic.GenerationInput{State: &state, MaxDraft: len(fh.candidates[slot]), Generate: func(int) (classic.GenerationResult, error) {
		return classic.GenerationResult{Candidates: fh.candidates[slot]}, nil
	}}, nil
}

func (*fakeHost) CommitClassicDraft(int, classic.GenerationResult) {}

func (fh *fakeHost) MTPDraftInput(slot int) (mtp.DraftInput, error) {
	fh.event("generate", slot)
	candidates := fh.candidates[slot]
	return mtp.DraftInput{
		Token:  1,
		Hidden: []float32{1},
		Count:  len(candidates),
		IsEOG:  func(llama.Token) bool { return false },
		DecodeStep: func(_ llama.Token, _ llama.Pos, _ []float32) (llama.Token, []float32, bool, error) {
			next := candidates[0]
			candidates = candidates[1:]
			return next, []float32{1}, true, nil
		},
	}, nil
}

func (fh *fakeHost) CommitMTPDraft(int, mtp.DraftResult) {}

func (fh *fakeHost) CommitSpeculative(slot int, _ []llama.Token, _ speculation.TargetRange) error {
	fh.event("commit", slot)
	return nil
}

func (fh *fakeHost) ClassicVerifyInput(slot int, _ []byte) (classic.VerifyInput, error) {
	return classic.VerifyInput{State: &classic.SlotState{AcceptanceEMA: 1}, Candidates: fh.candidates[slot], Greedy: true,
		Target: func(index int) classic.Target {
			if index < len(fh.candidates[slot]) {
				return classic.Target{Token: fh.candidates[slot][index]}
			}
			return classic.Target{Token: 99}
		},
		Accept: func(int, llama.Token, bool) bool { return true }}, nil
}

func (fh *fakeHost) CommitClassicVerify(slot int, _ []byte, _ classic.VerifyResult) {
	fh.event("verify", slot)
}
func (*fakeHost) ClassicFinalizePlan(int) (classic.FinalizePlan, bool) {
	return classic.FinalizePlan{}, true
}
func (*fakeHost) RollbackClassicTarget(int, classic.FinalizePlan) (bool, error) { return false, nil }
func (*fakeHost) RollbackClassicDraft(int, classic.FinalizePlan) error          { return nil }
func (fh *fakeHost) CompleteClassicFinalize(slot int, _ []byte, _ classic.FinalizePlan, _ bool) {
	fh.event("finalize", slot)
}

func (fh *fakeHost) MTPSyncInput(slot int, _ int) (mtp.SyncInput, error) {
	fh.event("sync", slot)
	return mtp.SyncInput{}, nil
}

func (fh *fakeHost) CommitMTPSync(int, mtp.SyncResult) {}

func (fh *fakeHost) MTPVerifyInput(slot int, _ []byte) (mtp.VerifyInput, error) {
	return mtp.VerifyInput{
		Candidates: fh.candidates[slot],
		Sample:     func(int) llama.Token { return 0 },
		Accept:     func(int, llama.Token) bool { return true },
	}, nil
}

func (fh *fakeHost) CommitMTPVerify(slot int, _ []byte, _ mtp.VerifyResult) {
	fh.event("verify", slot)
}

func (fh *fakeHost) MTPFinalizePlan(int) (mtp.FinalizePlan, bool) {
	return mtp.FinalizePlan{}, true
}

func (fh *fakeHost) RollbackMTPTarget(int, mtp.FinalizePlan) (bool, error) { return false, nil }
func (fh *fakeHost) RollbackMTPDraft(int, mtp.FinalizePlan) error          { return nil }
func (fh *fakeHost) DisableMTP(int, string, int) error                     { return nil }
func (fh *fakeHost) CompleteMTPFinalize(slot int, _ []byte, _ mtp.FinalizePlan, _ bool) {
	fh.event("finalize", slot)
}

func (fh *fakeHost) Fail(slot int, _ error) {
	fh.event("fail", slot)
}

func (fh *fakeHost) event(name string, slot int) {
	fh.events = append(fh.events, fmt.Sprintf("%s:%d", name, slot))
}

var _ speculation.Host = (*fakeHost)(nil)
var _ classic.Host = (*fakeHost)(nil)
