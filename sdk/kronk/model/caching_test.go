package model

import (
	"context"
	"testing"

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
			name: "different content different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "user", "content": "Goodbye"},
			},
			wantSame: false,
		},
		{
			name: "different role different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "assistant", "content": "Hello"},
			},
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
		{
			name:     "empty messages same hash",
			msgs1:    []D{},
			msgs2:    []D{},
			wantSame: true,
		},
		{
			name: "prefix subset different hash",
			msgs1: []D{
				{"role": "user", "content": "Hello"},
			},
			msgs2: []D{
				{"role": "user", "content": "Hello"},
				{"role": "assistant", "content": "Hi"},
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashMessages(tt.msgs1)
			hash2 := hashMessages(tt.msgs2)

			if tt.wantSame && hash1 != hash2 {
				t.Errorf("expected same hash, got %s != %s", hash1, hash2)
			}
			if !tt.wantSame && hash1 == hash2 {
				t.Errorf("expected different hash, got same: %s", hash1)
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

func TestRemoveMessagesAtIndices(t *testing.T) {
	tests := []struct {
		name      string
		messages  []D
		indices   []int
		wantCount int
		wantFirst string
	}{
		{
			name: "remove first message",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user"},
			},
			indices:   []int{0},
			wantCount: 1,
			wantFirst: "user",
		},
		{
			name: "remove last message",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user"},
			},
			indices:   []int{1},
			wantCount: 1,
			wantFirst: "sys",
		},
		{
			name: "remove multiple messages",
			messages: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "user1"},
				{"role": "assistant", "content": "asst"},
				{"role": "user", "content": "user2"},
			},
			indices:   []int{0, 2},
			wantCount: 2,
			wantFirst: "user1",
		},
		{
			name: "remove none",
			messages: []D{
				{"role": "user", "content": "keep"},
			},
			indices:   []int{},
			wantCount: 1,
			wantFirst: "keep",
		},
		{
			name: "remove all",
			messages: []D{
				{"role": "user", "content": "remove"},
			},
			indices:   []int{0},
			wantCount: 1, // Default message added when result would be empty
			wantFirst: "Tell the user you are ready to help them.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": tt.messages}
			result := removeMessagesAtIndices(d, tt.indices)

			msgs, ok := result["messages"].([]D)
			if !ok {
				t.Fatal("result messages not []D")
			}

			if len(msgs) != tt.wantCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantCount)
			}

			if len(msgs) > 0 {
				content, _ := msgs[0]["content"].(string)
				if content != tt.wantFirst {
					t.Errorf("first message content = %q, want %q", content, tt.wantFirst)
				}
			}
		})
	}
}

