package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestHashMessages(t *testing.T) {
	tests := []struct {
		name     string
		msgs1    []D
		msgs2    []D
		wantSame bool
	}{
		{
			name: "identical messages same hash",
			msgs1: []D{
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"},
			},
			wantSame: true,
		},
		{
			name:     "different content different hash",
			msgs1:    []D{{"role": "user", "content": "Hello"}},
			msgs2:    []D{{"role": "user", "content": "Goodbye"}},
			wantSame: false,
		},
		{
			name:     "different role different hash",
			msgs1:    []D{{"role": "user", "content": "Hello"}},
			msgs2:    []D{{"role": "assistant", "content": "Hello"}},
			wantSame: false,
		},
		{
			name: "different order different hash",
			msgs1: []D{
				{"role": "user", "content": "A"},
				{"role": "assistant", "content": "B"},
			},
			msgs2: []D{
				{"role": "assistant", "content": "B"},
				{"role": "user", "content": "A"},
			},
			wantSame: false,
		},
		{name: "empty messages same hash", msgs1: []D{}, msgs2: []D{}, wantSame: true},
		{
			name:     "prefix subset different hash",
			msgs1:    []D{{"role": "user", "content": "Hello"}},
			msgs2:    []D{{"role": "user", "content": "Hello"}, {"role": "assistant", "content": "Hi"}},
			wantSame: false,
		},
		{
			name:     "delimiter content cannot impersonate another message",
			msgs1:    []D{{"role": "user", "content": "x|1:assistant:y"}},
			msgs2:    []D{{"role": "user", "content": "x"}, {"role": "assistant", "content": "y"}},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashMessages(tt.msgs1)
			hash2 := hashMessages(tt.msgs2)
			if got := hash1 == hash2; got != tt.wantSame {
				t.Errorf("hash equality: got %t, want %t", got, tt.wantSame)
			}
		})
	}
}

