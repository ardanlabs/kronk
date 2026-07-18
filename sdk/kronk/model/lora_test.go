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
	closeErr := errors.New("close failed")
	store := &testCloseStore{}
	draftStore := &testCloseStore{err: closeErr}
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
	if !errors.Is(err, closeErr) {
		t.Errorf("cleanupGenerationRuntime() error = %v, want close error", err)
	}
	if store.closes != 1 {
		t.Errorf("store closes = %d, want 1", store.closes)
	}
	if draftStore.closes != 1 {
		t.Errorf("draft store closes = %d, want 1", draftStore.closes)
	}
	if m.imcSessions != nil {
		t.Errorf("imcSessions = %v, want nil", m.imcSessions)
	}
}

type testCloseStore struct {
	closes int
	err    error
}

func (s *testCloseStore) Len() int           { return 0 }
func (s *testCloseStore) Cap() int           { return 0 }
func (s *testCloseStore) Bytes() []byte      { return nil }
func (s *testCloseStore) Prepare(int) []byte { return nil }
func (s *testCloseStore) Commit(int)         {}
func (s *testCloseStore) Reset()             {}
func (s *testCloseStore) Close() error {
	s.closes++
	return s.err
}