func TestHashMessage(t *testing.T) {
	msg1 := cacheableMessage{role: "system", content: "Hello"}
	msg2 := cacheableMessage{role: "system", content: "Hello"}
	msg3 := cacheableMessage{role: "user", content: "Hello"}
	msg4 := cacheableMessage{role: "system", content: "World"}

	hash1 := hashMessage(msg1)
	hash2 := hashMessage(msg2)
	hash3 := hashMessage(msg3)
	hash4 := hashMessage(msg4)

	// Same role and content should produce same hash.
	if hash1 != hash2 {
		t.Errorf("identical messages should have same hash")
	}

	// Different role should produce different hash.
	if hash1 == hash3 {
		t.Errorf("different role should produce different hash")
	}

	// Different content should produce different hash.
	if hash1 == hash4 {
		t.Errorf("different content should produce different hash")
	}

	// Hash should be hex string of expected length (64 chars for SHA-256).
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

// =============================================================================
// Session Management Tests
// =============================================================================

func TestGetOrCreateIMCSession(t *testing.T) {
	// Create minimal model with IMC enabled.
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make(map[string]*imcSession),
		imcNextSeq:  0,
		imcMaxSeqs:  3,
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	ctx := context.Background()

	// Test 1: First session gets seq 0.
	session1, isNew1 := m.getOrCreateIMCSession(ctx, "user-1")
	if session1 == nil {
		t.Fatal("session1 should not be nil")
	}
	if !isNew1 {
		t.Error("session1 should be new")
	}
	if session1.seqID != 0 {
		t.Errorf("session1.seqID = %d, want 0", session1.seqID)
	}

	// Test 2: Same user returns same session.
	session1Again, isNew1Again := m.getOrCreateIMCSession(ctx, "user-1")
	if session1Again != session1 {
		t.Error("same user should return same session")
	}
	if isNew1Again {
		t.Error("same user should not be new")
	}

	// Test 3: Second user gets seq 1.
	session2, isNew2 := m.getOrCreateIMCSession(ctx, "user-2")
	if session2 == nil {
		t.Fatal("session2 should not be nil")
	}
	if !isNew2 {
		t.Error("session2 should be new")
	}
	if session2.seqID != 1 {
		t.Errorf("session2.seqID = %d, want 1", session2.seqID)
	}

	// Test 4: Third user gets seq 2.
	session3, _ := m.getOrCreateIMCSession(ctx, "user-3")
	if session3.seqID != 2 {
		t.Errorf("session3.seqID = %d, want 2", session3.seqID)
	}

	// Test 5: Fourth user should be rejected (max 3 sessions).
	session4, _ := m.getOrCreateIMCSession(ctx, "user-4")
	if session4 != nil {
		t.Error("session4 should be nil (max sessions reached)")
	}

	// Verify total sessions.
	if len(m.imcSessions) != 3 {
		t.Errorf("imcSessions count = %d, want 3", len(m.imcSessions))
	}
}

func TestIMCSessionState(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make(map[string]*imcSession),
		imcNextSeq:  0,
		imcMaxSeqs:  2,
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	ctx := context.Background()

	// Create session and update state.
	session, _ := m.getOrCreateIMCSession(ctx, "test-user")

	// Simulate cache build.
	session.cachedMsgsHash = "abc123"
	session.totalTokensCached = 1000
	session.lastMsgIdxCached = 2

	// Retrieve session again and verify state persists.
	sessionAgain, isNew := m.getOrCreateIMCSession(ctx, "test-user")
	if isNew {
		t.Error("should not be new")
	}
	if sessionAgain.cachedMsgsHash != "abc123" {
		t.Error("hash not persisted")
	}
	if sessionAgain.totalTokensCached != 1000 {
		t.Error("tokens not persisted")
	}
	if sessionAgain.lastMsgIdxCached != 2 {
		t.Error("msgCount not persisted")
	}
}

func TestClearCaches(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache:  true,
			SystemPromptCache: true,
		},
		imcSessions: make(map[string]*imcSession),
		imcNextSeq:  0,
		imcMaxSeqs:  2,
		spcEntries:  make(map[string]*spcEntry),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	ctx := context.Background()

	// Create some IMC sessions.
	m.getOrCreateIMCSession(ctx, "user-1")
	m.getOrCreateIMCSession(ctx, "user-2")

	// Create some SPC entries.
	m.getOrCreateSPCEntry("user-3")
	m.getOrCreateSPCEntry("user-4")

	if len(m.imcSessions) != 2 {
		t.Fatalf("expected 2 IMC sessions, got %d", len(m.imcSessions))
	}

	if len(m.spcEntries) != 2 {
		t.Fatalf("expected 2 SPC entries, got %d", len(m.spcEntries))
	}

	// Clear caches.
	m.clearCaches()

	// Verify IMC sessions cleared.
	if len(m.imcSessions) != 0 {
		t.Errorf("imcSessions not cleared, got %d", len(m.imcSessions))
	}

	// Verify SPC entries cleared.
	if len(m.spcEntries) != 0 {
		t.Errorf("spcEntries not cleared, got %d", len(m.spcEntries))
	}
}

func TestCacheResultFields(t *testing.T) {
	// Test that cacheResult correctly propagates IMC fields.
	result := cacheResult{
		modifiedD:  D{"test": "value"},
		cacheIdx:   1000,
		cacheID:    "user-123",
		cacheSeqID: llama.SeqId(2),
	}

	if result.cacheID != "user-123" {
		t.Errorf("cacheID = %s, want user-123", result.cacheID)
	}
	if result.cacheSeqID != 2 {
		t.Errorf("cacheSeqID = %d, want 2", result.cacheSeqID)
	}
	if result.cacheIdx != 1000 {
		t.Errorf("cacheIdx = %d, want 1000", result.cacheIdx)
	}
}