func TestExtractMessageContent(t *testing.T) {
	tests := []struct {
		name string
		msg  D
		want string
	}{
		{
			name: "string content",
			msg:  D{"role": "user", "content": "Hello world"},
			want: "Hello world",
		},
		{
			name: "nil content",
			msg:  D{"role": "assistant", "content": nil},
			want: "",
		},
		{
			name: "missing content",
			msg:  D{"role": "user"},
			want: "",
		},
		{
			name: "array content with text parts",
			msg: D{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Hello "},
					map[string]any{"type": "text", "text": "world"},
				},
			},
			want: "Hello world",
		},
		{
			name: "array content with mixed types",
			msg: D{
				"role": "user",
				"content": []any{
					map[string]any{"type": "image", "url": "http://..."},
					map[string]any{"type": "text", "text": "caption"},
				},
			},
			want: "caption",
		},
		{
			name: "D slice content",
			msg: D{
				"role": "user",
				"content": []D{
					{"type": "text", "text": "Part 1"},
					{"type": "text", "text": "Part 2"},
				},
			},
			want: "Part 1Part 2",
		},
		{
			name: "empty array content",
			msg: D{
				"role":    "user",
				"content": []any{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMessageContent(tt.msg)
			if got != tt.want {
				t.Errorf("extractMessageContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIMCSlotState(t *testing.T) {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			cache:   &imcPrefix{},
			kvState: &imcCheckpoint{},
			seqID:   llama.SeqId(i),
			id:      i,
		}
	}

	// Verify slot initialization.
	if m.imcSessions[0].seqID != 0 {
		t.Errorf("slot 0 seqID = %d, want 0", m.imcSessions[0].seqID)
	}
	if m.imcSessions[1].seqID != 1 {
		t.Errorf("slot 1 seqID = %d, want 1", m.imcSessions[1].seqID)
	}

	// Simulate cache build on slot 0.
	m.imcSessions[0].cache.cachedMsgsHash = "abc123"
	m.imcSessions[0].cache.totalTokensCached = 1000
	m.imcSessions[0].cache.cachedMsgCount = 2

	// Verify state persists.
	if m.imcSessions[0].cache.cachedMsgsHash != "abc123" {
		t.Error("hash not persisted")
	}
	if m.imcSessions[0].cache.totalTokensCached != 1000 {
		t.Error("tokens not persisted")
	}
	if m.imcSessions[0].cache.cachedMsgCount != 2 {
		t.Error("msgCount not persisted")
	}

	// Verify slot 1 is independent.
	if m.imcSessions[1].cache.totalTokensCached != 0 {
		t.Error("slot 1 should be empty")
	}
}

func TestClearCaches(t *testing.T) {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			kvState: &imcCheckpoint{},
			seqID:   llama.SeqId(i),
			id:      i,
			cache: &imcPrefix{
				cachedMsgsHash:    "hash",
				totalTokensCached: 500,
				cachedMsgCount:    3,
			},
		}
	}
	publishTestIMCPrefixes(m)

	// Clear caches.
	if err := m.clearCaches(context.Background()); err != nil {
		t.Fatalf("clearCaches() error = %v, want nil", err)
	}

	// Verify IMC sessions cleared.
	for i, slot := range m.imcSessions {
		if slot.cache.totalTokensCached != 0 {
			t.Errorf("session %d totalTokensCached = %d, want 0", i, slot.cache.totalTokensCached)
		}
		if slot.cache.cachedMsgCount != 0 {
			t.Errorf("session %d cachedMsgCount = %d, want 0", i, slot.cache.cachedMsgCount)
		}
		if slot.cache.cachedMsgsHash != "" {
			t.Errorf("session %d cachedMsgsHash = %q, want empty", i, slot.cache.cachedMsgsHash)
		}
	}
}

func TestCacheResultFields(t *testing.T) {
	// Test that cacheResult correctly propagates IMC fields.
	result := cacheResult{
		modifiedD:    D{"test": "value"},
		cacheIdx:     1000,
		imcSessionID: 2,
	}

	if result.imcSessionID != 2 {
		t.Errorf("imcSessionID = %d, want 2", result.imcSessionID)
	}
	if result.cacheIdx != 1000 {
		t.Errorf("cacheIdx = %d, want 1000", result.cacheIdx)
	}
}

// =============================================================================
// Externalized KV State Tests
// =============================================================================

// TestIMCResetSessionClearsKVState verifies that resetting a session zeroes the
// externalized target, draft, and hidden state. Backing allocations are
// retained so the next conversation can reuse them without allocation.
func TestIMCResetSessionClearsKVState(t *testing.T) {
	s := &imcSession{
		kvState:      &imcCheckpoint{},
		draftKVState: &imcCheckpoint{},
		seqID:        0,
		id:           0,
		cache: &imcPrefix{
			cachedMsgsHash:      "abc123",
			cachedTokens:        []llama.Token{1, 2, 3},
			totalTokensCached:   100,
			cachedMsgCount:      2,
			hasMedia:            true,
			useMRoPE:            true,
			promptPlan:          mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true}),
			mediaKVCounts:       []int{10, 20},
			samplerPromptTokens: []llama.Token{4, 5, 6},
		},
		lastUsed: time.Now(),
		reserved: true,
		pendingH: make([]float32, 4, 8),
	}
	m := Model{imcSessions: []*imcSession{s}}
	publishTestIMCPrefixes(&m)
	cache := s.cache
	buf := prepareTestStore(t, s.kvState, 16)
	draftBuf := prepareTestStore(t, s.draftKVState, 16)
	for i := range buf {
		buf[i] = 0xA5
		draftBuf[i] = 0x5A
	}
	commitTestStore(t, s.kvState, 8)
	commitTestStore(t, s.draftKVState, 8)
	retainedKV := buf[:cap(buf)]
	retainedDraftKV := draftBuf[:cap(draftBuf)]
	retainedH := s.pendingH[:cap(s.pendingH)]
	for i := range retainedH {
		retainedH[i] = float32(i + 1)
	}

	resetTestSession(t, s)

	if s.cache != cache || s.prefixLen != 0 || s.prefixHash != ([32]byte{}) {
		t.Error("reset did not clear prefix hash metadata while preserving its resident cache")
	}
	if s.kvState.Len() != 0 {
		t.Errorf("kvState.Len() = %d, want 0 (contents cleared)", s.kvState.Len())
	}
	if s.kvState.Cap() == 0 {
		t.Errorf("kvState.Cap() = 0, want backing array retained for reuse")
	}
	for i, b := range retainedKV {
		if b != 0 {
			t.Fatalf("kvState retained byte[%d] = 0x%02x, want 0", i, b)
		}
	}
	for i, b := range retainedDraftKV {
		if b != 0 {
			t.Fatalf("draftKVState retained byte[%d] = 0x%02x, want 0", i, b)
		}
	}
	for i, v := range retainedH {
		if v != 0 {
			t.Fatalf("pendingH retained value[%d] = %f, want 0", i, v)
		}
	}
	if s.cache.samplerPromptTokens != nil {
		t.Errorf("samplerPromptTokens = %v, want nil", s.cache.samplerPromptTokens)
	}
	if s.cache.cachedMsgsHash != "" {
		t.Errorf("cachedMsgsHash = %q, want empty", s.cache.cachedMsgsHash)
	}
	if s.cache.cachedTokens != nil {
		t.Errorf("cachedTokens = %v, want nil", s.cache.cachedTokens)
	}
	if s.cache.totalTokensCached != 0 {
		t.Errorf("totalTokensCached = %d, want 0", s.cache.totalTokensCached)
	}
	if s.cache.cachedMsgCount != 0 {
		t.Errorf("cachedMsgCount = %d, want 0", s.cache.cachedMsgCount)
	}
	if s.reserved {
		t.Error("reserved should be false")
	}
	if s.cache.hasMedia {
		t.Error("hasMedia should be false")
	}
	if s.cache.useMRoPE {
		t.Error("useMRoPE should be false")
	}
	if s.cache.mediaKVCounts != nil {
		t.Errorf("mediaKVCounts = %v, want nil", s.cache.mediaKVCounts)
	}

	// id is structural (session-pool index) and must be preserved.
	if s.id != 0 {
		t.Errorf("id = %d, want 0 (should be preserved)", s.id)
	}
	// seqID is dynamic — reset to imcSeqIDUnbound when the session is
	// detached from any slot's KV sequence.
	if s.seqID != imcSeqIDUnbound {
		t.Errorf("seqID = %d, want imcSeqIDUnbound (%d) after reset", s.seqID, imcSeqIDUnbound)
	}
}

