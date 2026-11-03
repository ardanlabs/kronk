package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestProcessIMCTokenPlanReturnsResetError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wantErr := context.Canceled
	log, events := capturePlanLogs(t)
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: log,
		imcSessions: []*imcSession{{
			id:      0,
			kvState: &imcCheckpoint{},
		}},
	}
	bindTestTokenCaches(&m)

	result := m.processIMCTokenPlan(ctx, D{"messages": []D{{"role": "user", "content": "hello"}}}, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("processIMCTokenPlan() error = %v, want %v", result.err, wantErr)
	}
	if m.imcSessions[0].reserved {
		t.Error("session remained reserved after reset failure")
	}
	assertPlanLogFields(t, events["plan-reset-failed"], map[string]any{
		"selection": "empty", "previous_cached_tokens": 0,
		"token_cache_id": m.imcSessions[0].cache.id, "imc_cache_entry": 0,
	})
}

func TestPrepareSessionSnapshotRejectsSizeOverflow(t *testing.T) {
	store := &imcCheckpoint{}

	buf, err := prepareSessionSnapshot(context.Background(), store, ^uint64(0))
	if err == nil {
		t.Fatal("prepareSessionSnapshot() error = nil, want non-nil")
	}
	if buf != nil {
		t.Errorf("prepareSessionSnapshot() buffer length = %d, want nil", len(buf))
	}
}
