package kronk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

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
	)
	tuned := model.NewConfig(
		model.WithContextWindow(8192),
		model.WithNSeqMax(8),
		model.WithCacheTypeK(model.GGMLTypeAuto),
		model.WithCacheTypeV(model.GGMLTypeAuto),
	)

	restoreAutoTunedSizing(&tuned, recommended)

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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireAdmission: got error %v, want %v", err, context.DeadlineExceeded)
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