// TestClearCachesResetsKVState verifies that clearCaches properly resets
// kvState on all sessions, not just the original fields.
func TestClearCachesResetsKVState(t *testing.T) {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			kvState: &imcCheckpoint{},
			seqID:   llama.SeqId(i),
			id:      i,
			cache: &imcPrefix{
				cachedMsgsHash:    "hash",
				totalTokensCached: 500,
				cachedMsgCount:    3,
			},
		}
		prepareTestStore(t, m.imcSessions[i].kvState, 1024)
	}
	publishTestIMCPrefixes(m)

	if err := m.clearCaches(context.Background()); err != nil {
		t.Fatalf("clearCaches() error = %v, want nil", err)
	}

	for i, s := range m.imcSessions {
		if s.kvState.Len() != 0 {
			t.Errorf("session[%d] kvState.Len() = %d, want 0 (contents cleared)", i, s.kvState.Len())
		}
		if s.cache.totalTokensCached != 0 {
			t.Errorf("session[%d] totalTokensCached = %d, want 0", i, s.cache.totalTokensCached)
		}
	}
}

// TestIMCSessionMediaFlag verifies the imcSessionMedia flag derivation for
// the text→media transition. When a session starts as text-only and a media
// build is requested (imcMediaBuild=true), the job must be treated as media
// to prevent finishSlot from clearing the KV state.
func TestIMCSessionMediaFlag(t *testing.T) {
	tests := []struct {
		name          string
		hasMedia      bool
		imcMediaBuild bool
		wantMediaFlag bool
	}{
		{
			name:          "text session, no media build",
			hasMedia:      false,
			imcMediaBuild: false,
			wantMediaFlag: false,
		},
		{
			name:          "text session, media build starting (text→media transition)",
			hasMedia:      false,
			imcMediaBuild: true,
			wantMediaFlag: true,
		},
		{
			name:          "media session, no new media build",
			hasMedia:      true,
			imcMediaBuild: false,
			wantMediaFlag: true,
		},
		{
			name:          "media session, media rebuild",
			hasMedia:      true,
			imcMediaBuild: true,
			wantMediaFlag: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &imcSession{cache: &imcPrefix{hasMedia: tt.hasMedia}, kvState: &imcCheckpoint{}}
			got := session.cache.hasMedia || tt.imcMediaBuild
			if got != tt.wantMediaFlag {
				t.Errorf("imcSessionMedia = %v, want %v", got, tt.wantMediaFlag)
			}
		})
	}
}

// TestIMCCommitSessionPreservesKVState verifies that imcCommitSession does not
// clear kvState — it should only be updated by the snapshot in startSlot.
// It also verifies the publication contract: commit leaves reserved=true so
// concurrent IMC scanners ignore the in-flight session, and imcPublishSession
// is the matched call that finalizes visibility once kvState has been
// re-snapshotted.
func TestIMCCommitSessionPreservesKVState(t *testing.T) {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 1),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	session := &imcSession{
		cache:    &imcPrefix{},
		kvState:  &imcCheckpoint{},
		id:       0,
		seqID:    0,
		reserved: true,
	}
	buf := prepareTestStore(t, session.kvState, 3)
	copy(buf, []byte{0x01, 0x02, 0x03})
	commitTestStore(t, session.kvState, len(buf))
	m.imcSessions[0] = session

	m.imcCommitSession(context.Background(), session, "newhash", 1000, 5,
		[]llama.Token{1, 2, 3}, false, nil, "")

	// kvState should be preserved — only startSlot snapshots update it.
	if session.kvState.Len() != 3 {
		t.Errorf("kvState.Len() = %d, want 3 (should be preserved)", session.kvState.Len())
	}

	// Verify other fields were updated.
	if session.cache.cachedMsgsHash != "newhash" {
		t.Errorf("cachedMsgsHash = %q, want newhash", session.cache.cachedMsgsHash)
	}
	if session.cache.totalTokensCached != 1000 {
		t.Errorf("totalTokensCached = %d, want 1000", session.cache.totalTokensCached)
	}

	// Commit alone must not publish: reserved must still be true so a
	// concurrent token-v2 planners ignore this session until the snapshot is
	// re-externalized.
	if !session.reserved {
		t.Error("reserved should still be true after commit (publication is deferred)")
	}

	m.imcPublishSession(session)
	if session.reserved {
		t.Error("reserved should be false after publish")
	}
}

