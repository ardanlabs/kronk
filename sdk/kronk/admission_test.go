package kronk

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestEmbedOrRerankAdmissionCapacity(t *testing.T) {
	tests := []struct {
		name       string
		nSeqMax    int
		queueDepth int
		want       int
	}{
		{"minimum", 0, 2, 1},
		{"single sequence", 1, 4, 1},
		{"multiple sequences", 4, 7, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := model.NewConfig(model.WithNSeqMax(tt.nSeqMax), model.WithQueueDepth(tt.queueDepth))
			if got := embedOrRerankAdmissionCapacity(cfg); got != tt.want {
				t.Errorf("embedOrRerankAdmissionCapacity: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGenerationAdmissionCapacity(t *testing.T) {
	tests := []struct {
		name       string
		nSlots     int
		queueDepth int
		want       int
	}{
		{"resolved default", 1, 2, 2},
		{"one", 3, 1, 3},
		{"two", 4, 2, 8},
		{"three", 2, 3, 6},
		{"four", 5, 4, 20},
		{"seven", 2, 7, 14},
		{"minimum slot", 0, 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := model.NewConfig(model.WithNSeqMax(tt.nSlots), model.WithQueueDepth(tt.queueDepth))
			if got := generationAdmissionCapacity(cfg); got != tt.want {
				t.Errorf("generationAdmissionCapacity: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRestoreAutoTunedSizing(t *testing.T) {
	recommended := model.NewConfig(
		model.WithContextWindow(131072),
		model.WithNSeqMax(2),
		model.WithCacheTypeK(model.GGMLTypeQ8_0),
		model.WithCacheTypeV(model.GGMLTypeQ8_0),
		model.WithFlashAttention(model.FlashAttentionEnabled),
		model.WithSplitMode(model.SplitModeNone),
		model.WithNGpuLayers(0),
	)
	tuned := model.NewConfig(
		model.WithContextWindow(8192),
		model.WithNSeqMax(8),
		model.WithCacheTypeK(model.GGMLTypeAuto),
		model.WithCacheTypeV(model.GGMLTypeAuto),
		model.WithFlashAttention(model.FlashAttentionDisabled),
		model.WithSplitMode(model.SplitModeRow),
		model.WithNGpuLayers(17),
	)

	restoreAutoTunedFields(&tuned, recommended)

	if tuned.ContextWindow() != recommended.ContextWindow() {
		t.Errorf("ContextWindow: got %d, want %d", tuned.ContextWindow(), recommended.ContextWindow())
	}
	if tuned.NSeqMax() != recommended.NSeqMax() {
		t.Errorf("NSeqMax: got %d, want %d", tuned.NSeqMax(), recommended.NSeqMax())
	}
	if tuned.CacheTypeK != recommended.CacheTypeK {
		t.Errorf("CacheTypeK: got %s, want %s", tuned.CacheTypeK, recommended.CacheTypeK)
	}
	if tuned.CacheTypeV != recommended.CacheTypeV {
		t.Errorf("CacheTypeV: got %s, want %s", tuned.CacheTypeV, recommended.CacheTypeV)
	}
	if tuned.FlashAttention() != recommended.FlashAttention() {
		t.Errorf("FlashAttention: got %s, want %s", tuned.FlashAttention(), recommended.FlashAttention())
	}
	if tuned.PtrSplitMode == nil || *tuned.PtrSplitMode != *recommended.PtrSplitMode {
		t.Errorf("PtrSplitMode: got %v, want %v", tuned.PtrSplitMode, recommended.PtrSplitMode)
	}
	if tuned.PtrNGpuLayers == nil || *tuned.PtrNGpuLayers != *recommended.PtrNGpuLayers {
		t.Errorf("PtrNGpuLayers: got %v, want %v", tuned.PtrNGpuLayers, recommended.PtrNGpuLayers)
	}
}

func TestAutoTuneLogValues(t *testing.T) {
	contextWindow := 131072
	flashAttention := model.FlashAttentionEnabled
	nGpuLayers := 0
	cfg := model.NewConfig(
		model.WithContextWindow(contextWindow),
		model.WithNSeqMax(2),
		model.WithCacheTypeK(model.GGMLTypeQ8_0),
		model.WithCacheTypeV(model.GGMLTypeQ8_0),
		model.WithFlashAttention(flashAttention),
		model.WithSplitMode(model.SplitModeNone),
		model.WithNGpuLayers(nGpuLayers),
	)
	constraints := models.ModelConfig{
		PtrContextWindow: &contextWindow,
		FlashAttention:   &flashAttention,
		PtrNGpuLayers:    &nGpuLayers,
	}

	selected, constrained := autoTuneLogValues(cfg, constraints)
	wantSelected := []string{
		"nseq_max=2",
		"cache_type_k=q8_0",
		"cache_type_v=q8_0",
		"split_mode=none",
	}
	wantConstrained := []string{
		"context_window=131072",
		"flash_attention=enabled",
		"ngpu_layers=0",
	}

	if !slices.Equal(selected, wantSelected) {
		t.Errorf("selected: got %v, want %v", selected, wantSelected)
	}
	if !slices.Equal(constrained, wantConstrained) {
		t.Errorf("constrained: got %v, want %v", constrained, wantConstrained)
	}
}

func TestAcquireAdmissionWait(t *testing.T) {
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}
	krn.admissionCh <- struct{}{}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := krn.acquireAdmission(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireAdmission: got error %v, want %v", err, context.Canceled)
	}

	if active := krn.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireAdmissionConfiguredTimeout(t *testing.T) {
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Millisecond)),
		admissionCh: make(chan struct{}, 1),
	}
	krn.admissionCh <- struct{}{}

	_, err := krn.acquireAdmission(t.Context())
	if !errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("acquireAdmission: got error %v, want %v", err, ErrAdmissionTimeout)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireAdmission: got context deadline for admission timeout: %v", err)
	}

	if active := krn.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireAdmissionCallerDeadline(t *testing.T) {
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}
	krn.admissionCh <- struct{}{}

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()

	_, err := krn.acquireAdmission(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireAdmission: got error %v, want %v", err, context.DeadlineExceeded)
	}
	if errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("acquireAdmission: got admission timeout for caller deadline: %v", err)
	}

	if active := krn.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireAdmissionTimeoutIsLocal(t *testing.T) {
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}

	ctx := t.Context()
	if _, err := krn.acquireAdmission(ctx); err != nil {
		t.Fatalf("acquireAdmission() error = %v", err)
	}
	defer krn.releaseAdmission()

	if err := ctx.Err(); err != nil {
		t.Errorf("caller context after admission = %v, want nil", err)
	}
}
