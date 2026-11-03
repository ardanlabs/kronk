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
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: func(context.Context, string, ...any) {},
		imcSessions: []*imcSession{{
			id:      0,
			kvState: &imcCheckpoint{},
		}},
	}
	publishTestIMCPrefixes(&m)

	result := m.processIMCTokenPlan(ctx, D{"messages": []D{{"role": "user", "content": "hello"}}}, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("processIMCTokenPlan() error = %v, want %v", result.err, wantErr)
	}
	if m.imcSessions[0].reserved {
		t.Error("session remained reserved after reset failure")
	}
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

func TestIMCReclaimsOversizedBuffersOnlyOnShorterRebuild(t *testing.T) {
	for _, media := range []bool{false, true} {
		name := "text"
		if media {
			name = "media"
		}
		t.Run(name, func(t *testing.T) {
			for _, tt := range []struct {
				name         string
				messages     []int
				size         int
				incoming     int
				cancel       bool
				finalSize    int
				draft        bool
				missingDraft bool
				reassign     bool
				wantRelease  bool
			}{
				{name: "three mature publications", messages: []int{20, 22, 24}, size: 600, incoming: 2, wantRelease: true},
				{name: "draft qualifies independently", messages: []int{20, 22, 24}, size: 600, incoming: 2, draft: true, wantRelease: true},
				{name: "missing draft publication", messages: []int{20, 22, 24}, size: 600, incoming: 2, draft: true, missingDraft: true},
				{name: "rounded quarter reduction", messages: []int{20, 22, 24}, size: 675, incoming: 2, wantRelease: true},
				{name: "capacity slack alone", messages: []int{20, 22, 24}, size: 676, incoming: 2},
				{name: "immature history", messages: []int{2, 4, 6}, size: 100, incoming: 1},
				{name: "repeated publication", messages: []int{20, 20, 20}, size: 600, incoming: 2},
				{name: "two observations", messages: []int{20, 22}, size: 600, incoming: 2},
				{name: "not below baseline", messages: []int{20, 22, 24}, size: 600, incoming: 20},
				{name: "not below outgoing", messages: []int{20, 22, 24, 2}, size: 600, incoming: 2},
				{name: "canceled reset", messages: []int{20, 22, 24}, size: 600, incoming: 2, cancel: true},
				{name: "new allocation", messages: []int{20, 22, 24}, size: 600, incoming: 2, finalSize: 1300},
				{name: "higher demand clears qualification", messages: []int{20, 22, 24}, size: 600, incoming: 2, finalSize: 800},
				{name: "different replacement histories", messages: []int{20, 22, 24}, size: 600, incoming: 2, reassign: true},
			} {
				t.Run(tt.name, func(t *testing.T) {
					ctx := context.Background()
					s := &imcSession{id: 0, cache: &imcPrefix{}, kvState: &imcCheckpoint{}, draftKVState: &imcCheckpoint{}}
					m := Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: func(context.Context, string, ...any) {}, imcSessions: []*imcSession{s}}
					publish := func(messages, targetBytes, draftBytes int) {
						t.Helper()
						s.reserved = true
						m.imcCommitSession(ctx, s, "history", 1, messages, []llama.Token{7}, false, nil, "render")
						for store, size := range map[*imcCheckpoint]int{s.kvState: targetBytes, s.draftKVState: draftBytes} {
							if size == 0 {
								continue
							}
							buf := prepareTestStore(t, store, size)
							for i := range buf {
								buf[i] = 42
							}
							commitTestStore(t, store, size)
						}
						s.pendingH = []float32{1}
						if draftBytes == 0 {
							s.pendingH = nil
						}
						m.imcPublishSession(s)
					}
					// Separate valid bytes from capacity so the test rejects a
					// policy that mistakes allocation headroom for lower demand.
					prepareTestStore(t, s.kvState, 1024)
					prepareTestStore(t, s.draftKVState, 1024)
					publish(20, 901, 901)
					s.reserved = true
					if err := resetSessionForRequest(ctx, s, 2); err != nil {
						t.Fatal(err)
					}
					resetSessionMetadata(s)
					for i, messages := range tt.messages {
						if tt.reassign && i == 2 {
							s.reserved = true
							if err := resetSessionForRequest(ctx, s, 2); err != nil {
								t.Fatal(err)
							}
							resetSessionMetadata(s)
						}
						if tt.draft {
							size := tt.size
							if tt.missingDraft && i == 2 {
								size = 0
							}
							publish(messages, 800, size)
						} else {
							publish(messages, tt.size, 800)
						}
					}
					if tt.finalSize > 0 {
						publish(26, tt.finalSize, 800)
					}
					before := s.kvState.Cap()
					if before < 1024 || s.kvState.Len() == 0 || bytesTestStore(t, s.kvState)[0] != 42 {
						t.Fatal("publication reclaimed or corrupted a valid checkpoint")
					}
					if tt.cancel {
						var cancel context.CancelFunc
						ctx, cancel = context.WithCancel(ctx)
						cancel()
					}
					d := D{"messages": make([]D, tt.incoming)}
					var result cacheResult
					if media {
						stable := mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true})
						actual := mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true}, promptUnit{token: 9})
						result = m.processIMCMediaPlans(ctx, d, d, actual, stable, []llama.Token{9}, nil, time.Now())
					} else {
						result = m.processIMCTokenPlan(ctx, d, []llama.Token{1, 2, 9}, []llama.Token{1, 2}, nil, time.Now())
					}
					if tt.cancel {
						if !errors.Is(result.err, context.Canceled) {
							t.Fatalf("reset error: got %v, want canceled", result.err)
						}
					} else if result.err != nil || result.imcMatchKind != "rebuild" || !s.reserved {
						t.Fatalf("rebuild: got %q reserved %t error %v", result.imcMatchKind, s.reserved, result.err)
					}
					wantCap := before
					wantDraftCap := 1024
					if tt.wantRelease && tt.draft {
						wantDraftCap = 0
					} else if tt.wantRelease {
						wantCap = 0
					}
					if got := s.kvState.Cap(); got != wantCap {
						t.Errorf("target capacity: got %d, want %d", got, wantCap)
					}
					if got := s.draftKVState.Cap(); got != wantDraftCap {
						t.Errorf("draft capacity: got %d, want %d", got, wantDraftCap)
					}
					if got, want := s.snapshotBytes, imcSnapshotBytes(s.kvState, s.draftKVState, s.pendingH); got != want {
						t.Errorf("snapshot accounting: got %d, want %d", got, want)
					}
					if tt.wantRelease {
						publish(2, 120, 100)
						store, size := s.kvState, 120
						if tt.draft {
							store, size = s.draftKVState, 100
						}
						if store.Cap() != size || store.Len() != size || bytesTestStore(t, store)[0] != 42 {
							t.Fatal("released checkpoint did not lazily allocate valid replacement state")
						}
					}
				})
			}
		})
	}
}