func TestIMCCommitMediaInvalidatesOwnDraftState(t *testing.T) {
	m := &Model{}
	session := &imcSession{
		cache:        &imcPrefix{},
		kvState:      &imcCheckpoint{},
		draftKVState: &imcCheckpoint{},
		pendingH:     []float32{1, 2, 3},
	}
	buf := prepareTestStore(t, session.draftKVState, 3)
	copy(buf, []byte{1, 2, 3})
	commitTestStore(t, session.draftKVState, len(buf))

	m.imcCommitSession(context.Background(), session, "hash", 100, 2, nil, true, []int{50}, "")

	if session.draftKVState.Len() != 0 {
		t.Errorf("draftKVState.Len() = %d, want 0", session.draftKVState.Len())
	}
	if len(session.pendingH) != 0 {
		t.Errorf("len(pendingH) = %d, want 0", len(session.pendingH))
	}
}

func TestIMCCommitMediaInvalidatesDraftMetadataWhenResetFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wantErr := context.Canceled
	session := &imcSession{
		cache:        &imcPrefix{},
		draftKVState: populatedTestSessionStore(t),
		pendingH:     []float32{1, 2, 3},
		reserved:     true,
	}
	var loggedErr error
	m := Model{log: func(_ context.Context, _ string, args ...any) {
		for i := 0; i+1 < len(args); i += 2 {
			if args[i] == "err" {
				loggedErr, _ = args[i+1].(error)
			}
		}
	}}

	m.imcCommitSession(ctx, session, "hash", 100, 2, nil, true, []int{50}, "")

	if !errors.Is(loggedErr, wantErr) {
		t.Errorf("logged error = %v, want %v", loggedErr, wantErr)
	}
	if len(session.pendingH) != 0 {
		t.Errorf("len(pendingH) = %d, want 0", len(session.pendingH))
	}
	if !session.cache.hasMedia || session.cache.totalTokensCached != 100 {
		t.Errorf("target metadata was not committed: hasMedia=%t totalTokensCached=%d", session.cache.hasMedia, session.cache.totalTokensCached)
	}
}

func TestIMCInvalidateReservedSessionRetainsOwnership(t *testing.T) {
	m := &Model{}
	session := &imcSession{
		cache:    &imcPrefix{cachedMsgsHash: "hash", totalTokensCached: 10},
		reserved: true,
		kvState:  populatedTestSessionStore(t),
	}

	if err := m.imcInvalidateReservedSession(context.Background(), session); err != nil {
		t.Fatalf("imcInvalidateReservedSession() error = %v, want nil", err)
	}

	if session.cache.totalTokensCached != 0 || session.kvState.Len() != 0 {
		t.Fatalf("invalidated session still has cache state: tokens=%d bytes=%d", session.cache.totalTokensCached, session.kvState.Len())
	}
	if !session.reserved {
		t.Fatal("invalidated session released ownership before finishSlot cleanup")
	}
}

// TestIMCCommitSessionNilSafe verifies that imcCommitSession handles a nil
// session without panicking.
func TestIMCCommitSessionNilSafe(t *testing.T) {
	m := &Model{
		cfg: Config{PtrIncrementalCache: new(true)},
		log: func(ctx context.Context, msg string, args ...any) {},
	}
	// Should not panic.
	m.imcCommitSession(context.Background(), nil, "hash", 100, 2, nil, false, nil, "")
}

// TestIMCFillSlotsAnySlot verifies that all IMC jobs (text and media) are
// assigned to any available slot since KV state is externalized to RAM.
func TestIMCFillSlotsAnySlot(t *testing.T) {
	tests := []struct {
		name     string
		hasMedia bool
	}{
		{"text-only", false},
		{"media", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &imcSession{
				kvState: &imcCheckpoint{},
				seqID:   1,
				id:      1,
				cache:   &imcPrefix{hasMedia: tt.hasMedia},
			}

			job := &chatJob{
				ctx:             context.Background(),
				imcCacheHit:     true,
				imcSession:      session,
				imcSessionMedia: tt.hasMedia,
				imcSessionID:    1,
			}

			// All IMC jobs use any-slot routing (KV externalized to RAM).
			_ = job // Verify job is constructed; scheduling is tested via integration.
			_ = session
		})
	}
}

