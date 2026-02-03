package model

import (
	"context"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestHashPrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompt1  string
		prompt2  string
		wantSame bool
	}{
		{
			name:     "identical prompts same hash",
			prompt1:  "You are helpful\nHello",
			prompt2:  "You are helpful\nHello",
			wantSame: true,
		},
		{
			name:     "different content different hash",
			prompt1:  "Hello",
			prompt2:  "Goodbye",
			wantSame: false,
		},
		{
			name:     "empty prompts same hash",
			prompt1:  "",
			prompt2:  "",
			wantSame: true,
		},
		{
			name:     "prefix different from full",
			prompt1:  "Hello",
			prompt2:  "Hello World",
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashPrompt(tt.prompt1)
			hash2 := hashPrompt(tt.prompt2)

			switch {
			case tt.wantSame && hash1 != hash2:
				t.Errorf("expected same hash, got %s != %s", hash1, hash2)
			case !tt.wantSame && hash1 == hash2:
				t.Errorf("expected different hash, got same: %s", hash1)
			}
		})
	}
}



func TestRemoveMessagesAtIndices(t *testing.T) {
	tests := []struct {
		name       string
		messages   []D
		indices    []int
		wantCount  int
		wantFirst  string
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
			wantCount: 1, // Original returned when result would be empty
			wantFirst: "remove",
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

func TestFindCacheableMessage(t *testing.T) {
	tests := []struct {
		name       string
		messages   []D
		targetRole string
		wantFound  bool
		wantIndex  int
		wantContent string
	}{
		{
			name: "find system message",
			messages: []D{
				{"role": "system", "content": "You are helpful"},
				{"role": "user", "content": "Hello"},
			},
			targetRole:  RoleSystem,
			wantFound:   true,
			wantIndex:   0,
			wantContent: "You are helpful",
		},
		{
			name: "find user message",
			messages: []D{
				{"role": "system", "content": "System"},
				{"role": "user", "content": "User message"},
			},
			targetRole:  RoleUser,
			wantFound:   true,
			wantIndex:   1,
			wantContent: "User message",
		},
		{
			name: "no system message",
			messages: []D{
				{"role": "user", "content": "Hello"},
			},
			targetRole: RoleSystem,
			wantFound:  false,
		},
		{
			name: "empty content skipped",
			messages: []D{
				{"role": "system", "content": ""},
				{"role": "system", "content": "Valid system"},
			},
			targetRole:  RoleSystem,
			wantFound:   true,
			wantIndex:   1,
			wantContent: "Valid system",
		},
		{
			name: "finds first matching role",
			messages: []D{
				{"role": "user", "content": "First"},
				{"role": "user", "content": "Second"},
			},
			targetRole:  RoleUser,
			wantFound:   true,
			wantIndex:   0,
			wantContent: "First",
		},
		{
			name: "array content extraction",
			messages: []D{
				{"role": "system", "content": []any{
					map[string]any{"type": "text", "text": "Array content"},
				}},
			},
			targetRole:  RoleSystem,
			wantFound:   true,
			wantIndex:   0,
			wantContent: "Array content",
		},
		{
			name:       "empty messages",
			messages:   []D{},
			targetRole: RoleSystem,
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := findCacheableMessage(tt.messages, tt.targetRole)

			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
				return
			}

			if !found {
				return
			}

			if msg.index != tt.wantIndex {
				t.Errorf("index = %d, want %d", msg.index, tt.wantIndex)
			}

			if msg.content != tt.wantContent {
				t.Errorf("content = %q, want %q", msg.content, tt.wantContent)
			}

			if msg.role != tt.targetRole {
				t.Errorf("role = %q, want %q", msg.role, tt.targetRole)
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

	// Simulate cache build with new struct fields.
	session.promptHash = "abc123hash"
	session.promptLen = 5000
	session.tokens = 1000

	// Retrieve session again and verify state persists.
	sessionAgain, isNew := m.getOrCreateIMCSession(ctx, "test-user")
	if isNew {
		t.Error("should not be new")
	}
	if sessionAgain.promptHash != "abc123hash" {
		t.Error("promptHash not persisted")
	}
	if sessionAgain.tokens != 1000 {
		t.Error("tokens not persisted")
	}
	if sessionAgain.promptLen != 5000 {
		t.Error("promptLen not persisted")
	}
}

func TestClearCaches(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions:     make(map[string]*imcSession),
		imcNextSeq:      0,
		imcMaxSeqs:      2,
		sysPromptHash:   "sys-hash",
		sysPromptTokens: 100,
		sysPromptLen:    500,
		log:             func(ctx context.Context, msg string, args ...any) {},
	}

	ctx := context.Background()

	// Create some sessions.
	m.getOrCreateIMCSession(ctx, "user-1")
	m.getOrCreateIMCSession(ctx, "user-2")

	if len(m.imcSessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(m.imcSessions))
	}

	// Clear caches.
	m.clearCaches()

	// Verify SPC state cleared.
	if m.sysPromptHash != "" {
		t.Error("sysPromptHash not cleared")
	}
	if m.sysPromptTokens != 0 {
		t.Error("sysPromptTokens not cleared")
	}
	if m.sysPromptLen != 0 {
		t.Error("sysPromptLen not cleared")
	}

	// Verify IMC sessions cleared.
	if len(m.imcSessions) != 0 {
		t.Errorf("imcSessions not cleared, got %d", len(m.imcSessions))
	}
}

func TestCacheResultFields(t *testing.T) {
	// Test that cacheResult correctly propagates IMC fields.
	result := cacheResult{
		modifiedD: D{"test": "value"},
		prompt:    "test prompt",
		nPast:     1000,
		cached:    true,
		imcID:     "user-123",
		imcSeqID:  llama.SeqId(2),
	}

	if result.imcID != "user-123" {
		t.Errorf("imcID = %s, want user-123", result.imcID)
	}
	if result.imcSeqID != 2 {
		t.Errorf("imcSeqID = %d, want 2", result.imcSeqID)
	}
	if result.nPast != 1000 {
		t.Errorf("nPast = %d, want 1000", result.nPast)
	}
	if !result.cached {
		t.Error("cached should be true")
	}
}
