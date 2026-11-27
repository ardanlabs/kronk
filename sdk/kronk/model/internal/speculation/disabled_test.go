package speculation_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/dspark"
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

func TestMTPControllerDisablesSpeculationAfterOrdinarySyncError(t *testing.T) {
	host := fakeHost{
		active:   []bool{true},
		hasRound: []bool{false},
		pending:  []bool{false},
		syncErr:  map[int]error{0: errors.New("sync")},
	}
	controller := mtp.New(&host)

	controller.TargetRowsStaged(0, speculation.TargetRange{Start: 0, Count: 1, BasePos: 7})
	controller.AfterTargetDecode(nil)

	want := []string{"track:0", "sync:0", "disable:0", "ordinary:0"}
	if !slices.Equal(host.events, want) {
		t.Errorf("events = %v, want %v", host.events, want)
	}
}

func TestMTPControllerDiscardsDraftWhenDecodeDisablesMTP(t *testing.T) {
	host := fakeHost{
		active:             []bool{true},
		canSpeculate:       []bool{true},
		candidates:         map[int][]llama.Token{0: {21}},
		disableDuringDraft: map[int]bool{0: true},
	}
	controller := mtp.New(&host)

	generation, err := controller.PlanGeneration(0)
	if err != nil {
		t.Fatalf("PlanGeneration() error = %v, want nil", err)
	}
	if len(generation.Candidates) != 0 || generation.Mode != "" {
		t.Errorf("generation = %+v, want ordinary fallback", generation)
	}
	if len(host.mtpCommits) != 0 {
		t.Errorf("MTP commits = %v, want none", host.mtpCommits)
	}
}

type dsparkHost struct {
	fakeHost
	input dspark.GenerationInput
}

func (h *dsparkHost) DSparkGenerationInput(int) dspark.GenerationInput { return h.input }

func TestDSparkGenerationPolicy(t *testing.T) {
	for _, tt := range []struct {
		name              string
		temperature       float32
		media, compatible bool
		limit             int
		reason            string
	}{
		{"greedy text", 0, false, true, 3, ""},
		{"sampled", 0.8, false, true, 3, ""},
		{"unsupported controls", 0, false, false, 3, "unsupported-sampling"},
		{"media", 0, true, true, 3, "unsupported-media"},
		{"no room for proposals", 0, false, true, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			host := dsparkHost{active: []bool{true}}
			host.input = dspark.GenerationInput{Temperature: tt.temperature, HasMedia: tt.media, SamplingCompatible: tt.compatible, MaxDraft: tt.limit,
				Generate: func(count int) (dspark.Proposal, error) {
					return dspark.Proposal{Candidates: []llama.Token{11, 23, 47}[:count]}, nil
				}}
			controller := dspark.New(&host)
			got, err := controller.PlanGeneration(0)
			want := 0
			if tt.reason == "" {
				want = tt.limit
			}
			if err != nil || len(got.Candidates) != want || controller.DisableReason(0) != tt.reason {
				t.Fatalf("generation=%+v error=%v reason=%q, want count=%d reason=%q", got, err, controller.DisableReason(0), want, tt.reason)
			}
			if err := controller.StartRequest(0); err != nil || controller.DisableReason(0) != "" {
				t.Fatalf("new request retained fallback: %v %q", err, controller.DisableReason(0))
			}
		})
	}
}

func TestDSparkGenerationFailure(t *testing.T) {
	nativeErr := errors.New("native block cleanup failed")
	host := dsparkHost{active: []bool{true}}
	host.input = dspark.GenerationInput{SamplingCompatible: true, MaxDraft: 3,
		Generate: func(int) (dspark.Proposal, error) { return dspark.Proposal{}, nativeErr }}
	got, err := dspark.New(&host).PlanGeneration(0)
	if !errors.Is(err, nativeErr) || len(got.Candidates) != 0 {
		t.Errorf("generation: got %+v/%v, want no proposals and native cause", got, err)
	}
}

func TestControllerPrefillContract(t *testing.T) {
	for _, tt := range []struct {
		name          string
		newController func(*fakeHost) speculation.Controller
		wantReason    string
	}{
		{"disabled", func(h *fakeHost) speculation.Controller { return speculation.NewDisabled(h) }, ""},
		{"classic", func(h *fakeHost) speculation.Controller { return classic.New(h) }, ""},
		{"mtp", func(h *fakeHost) speculation.Controller { return mtp.New(h) }, ""},
		{"dspark", func(h *fakeHost) speculation.Controller { return dspark.New(h) }, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			host := fakeHost{active: []bool{true}}
			controller := tt.newController(&host)
			for range 2 {
				if err := controller.StartRequest(0); err != nil {
					t.Fatal(err)
				}
				if err := controller.AfterPrefill(0, speculation.Prefill{Start: 3, Count: 2, NextPosition: 17}); err != nil {
					t.Fatal(err)
				}
				if got := controller.DisableReason(0); got != tt.wantReason {
					t.Errorf("DisableReason: got %q, want %q", got, tt.wantReason)
				}
			}
			if len(host.events) != 0 {
				t.Errorf("prefill emitted target output or started generation: %v", host.events)
			}
		})
	}
}