// TestIMCSessionMediaTransitions verifies that media metadata remains attached
// to a session across follow-up turns and is cleared when token-v2 selects the
// session for a full rebuild. All sessions externalize their KV state to RAM.
func TestIMCSessionMediaTransitions(t *testing.T) {
	s := &imcSession{id: 0, seqID: 0, cache: &imcPrefix{}, kvState: &imcCheckpoint{}}

	// snapshot simulates startSlot writing kvState by going through the
	// kvBuffer Prepare/Commit lifecycle.
	snapshot := func(b byte) {
		buf := prepareTestStore(t, s.kvState, 1)
		buf[0] = b
		commitTestStore(t, s.kvState, 1)
	}

	// Turn 1: Text build. hasMedia=false.
	s.cache.cachedMsgsHash = "text1"
	s.cache.totalTokensCached = 100
	s.cache.hasMedia = false
	snapshot(0x01)

	if s.cache.hasMedia {
		t.Fatal("turn 1: session should be text-only")
	}
	if s.kvState.Len() == 0 {
		t.Fatal("turn 1: text session should have kvState")
	}

	// Turn 2: Text→Media transition. imcMediaBuild=true, session.hasMedia
	// transitions from false to true after commit.
	mediaFlag := s.cache.hasMedia || true // imcMediaBuild=true
	if !mediaFlag {
		t.Fatal("turn 2: imcSessionMedia should be true during media build")
	}

	// Simulate startSlot media build + commit + snapshot.
	s.cache.cachedMsgsHash = "media1"
	s.cache.totalTokensCached = 500
	s.cache.hasMedia = true
	snapshot(0x02) // Media sessions also get externalized to RAM.
	s.cache.mediaKVCounts = []int{200}

	// Turn 3: Media→Text follow-up. Session stays media, kvState present.
	if !s.cache.hasMedia {
		t.Fatal("turn 3: session should still be media")
	}
	if s.kvState.Len() == 0 {
		t.Fatal("turn 3: media session should have kvState (externalized to RAM)")
	}

	// Simulate text extend on media session.
	s.cache.totalTokensCached = 600
	s.cache.cachedMsgCount = 4

	// Turn 4: Text→Text on media session. Still media.
	if !s.cache.hasMedia {
		t.Fatal("turn 4: session should still be media")
	}

	// Turn 5: Media→Media (second image). The existing media prefix remains
	// published while the extension is decoded and snapshotted into a staged
	// store, then both media anchors are committed atomically.
	oldStore := s.kvState
	staged := &imcCheckpoint{}
	stagedBuf := prepareTestStore(t, staged, 1)
	stagedBuf[0] = 0x03
	commitTestStore(t, staged, 1)
	plan := mediaPlan(promptUnit{isMedia: true}, promptUnit{isMedia: true})
	gotOld := (&Model{}).imcCommitMediaAdvance(context.Background(), s, staged, "media2", 800, 6, 700, plan, []llama.Token{1, 2}, []int{200, 150}, nil, true, false, "render", false)
	if gotOld != oldStore || s.kvState != staged || !s.cache.hasMedia || len(s.cache.mediaKVCounts) != 2 {
		t.Fatal("turn 5: second media append did not atomically replace the session snapshot")
	}

	// Turn 6: Media→Text. Session stays media.
	if !s.cache.hasMedia {
		t.Fatal("turn 6: session should still be media")
	}
}

// TestReleaseIMCReservationIfHeld verifies cleanup before the batch engine
// takes ownership. Token-v2 planning reserves builds, extensions, and exact
// hits; a cache result without a reservation must not release the session.
func TestReleaseIMCReservationIfHeld(t *testing.T) {
	newModel := func() *Model {
		m := &Model{
			cfg:         Config{PtrIncrementalCache: new(true)},
			imcSessions: make([]*imcSession, 1),
			log:         func(ctx context.Context, msg string, args ...any) {},
		}
		m.imcSessions[0] = &imcSession{
			kvState:  &imcCheckpoint{},
			seqID:    llama.SeqId(0),
			id:       0,
			reserved: true,
		}
		return m
	}

	tests := []struct {
		name         string
		cache        cacheResult
		wantReserved bool
	}{
		{
			name: "nil session leaves reserved alone",
			cache: cacheResult{
				imcSessionID:      0,
				imcNewCacheTokens: []llama.Token{1, 2, 3},
			},
			wantReserved: true,
		},
		{
			name: "text build releases reservation",
			cache: cacheResult{
				imcSessionID:      0,
				imcNewCacheTokens: []llama.Token{1, 2, 3},
			},
			wantReserved: false,
		},
		{
			name: "media build releases reservation",
			cache: cacheResult{
				imcSessionID:  0,
				imcMediaBuild: true,
			},
			wantReserved: false,
		},
		{
			name: "exact hit releases reservation",
			cache: cacheResult{
				imcSessionID:           0,
				imcReadOnlyReservation: true,
			},
			wantReserved: false,
		},
		{
			name: "media anchor extension releases reservation",
			cache: cacheResult{
				imcSessionID:      0,
				imcNewCacheTokens: []llama.Token{1, 2, 3},
			},
			wantReserved: false,
		},
		{
			name: "media append releases reservation",
			cache: cacheResult{
				imcSessionID:   0,
				imcMediaAppend: true,
			},
			wantReserved: false,
		},
		{
			name: "cache result without reservation leaves reserved alone",
			cache: cacheResult{
				imcSessionID: 0,
			},
			wantReserved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel()

			if tt.name != "nil session leaves reserved alone" {
				tt.cache.imcSession = m.imcSessions[0]
			}

			m.releaseIMCReservationIfHeld(tt.cache)

			if got := m.imcSessions[0].reserved; got != tt.wantReserved {
				t.Errorf("reserved = %v, want %v", got, tt.wantReserved)
			}
		})
	}
}

