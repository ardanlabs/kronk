package model

import (
	"context"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestSlotResetClearsPerRequestLifecycleState(t *testing.T) {
	s := slot{
		startTime:          time.Now(),
		specSnapshot:       []byte{1},
		pendingH:           []float32{1},
		mtpDraftH:          []float32{2},
		reusedPromptTokens: 42,
		specAccEMA:         0.17,
		mtpProbeTick:       31,
	}

	s.reset()

	if !s.startTime.IsZero() {
		t.Errorf("startTime = %v, want zero time", s.startTime)
	}
	if len(s.specSnapshot) != 0 {
		t.Errorf("len(specSnapshot) = %d, want 0", len(s.specSnapshot))
	}
	if len(s.pendingH) != 0 {
		t.Errorf("len(pendingH) = %d, want 0", len(s.pendingH))
	}
	if len(s.mtpDraftH) != 0 {
		t.Errorf("len(mtpDraftH) = %d, want 0", len(s.mtpDraftH))
	}
	if s.reusedPromptTokens != 0 {
		t.Errorf("reusedPromptTokens = %d, want 0", s.reusedPromptTokens)
	}
	if s.specAccEMA != 1.0 {
		t.Errorf("specAccEMA = %f, want 1.0", s.specAccEMA)
	}
	if s.mtpProbeTick != 0 {
		t.Errorf("mtpProbeTick = %d, want 0", s.mtpProbeTick)
	}
}

func TestChooseNDraftPolicy(t *testing.T) {
	tests := []struct {
		name     string
		ema      float64
		probe    int
		want     int
		wantTick int
	}{
		{name: "full draft", ema: 1.0, want: 2},
		{name: "upper boundary", ema: 0.50, want: 2},
		{name: "reduced draft", ema: 0.499, probe: 7, want: 1},
		{name: "lower boundary", ema: 0.30, want: 1},
		{name: "fully throttled", ema: 0.299, want: 0, wantTick: 1},
		{name: "recovery probe", ema: 0.299, probe: mtpProbeInterval - 1, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := slot{specAccEMA: tt.ema, mtpProbeTick: tt.probe}
			if got := chooseNDraft(&s, 2); got != tt.want {
				t.Errorf("chooseNDraft() = %d, want %d", got, tt.want)
			}
			if s.mtpProbeTick != tt.wantTick {
				t.Errorf("mtpProbeTick = %d, want %d", s.mtpProbeTick, tt.wantTick)
			}
		})
	}
}

func TestAdaptiveDraftStateDoesNotCrossRequestBoundary(t *testing.T) {
	s := slot{specAccEMA: 0.29, mtpProbeTick: 7}
	if got := chooseNDraft(&s, 2); got != 0 {
		t.Fatalf("chooseNDraft() before reset = %d, want 0", got)
	}

	s.reset()

	if got := chooseNDraft(&s, 2); got != 2 {
		t.Errorf("chooseNDraft() after reset = %d, want 2", got)
	}
	if s.mtpProbeTick != 0 {
		t.Errorf("mtpProbeTick = %d, want 0", s.mtpProbeTick)
	}
}

func TestNeedsTargetSpecSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		modelType     ModelType
		rollbackDepth uint32
		draftCount    int
		want          bool
	}{
		{"dense target", ModelTypeDense, 0, 2, false},
		{"hybrid native rollback covers draft", ModelTypeHybrid, 2, 2, false},
		{"hybrid native rollback is insufficient", ModelTypeHybrid, 1, 2, true},
		{"hybrid rollback unavailable", ModelTypeHybrid, 0, 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsTargetSpecSnapshot(tt.modelType, tt.rollbackDepth, tt.draftCount)
			if got != tt.want {
				t.Errorf("needsTargetSpecSnapshot() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMaxDraftForSlot(t *testing.T) {
	e := batchEngine{model: &Model{cfg: Config{PtrContextWindow: new(10)}}}

	tests := []struct {
		name             string
		nPast            llama.Pos
		configured       int
		maxTokens        int
		reasonTokens     int
		completionTokens int
		want             int
	}{
		{name: "full draft fits", nPast: 5, configured: 3, maxTokens: 100, want: 3},
		{name: "draft capped at two-cell context reserve", nPast: 7, configured: 3, maxTokens: 100, want: 1},
		{name: "context reserve exhausted", nPast: 8, configured: 3, maxTokens: 100, want: 0},
		{name: "window exhausted", nPast: 10, configured: 3, maxTokens: 100, want: 0},
		{name: "draft capped at remaining budget", nPast: 1, configured: 3, maxTokens: 5, completionTokens: 3, want: 1},
		{name: "reasoning and completion share budget", nPast: 1, configured: 3, maxTokens: 5, reasonTokens: 2, completionTokens: 2, want: 0},
		{name: "last output token reserved for target", nPast: 1, configured: 3, maxTokens: 5, completionTokens: 4, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := slot{
				nPast:            tt.nPast,
				reasonTokens:     tt.reasonTokens,
				completionTokens: tt.completionTokens,
				job:              &chatJob{params: Params{MaxTokens: tt.maxTokens}},
			}
			got := e.maxDraftForSlot(&s, tt.configured)
			if got != tt.want {
				t.Errorf("maxDraftForSlot() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestContextOutputBudget(t *testing.T) {
	tests := []struct {
		name               string
		promptTokens       int
		requestedMaxTokens int
		contextWindow      int
		wantMaxTokens      int
		wantOK             bool
	}{
		{"requested budget fits", 4096, 2048, 8192, 2048, true},
		{"requested budget clamped", 7000, 2048, 8192, 1192, true},
		{"one token remains", 8191, 2048, 8192, 1, true},
		{"prompt fills window", 8192, 2048, 8192, 0, false},
		{"prompt exceeds window", 8193, 2048, 8192, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMaxTokens, gotOK := contextOutputBudget(tt.promptTokens, tt.requestedMaxTokens, tt.contextWindow)
			if gotMaxTokens != tt.wantMaxTokens || gotOK != tt.wantOK {
				t.Errorf("contextOutputBudget() = (%d, %t), want (%d, %t)", gotMaxTokens, gotOK, tt.wantMaxTokens, tt.wantOK)
			}
		})
	}
}

func TestApplyContextTokenBudgetClampsRequest(t *testing.T) {
	contextWindow := 8192
	logged := false
	e := batchEngine{
		model: &Model{
			cfg: Config{PtrContextWindow: &contextWindow},
			log: func(_ context.Context, msg string, args ...any) {
				if msg == "start-slot" {
					logged = true
				}
			},
		},
	}
	s := slot{
		nPrompt: 7000,
		job: &chatJob{
			ctx:    context.Background(),
			params: Params{MaxTokens: 2048},
		},
	}

	if !e.applyContextTokenBudget(&s, "start-slot") {
		t.Fatal("applyContextTokenBudget() = false, want true")
	}
	if s.job.params.MaxTokens != 1192 {
		t.Errorf("MaxTokens = %d, want 1192", s.job.params.MaxTokens)
	}
	if !logged {
		t.Error("clamp log not emitted")
	}
}

func TestValidMTPDraftState(t *testing.T) {
	tests := []struct {
		name          string
		actualBytes   uint64
		expectedBytes uint64
		pendingH      []float32
		nEmbd         int
		want          bool
	}{
		{"complete state", 100, 100, []float32{1, 2}, 2, true},
		{"empty state", 0, 0, []float32{}, 0, false},
		{"partial bytes", 99, 100, []float32{1, 2}, 2, false},
		{"missing hidden state", 100, 100, nil, 2, false},
		{"wrong hidden width", 100, 100, []float32{1}, 2, false},
		{"invalid embedding width", 100, 100, nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validMTPDraftState(tt.actualBytes, tt.expectedBytes, tt.pendingH, tt.nEmbd)
			if got != tt.want {
				t.Errorf("validMTPDraftState() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPrepareMTPDraftHiddenDoesNotAliasPendingH(t *testing.T) {
	s := slot{pendingH: []float32{1, 2, 3}}

	draftH := prepareMTPDraftHidden(&s, len(s.pendingH))
	draftH[0] = 9

	if s.pendingH[0] != 1 {
		t.Errorf("pendingH[0] = %v, want 1", s.pendingH[0])
	}
}

func TestSpecAcceptedNPast(t *testing.T) {
	tests := []struct {
		name     string
		basePast llama.Pos
		accepted int
		want     llama.Pos
	}{
		{"no accepted drafts", 100, 0, 101},
		{"first draft accepted", 100, 1, 102},
		{"both drafts accepted", 100, 2, 103},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := specAcceptedNPast(tt.basePast, tt.accepted); got != tt.want {
				t.Errorf("specAcceptedNPast(%d, %d) = %d, want %d", tt.basePast, tt.accepted, got, tt.want)
			}
		})
	}
}

func TestClassicDraftKeepPosition(t *testing.T) {
	tests := []struct {
		name      string
		startPast llama.Pos
		endPast   llama.Pos
		accepted  int
		want      llama.Pos
	}{
		{"partial acceptance", 100, 104, 1, 102},
		{"full acceptance", 100, 104, 4, 104},
		{"EOG-truncated full acceptance", 100, 103, 2, 103},
		{"EOG-truncated partial acceptance", 100, 103, 1, 102},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classicDraftKeepPosition(tt.startPast, tt.endPast, tt.accepted)
			if got != tt.want {
				t.Errorf("classicDraftKeepPosition() = %d, want %d", got, tt.want)
			}
		})
	}
}
