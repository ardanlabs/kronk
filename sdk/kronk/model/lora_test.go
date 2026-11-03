package model

import (
	"context"
	"errors"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestApplyAdaptersNoHandles(t *testing.T) {
	m := Model{}

	if err := m.applyAdapters(0); err != nil {
		t.Fatalf("applyAdapters() error = %v", err)
	}
}

func TestAdapterDraftContext(t *testing.T) {
	mtpCtx := llama.Context(42)
	tests := []struct {
		name  string
		draft drafter
		want  llama.Context
	}{
		{name: "no draft", draft: nil, want: 0},
		{name: "classic draft", draft: &classicDrafter{c: &draftCore{lctx: 11}}, want: 0},
		{name: "separate MTP assistant", draft: &sharedMTPDrafter{c: &draftCore{lctx: 12}}, want: 0},
		{name: "embedded MTP", draft: &mtpDrafter{c: &draftCore{lctx: mtpCtx}}, want: mtpCtx},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapterDraftContext(tt.draft)
			if got != tt.want {
				t.Errorf("adapterDraftContext() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCleanupGenerationRuntimeClosesStores(t *testing.T) {
	cause := errors.New("initialization failed")
	store := populatedTestSessionStore(t)
	draftStore := populatedTestSessionStore(t)
	m := Model{
		imcSessions: []*imcSession{{
			kvState:      store,
			draftKVState: draftStore,
		}},
	}

	err := m.cleanupGenerationRuntime(context.Background(), cause)
	if !errors.Is(err, cause) {
		t.Errorf("cleanupGenerationRuntime() error = %v, want initial cause", err)
	}
	for _, checkpoint := range []*imcCheckpoint{store, draftStore} {
		if checkpoint.Len() != 0 || checkpoint.Cap() != 0 || bytesTestStore(t, checkpoint) != nil {
			t.Errorf("closed checkpoint: got length %d capacity %d bytes %v, want 0/0/nil", checkpoint.Len(), checkpoint.Cap(), bytesTestStore(t, checkpoint))
		}
	}
	if m.imcSessions != nil {
		t.Errorf("imcSessions = %v, want nil", m.imcSessions)
	}
}