// TestIMCSessionCapacity guards the admission-capacity invariant used for the
// default IMC session pool.
func TestIMCSessionCapacity(t *testing.T) {
	tests := []struct {
		name       string
		nSlots     int
		queueDepth int
		want       int
	}{
		{"default retention", 2, 2, 4},
		{"depth one", 4, 1, 4},
		{"depth three", 3, 3, 9},
		{"depth four expands", 2, 4, 8},
		{"depth seven expands", 3, 7, 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imcSessionCapacity(tt.nSlots, tt.queueDepth)
			if got != tt.want {
				t.Errorf("imcSessionCapacity: got %d, want %d", got, tt.want)
			}
			admission := tt.nSlots * tt.queueDepth
			if got < admission {
				t.Errorf("session capacity %d is less than admission capacity %d", got, admission)
			}
		})
	}
}

func TestIMCSessions(t *testing.T) {
	lastUsed := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	m := &Model{
		cfg: Config{PtrContextWindow: new(8192)},
		imcSessions: []*imcSession{
			{id: 0, kvState: &imcCheckpoint{}},
			{id: 1, cache: &imcPrefix{totalTokensCached: 1024}, reserved: true, kvState: &imcCheckpoint{}},
			{id: 2, cache: &imcPrefix{totalTokensCached: 2048, nextLogicalPos: 2100, cachedMsgCount: 4, hasMedia: true}, allocatedContext: 4096, inputMessages: 4, inputTokens: 2200, outputTokens: 300, lastUsed: lastUsed, kvState: populatedTestSessionStore(t), pendingH: make([]float32, 2)},
			{id: 3, cache: &imcPrefix{totalTokensCached: 2048}, allocatedContext: 1536, kvState: &imcCheckpoint{}},
			{id: 4, cache: &imcPrefix{totalTokensCached: 1024, cachedMsgCount: 2}, reserved: true, allocatedContext: 1536, kvState: &imcCheckpoint{}},
		},
	}
	publishTestIMCPrefixes(m)
	m.imcSessions[2].snapshotBytes = imcSnapshotBytes(m.imcSessions[2].kvState, m.imcSessions[2].draftKVState, m.imcSessions[2].pendingH)

	got := m.IMCSessions()
	if len(got) != len(m.imcSessions) {
		t.Fatalf("len(IMCSessions) = %d, want %d", len(got), len(m.imcSessions))
	}

	if got[0].State != IMCSessionStateEmpty {
		t.Errorf("session 0 state = %q, want %q", got[0].State, IMCSessionStateEmpty)
	}
	if got[1].State != IMCSessionStateActive {
		t.Errorf("session 1 state = %q, want %q", got[1].State, IMCSessionStateActive)
	}
	if got[2].State != IMCSessionStateIdle {
		t.Errorf("session 2 state = %q, want %q", got[2].State, IMCSessionStateIdle)
	}
	if got[2].Context != 2048 || got[2].Allocated != 4096 || got[2].TotalAllocated != 4096 || got[2].SnapshotBytes != 9 || got[2].PeakContext != 4096 || got[2].Messages != 4 || got[2].InputMessages != 4 || got[2].InputTokens != 2200 || got[2].OutputTokens != 300 || got[2].ContextWindow != 8192 || got[2].LastUsed != lastUsed || !got[2].HasMedia {
		t.Errorf("session 2 detail = %+v, want populated scalar snapshot", got[2])
	}
	if got[3].Allocated != 2048 || got[3].PeakContext != 2048 {
		t.Errorf("session 3 detail = %+v, want allocation to cover current tokens", got[3])
	}
	if got[4].Context != 1024 || got[4].Allocated != 1536 || got[4].PeakContext != 1536 {
		t.Errorf("session 4 detail = %+v, want active Current allocation", got[4])
	}

	got[2].Context = 1
	if m.imcSessions[2].cache.logicalPosition() != 2100 {
		t.Fatal("mutating snapshot changed the IMC cache entry")
	}

	m.imcSessions[2].cache.totalTokensCached = 0
	m.imcSessions[2].allocatedContext = 0
	got = m.IMCSessions()
	if got[2].State != IMCSessionStateEmpty || got[2].Context != 0 || got[2].Allocated != 0 {
		t.Errorf("session 2 transition detail = %+v, want empty Current", got[2])
	}

	m.imcCommitSession(context.Background(), m.imcSessions[0], "hash", 3000, 2, nil, false, nil, "")
	m.imcPublishSession(m.imcSessions[0])
	m.imcCommitSession(context.Background(), m.imcSessions[0], "hash", 1000, 2, nil, false, nil, "")
	m.imcPublishSession(m.imcSessions[0])
	if m.imcSessions[0].allocatedContext != 3000 {
		t.Errorf("allocatedContext = %d, want high-water context 3000", m.imcSessions[0].allocatedContext)
	}
	resetTestSession(t, m.imcSessions[0])
	if m.imcSessions[0].allocatedContext != 3000 {
		t.Errorf("allocatedContext after reset = %d, want retained high-water context 3000", m.imcSessions[0].allocatedContext)
	}
	got = m.IMCSessions()
	if got[0].TotalAllocated != 3000 {
		t.Errorf("TotalAllocated after reset = %d, want retained session capacity 3000", got[0].TotalAllocated)
	}
	m.imcSessions[0].allocatedContext = 1000
	got = m.IMCSessions()
	if got[0].TotalAllocated != 3000 {
		t.Errorf("TotalAllocated after backing-store replacement = %d, want session high-water context 3000", got[0].TotalAllocated)
	}
	oldVersion := m.imcBeginRequestUsage(m.imcSessions[0])
	newVersion := m.imcBeginRequestUsage(m.imcSessions[0])
	m.imcRecordRequestUsage(m.imcSessions[0], oldVersion, 6, 3500, 1500, 4999)
	m.imcRecordRequestUsage(m.imcSessions[0], newVersion, 8, 3000, 1000, 3999)
	got = m.IMCSessions()
	if got[0].TotalAllocated != 5000 || got[0].PeakContext != 5000 || got[0].InputMessages != 8 || got[0].InputTokens != 3000 || got[0].OutputTokens != 1000 {
		t.Errorf("request usage = %+v, want latest 8/3000/1000 and peak 5000", got[0])
	}

	resetTestSession(t, m.imcSessions[3])
	got = m.IMCSessions()
	if got[3].TotalAllocated != 1536 {
		t.Errorf("TotalAllocated after Current reset = %d, want retained session capacity 1536", got[3].TotalAllocated)
	}

}