func TestMTPPrefillFallback(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		decodeFails, cleanupFails bool
	}{
		{"input failure", false, false},
		{"draft decode failure", true, false},
		{"input and cleanup failure", false, true},
		{"draft decode and cleanup failure", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			syncErr, cleanupErr := errors.New("sync"), errors.New("cleanup")
			host := fakeHost{active: []bool{true, true}, syncErr: map[int]error{0: syncErr}}
			if tt.decodeFails {
				delete(host.syncErr, 0)
				host.prefillInput = map[int]mtp.SyncInput{0: {
					RowCount: 1, HiddenRows: []float32{11, 13}, EmbeddingSize: 2, ChunkSize: 1,
					DecodeOwnChunk: func(int, int, []float32) error { return syncErr },
				}}
			}
			if tt.cleanupFails {
				host.disableErr = cleanupErr
			}
			controller := mtp.New(&host)
			err := controller.AfterPrefill(0, speculation.Prefill{Count: 2})
			if tt.cleanupFails && !errors.Is(err, cleanupErr) || !tt.cleanupFails && err != nil {
				t.Fatalf("AfterPrefill: got %v, cleanup-fails=%t", err, tt.cleanupFails)
			}
			if tt.cleanupFails && !errors.Is(err, syncErr) {
				t.Errorf("cleanup failure lost sync cause: %v", err)
			}
			if got := controller.DisableReason(0); got != "sync-error" {
				t.Errorf("failed request reason: got %q, want sync-error", got)
			}
			if err := controller.AfterPrefill(1, speculation.Prefill{Count: 3}); err != nil {
				t.Fatal(err)
			}
			if got := controller.DisableReason(1); got != "" {
				t.Errorf("unaffected request reason: got %q, want empty", got)
			}
			if !tt.cleanupFails {
				if err := controller.AfterPrefill(0, speculation.Prefill{Count: 1}); err != nil {
					t.Fatalf("target-only continuation: %v", err)
				}
			}
		})
	}
}

type fakeHost struct {
	active             []bool
	needsPrefill       []bool
	canSpeculate       []bool
	hasRound           []bool
	pending            []bool
	candidates         map[int][]llama.Token
	disableDuringDraft map[int]bool
	prefillErr         map[int]error
	prefillInput       map[int]mtp.SyncInput
	syncErr            map[int]error
	disabled           map[int]string
	disableErr         error
	mtpCommits         []int
	events             []string
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
			if fh.disableDuringDraft[slot] {
				fh.canSpeculate[slot] = false
				return 0, nil, false, nil
			}
			next := candidates[0]
			candidates = candidates[1:]
			return next, []float32{1}, true, nil
		},
	}, nil
}

func (fh *fakeHost) CommitMTPDraft(slot int, _ mtp.DraftResult) {
	fh.mtpCommits = append(fh.mtpCommits, slot)
}

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

func (fh *fakeHost) CommitDistributionVerify(slot int, _ []byte, _ speculation.DistributionResult) {
	fh.event("verify", slot)
}

func (fh *fakeHost) DistributionVerifyInput(slot int, _ []byte, q [][]llama.DraftCandidate) speculation.DistributionInput {
	return speculation.DistributionInput{Candidates: fh.candidates[slot], Distributions: q,
		Target: func(int) speculation.DistributionTarget {
			return speculation.DistributionTarget{Probabilities: []float32{0, 1}}
		},
		Accept: func(int, llama.Token, bool) bool { return true }}
}
func (*fakeHost) RollbackClassicDraft(int, classic.FinalizePlan) error { return nil }
func (fh *fakeHost) CompleteClassicFinalize(slot int, _ []byte, _ classic.FinalizePlan, _ bool) {
	fh.event("finalize", slot)
}

func (fh *fakeHost) MTPSyncInput(slot int, _ int) (mtp.SyncInput, error) {
	fh.event("sync", slot)
	return mtp.SyncInput{}, fh.syncErr[slot]
}

func (fh *fakeHost) CommitMTPSync(int, mtp.SyncResult) {}

func (*fakeHost) ResetDSparkSequence(int) error { return nil }

func (*fakeHost) WithDSparkTarget(consume func(dspark.SyncInput) []dspark.SyncResult) {
	consume(dspark.SyncInput{})
}

func (*fakeHost) DSparkGenerationInput(int) dspark.GenerationInput {
	return dspark.GenerationInput{}
}

func (fh *fakeHost) MatchVerifyInput(slot int, buf []byte) speculation.MatchInput {
	input, _ := fh.MTPVerifyInput(slot, buf)
	return input
}

func (fh *fakeHost) MTPDisableReason(slot int) string {
	return fh.disabled[slot]
}

func (fh *fakeHost) MTPPrefillInput(slot int, _ speculation.Prefill) (mtp.SyncInput, error) {
	if fh.disabled[slot] != "" {
		return mtp.SyncInput{}, nil
	}
	return fh.prefillInput[slot], fh.syncErr[slot]
}

func (fh *fakeHost) MTPVerifyInput(slot int, _ []byte) (mtp.VerifyInput, error) {
	return mtp.VerifyInput{
		Candidates: fh.candidates[slot],
		Sample:     func(int) llama.Token { return 0 },
		Accept:     func(int, llama.Token) bool { return true },
	}, nil
}

func (fh *fakeHost) CommitMatchVerify(slot int, _ []byte, _ mtp.VerifyResult) {
	fh.event("verify", slot)
}

func (fh *fakeHost) SpeculativeFinalizePlan(int) (mtp.FinalizePlan, bool) {
	return mtp.FinalizePlan{}, true
}

func (fh *fakeHost) RollbackSpeculativeTarget(int, mtp.FinalizePlan) (bool, error) { return false, nil }
func (fh *fakeHost) RollbackMTPDraft(int, mtp.FinalizePlan) error                  { return nil }
func (fh *fakeHost) DisableMTP(slot int, reason string, _ int) error {
	fh.event("disable", slot)
	if fh.disabled == nil {
		fh.disabled = make(map[int]string)
	}
	fh.disabled[slot] = reason
	return fh.disableErr
}
func (fh *fakeHost) CompleteSpeculativeFinalize(slot int, _ []byte, _ mtp.FinalizePlan, _ bool) {
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