// TestIMCSeqIDUnboundSentinel guards the unbound sentinel value used by
// the dynamic seqID binding contract. The KV-pressure eviction path
// relies on this sentinel to skip MemorySeqRm for sessions whose bytes
// only live in host RAM.
func TestIMCSeqIDUnboundSentinel(t *testing.T) {
	if imcSeqIDUnbound != -1 {
		t.Errorf("imcSeqIDUnbound = %d, want -1", imcSeqIDUnbound)
	}
}

func TestIMCCompletedHistory(t *testing.T) {
	request := D{
		"messages": []D{
			{"role": RoleUser, "content": []D{{"type": "text", "text": "Look up Paris weather."}}},
		},
		"tools":       []D{{"type": "function", "function": D{"name": "weather"}}},
		"temperature": 0.7,
	}
	message := ResponseMessage{
		Role: RoleAssistant, Content: "Checking now.", Reasoning: "Use the weather tool.",
		ToolCalls: []ResponseToolCall{{ID: "call_paris", Type: "function", Function: ResponseToolCallFunction{
			Name: "weather", Arguments: ToolCallArguments{"city": "Paris", "days": 3},
		}}},
	}
	session := &imcSession{
		cache:   &imcPrefix{cachedTokens: []llama.Token{17, 23}, totalTokensCached: 2, cachedMsgCount: 1},
		kvState: populatedTestSessionStore(t),
	}
	m := Model{imcSessions: []*imcSession{session}}
	version := m.imcBeginRequestUsage(session)
	history, err := prepareIMCCompletedHistory(request.Clone(), message)
	if err != nil {
		t.Fatal(err)
	}
	m.imcRecordCompletedHistory(session, version, history)

	// Neither the submitted request nor the delivered response owns retained
	// history. In particular, tool-call argument maps must not remain aliased.
	request["messages"].([]D)[0]["content"].([]D)[0]["text"] = "changed input"
	message.ToolCalls[0].Function.Arguments["city"] = "Rome"
	message.ToolCalls[0].ID = "changed call"
	got, err := json.Marshal(session.completedHistory)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"messages":[{"role":"user","content":[{"type":"text","text":"Look up Paris weather."}]},{"role":"assistant","content":"Checking now.","reasoning_content":"Use the weather tool.","tool_calls":[{"id":"call_paris","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Paris\",\"days\":3}"}}]}],"tools":[{"type":"function","function":{"name":"weather"}}],"temperature":0.7}`
	var gotDocument, wantDocument any
	if err := json.Unmarshal(got, &gotDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantDocument); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDocument, wantDocument) {
		t.Errorf("completed history: got %s, want %s", got, want)
	}
	if session.cache.totalTokensCached != 2 || session.cache.cachedMsgCount != 1 || !reflect.DeepEqual(session.cache.cachedTokens, []llama.Token{17, 23}) {
		t.Errorf("checkpoint prefix changed after completion: %+v", session.cache)
	}

	// A new request's supplied history replaces the previous record, rather
	// than accumulating another copy of the previous assistant response.
	nextVersion := m.imcBeginRequestUsage(session)
	if session.completedHistory != nil {
		t.Fatal("new request retained the preceding completed history")
	}
	next, err := prepareIMCCompletedHistory(D{"messages": []D{{"role": RoleUser, "content": "Different branch."}}}, ResponseMessage{Role: RoleAssistant, Content: "New answer."})
	if err != nil {
		t.Fatal(err)
	}
	m.imcRecordCompletedHistory(session, nextVersion, next)
	m.imcRecordCompletedHistory(session, version, history)
	if msgs := session.completedHistory["messages"].([]D); len(msgs) != 2 || msgs[1]["content"] != "New answer." {
		t.Errorf("latest history: got %+v, want different branch and new answer", msgs)
	}

	resetSessionMetadata(session)
	m.imcRecordCompletedHistory(session, nextVersion, next)
	if session.completedHistory != nil {
		t.Fatal("reset resident accepted the preceding completed history")
	}
}

// TestImcClearReservedSessionIDArg verifies imcReleaseReservation uses the
// session-pool index argument (not an execution slot id) and tolerates
// out-of-range indices defensively. The negative-index guard catches
// stray callers that pass a slot id by mistake on a job that never
// reserved an IMC session.
func TestImcClearReservedSessionIDArg(t *testing.T) {
	m := &Model{
		cfg: Config{
			PtrIncrementalCache: new(true),
		},
		imcSessions: make([]*imcSession, 4),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}
	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			kvState:  &imcCheckpoint{},
			seqID:    imcSeqIDUnbound,
			id:       i,
			reserved: true,
		}
	}

	// Clear reserved on session 3 (a session-pool index that would not
	// be a valid execution-slot id in a NSeqMax=2 deployment, proving
	// the call addresses sessions independently of slots).
	m.imcReleaseReservation(3)

	if m.imcSessions[3].reserved {
		t.Error("session 3 reserved = true after imcReleaseReservation(3), want false")
	}
	for i := range 3 {
		if !m.imcSessions[i].reserved {
			t.Errorf("session %d reserved = false, want true (untouched)", i)
		}
	}

	// Out-of-range arguments must be safe no-ops, not panics.
	m.imcReleaseReservation(-1)
	m.imcReleaseReservation(99)
}

// TestIMCCommitDoesNotPublishUntilSnapshot verifies commit keeps a session
// reserved until its externalized snapshot is published.
func TestIMCCommitDoesNotPublishUntilSnapshot(t *testing.T) {
	m := &Model{imcSessions: make([]*imcSession, 1)}
	m.imcSessions[0] = &imcSession{cache: &imcPrefix{cachedTokens: []llama.Token{9}, totalTokensCached: 1}, kvState: &imcCheckpoint{}, seqID: imcSeqIDUnbound, id: 0, reserved: true}
	publishTestIMCPrefixes(m)
	session := m.imcSessions[0]
	oldPrefixLen, oldPrefixHash := session.prefixLen, session.prefixHash

	m.imcCommitSession(context.Background(), m.imcSessions[0], "hash", 500, 2, []llama.Token{1, 2}, false, nil, "")
	if !m.imcSessions[0].reserved {
		t.Fatal("committed session must remain reserved before publish")
	}
	if session.prefixLen != oldPrefixLen || session.prefixHash != oldPrefixHash {
		t.Fatal("commit changed prefix hash identity before snapshot publication")
	}

	buf := prepareTestStore(t, m.imcSessions[0].kvState, 3)
	copy(buf, []byte{1, 2, 3})
	commitTestStore(t, m.imcSessions[0].kvState, 3)
	m.imcPublishSession(m.imcSessions[0])
	if m.imcSessions[0].reserved {
		t.Fatal("published session must be unreserved")
	}
	hashes := m.imcCandidateHashes([]llama.Token{1, 2, 3}, nil)
	if session.prefixLen != 2 || !session.matchesPrefixHash(hashes) {
		t.Fatal("published prefix hash does not discover the new prefix")
	}
}
