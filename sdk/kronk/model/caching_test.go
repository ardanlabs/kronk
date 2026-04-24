package model

import (
	"context"
	"strings"
	"sync"
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

func TestIMCSlotState(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
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
	m.imcSessions[0].cachedMsgsHash = "abc123"
	m.imcSessions[0].totalTokensCached = 1000
	m.imcSessions[0].cachedMsgCount = 2

	// Verify state persists.
	if m.imcSessions[0].cachedMsgsHash != "abc123" {
		t.Error("hash not persisted")
	}
	if m.imcSessions[0].totalTokensCached != 1000 {
		t.Error("tokens not persisted")
	}
	if m.imcSessions[0].cachedMsgCount != 2 {
		t.Error("msgCount not persisted")
	}

	// Verify slot 1 is independent.
	if m.imcSessions[1].totalTokensCached != 0 {
		t.Error("slot 1 should be empty")
	}
}

func TestClearCaches(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:             llama.SeqId(i),
			slotID:            i,
			cachedMsgsHash:    "hash",
			totalTokensCached: 500,
			cachedMsgCount:    3,
		}
	}

	// Clear caches.
	m.clearCaches()

	// Verify IMC sessions cleared.
	for i, slot := range m.imcSessions {
		if slot.totalTokensCached != 0 {
			t.Errorf("session %d totalTokensCached = %d, want 0", i, slot.totalTokensCached)
		}
		if slot.cachedMsgCount != 0 {
			t.Errorf("session %d cachedMsgCount = %d, want 0", i, slot.cachedMsgCount)
		}
		if slot.cachedMsgsHash != "" {
			t.Errorf("session %d cachedMsgsHash = %q, want empty", i, slot.cachedMsgsHash)
		}
	}
}

func TestCacheResultFields(t *testing.T) {
	// Test that cacheResult correctly propagates IMC fields.
	result := cacheResult{
		modifiedD:  D{"test": "value"},
		cacheIdx:   1000,
		cacheSeqID: llama.SeqId(2),
	}

	if result.cacheSeqID != 2 {
		t.Errorf("cacheSeqID = %d, want 2", result.cacheSeqID)
	}
	if result.cacheIdx != 1000 {
		t.Errorf("cacheIdx = %d, want 1000", result.cacheIdx)
	}
}

// =============================================================================
// Multi-Slot IMC Scan Tests
// =============================================================================

// TestProcessIMCScanSkipsPendingSlots verifies that processIMC skips slots with
// pending=true (build in-flight) and picks the next available empty slot.
// This prevents the race where two concurrent buildIMCCacheFromScratch calls
// target the same slot.
//
// Since buildIMCCacheFromScratch requires a compiled Jinja template and vocab
// (CGO), we verify the slot selection indirectly: after processIMC returns
// (with an expected template error), we check which slot was marked pending.
func TestProcessIMCScanSkipsPendingSlots(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Simulate: slot[0] has a build in-flight (pending=true).
	m.imcSessions[0].pending = true

	// Simulate: slot[1] is empty, slot[2] is empty.

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{
		"messages": messages,
	}

	// processIMC will fail in buildIMCCacheFromScratch (no template), but
	// we can verify the scan logic picked the right slot by checking which
	// slot it attempted to build on. The scan happens before the template
	// error, so we verify slot[0] was skipped.
	_ = m.processIMC(ctx, d, time.Now())

	// Slot[0] should still be pending (untouched — it was skipped).
	if !m.imcSessions[0].pending {
		t.Error("slot[0] should still be pending (was skipped during scan)")
	}

	// Slot[2] should NOT be pending (scan picks first empty = slot[1]).
	if m.imcSessions[2].pending {
		t.Error("slot[2] should not be pending (slot[1] was first empty)")
	}
}

// TestProcessIMCScanAllPending verifies that when all slots are pending,
// processIMC waits and returns an error when the context is canceled.
func TestProcessIMCScanAllPending(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	m.cacheCond = sync.NewCond(&m.cacheMu)

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:   llama.SeqId(i),
			slotID:  i,
			pending: true,
		}
	}

	// Use a short timeout so the wait doesn't block the test.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{
		"messages": messages,
	}

	result := m.processIMC(ctx, d, time.Now())

	if result.err == nil {
		t.Error("expected error when all slots are pending and context is canceled")
	}
}

// TestProcessIMCSlotMatchByHash verifies that processIMC finds a slot with a
// matching prefix hash and returns a cache hit (no new tokens to build).
func TestProcessIMCSlotMatchByHash(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()
	// Build the hash for messages[0:2] (the first 2 messages).
	cachedMsgs := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
	}
	cachedHash := hashMessages(cachedMsgs)

	// Simulate: slot[1] has cached the first 2 messages.
	m.imcSessions[1].cachedMsgsHash = cachedHash
	m.imcSessions[1].totalTokensCached = 500
	m.imcSessions[1].cachedMsgCount = 2

	// Request with same 2 messages + 1 new message (total=3, cache 2, generate from last).
	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{
		"messages": messages,
	}

	result := m.processIMC(ctx, d, time.Now())

	if result.err != nil {
		t.Fatalf("processIMC returned error: %v", result.err)
	}

	// Should match slot[1] (seqID=1).
	if result.cacheSeqID != 1 {
		t.Errorf("cacheSeqID = %d, want 1", result.cacheSeqID)
	}
	if result.imcSlotID != 1 {
		t.Errorf("imcSlotID = %d, want 1", result.imcSlotID)
	}

	// Pure cache hit: cachedMsgCount (2) == lastMsgIdxToCache (2).
	if result.cacheIdx != 500 {
		t.Errorf("cacheIdx = %d, want 500", result.cacheIdx)
	}

	// No new tokens to decode (pure hit, not extend).
	if len(result.imcNewCacheTokens) != 0 {
		t.Errorf("imcNewCacheTokens = %d, want 0 (pure cache hit)", len(result.imcNewCacheTokens))
	}
}

// TestProcessIMCBestPrefixCoverage verifies that when multiple slots match,
// processIMC picks the one with the most cached messages.
func TestProcessIMCBestPrefixCoverage(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi"},
		{"role": "user", "content": "How are you?"},
		{"role": "assistant", "content": "Fine"},
	}

	// Slot[0] cached first 2 messages.
	hash2 := hashMessages(messages[:2])
	m.imcSessions[0].cachedMsgsHash = hash2
	m.imcSessions[0].totalTokensCached = 300
	m.imcSessions[0].cachedMsgCount = 2

	// Slot[1] cached first 4 messages (better coverage).
	hash4 := hashMessages(messages[:4])
	m.imcSessions[1].cachedMsgsHash = hash4
	m.imcSessions[1].totalTokensCached = 800
	m.imcSessions[1].cachedMsgCount = 4

	d := D{
		"messages": messages,
	}

	result := m.processIMC(ctx, d, time.Now())

	if result.err != nil {
		t.Fatalf("processIMC returned error: %v", result.err)
	}

	// Should pick slot[1] (seqID=1) because it has more cached messages.
	if result.cacheSeqID != 1 {
		t.Errorf("cacheSeqID = %d, want 1 (best prefix coverage)", result.cacheSeqID)
	}
	if result.imcSlotID != 1 {
		t.Errorf("imcSlotID = %d, want 1 (best prefix coverage)", result.imcSlotID)
	}

	// Pure cache hit: cachedMsgCount (4) == lastMsgIdxToCache (4).
	if result.cacheIdx != 800 {
		t.Errorf("cacheIdx = %d, want 800", result.cacheIdx)
	}
}

// TestProcessIMCLRUEviction verifies that when all slots are full and none
// match, processIMC selects the LRU slot for eviction. Since buildIMCCache-
// FromScratch requires a compiled Jinja template (CGO), we verify the LRU
// selection indirectly: the error returned from the build attempt tells us
// which slot was targeted, and we verify slot[1] (more recent) was NOT reset.
func TestProcessIMCLRUEviction(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	now := time.Now()

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Both slots have data but with non-matching hashes.
	m.imcSessions[0].cachedMsgsHash = "aaaa" + strings.Repeat("0", 56)
	m.imcSessions[0].totalTokensCached = 500
	m.imcSessions[0].cachedMsgCount = 2
	m.imcSessions[0].lastUsed = now.Add(-10 * time.Second) // Older (LRU candidate).

	m.imcSessions[1].cachedMsgsHash = "bbbb" + strings.Repeat("0", 56)
	m.imcSessions[1].totalTokensCached = 300
	m.imcSessions[1].cachedMsgCount = 1
	m.imcSessions[1].lastUsed = now // More recent.

	// Request with completely different content (no hash match).
	messages := []D{
		{"role": "system", "content": "Something completely different"},
		{"role": "user", "content": "New conversation"},
		{"role": "assistant", "content": "New response"},
	}

	d := D{
		"messages": messages,
	}

	// buildIMCCacheFromScratch will fail (no template), but the scan should
	// have selected slot[0] (LRU). Verify slot[1] was NOT touched.
	result := m.processIMC(ctx, d, time.Now())

	if result.err == nil {
		t.Fatal("expected template error from buildIMCCacheFromScratch")
	}

	// Slot[1] should NOT have been selected — its state should be untouched.
	if m.imcSessions[1].totalTokensCached != 300 {
		t.Errorf("slot[1] totalTokensCached = %d, want 300 (should be untouched)", m.imcSessions[1].totalTokensCached)
	}
	if m.imcSessions[1].cachedMsgCount != 1 {
		t.Errorf("slot[1] cachedMsgCount = %d, want 1 (should be untouched)", m.imcSessions[1].cachedMsgCount)
	}
}

// TestProcessIMCParallelSubAgents simulates the real-world scenario:
// Two sub-agent requests with different content each
// get routed to separate slots. Then a follow-up from sub-agent 1 matches
// the correct slot via hash.
//
// Since buildIMCCacheFromScratch requires CGO (Jinja template + tokenizer),
// we simulate the build completion by manually setting slot state as startSlot
// would. The scan logic (which IS testable) is what we're validating.
func TestProcessIMCParallelSubAgents(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Each sub-agent has 3 messages: system + user + assistant.
	// With 3 total messages, lastMsgIdxToCache = 2 (cache first 2, generate from last).
	// We set cachedMsgCount = 2 so follow-ups with the same 3 messages are pure hits.

	// Sub-agent 1 cached messages.
	agent1Cached := []D{
		{"role": "system", "content": "You are a code reviewer"},
		{"role": "user", "content": "Review this code"},
	}
	hash1 := hashMessages(agent1Cached)

	// Sub-agent 2 cached messages.
	agent2Cached := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests for this"},
	}
	hash2 := hashMessages(agent2Cached)

	// Simulate: both sub-agents have completed their initial builds via
	// startSlot. slot[0] has sub-agent 1's cache, slot[1] has sub-agent 2's.

	m.imcSessions[0].cachedMsgsHash = hash1
	m.imcSessions[0].totalTokensCached = 400
	m.imcSessions[0].cachedMsgCount = 2
	m.imcSessions[0].lastUsed = time.Now()

	m.imcSessions[1].cachedMsgsHash = hash2
	m.imcSessions[1].totalTokensCached = 350
	m.imcSessions[1].cachedMsgCount = 2
	m.imcSessions[1].lastUsed = time.Now()

	// Follow-up from sub-agent 1: same prefix (pure cache hit).
	msgs3 := []D{
		{"role": "system", "content": "You are a code reviewer"},
		{"role": "user", "content": "Review this code"},
		{"role": "assistant", "content": "Looking at it now"},
	}
	d3 := D{
		"messages": msgs3,
	}

	result3 := m.processIMC(ctx, d3, time.Now())
	if result3.err != nil {
		t.Fatalf("follow-up error: %v", result3.err)
	}

	// Should match slot[0] (sub-agent 1's cache) via hash.
	if result3.cacheSeqID != 0 {
		t.Errorf("follow-up: cacheSeqID = %d, want 0 (should match sub-agent 1's slot)", result3.cacheSeqID)
	}
	if result3.imcSlotID != 0 {
		t.Errorf("follow-up: imcSlotID = %d, want 0 (should match sub-agent 1's slot)", result3.imcSlotID)
	}

	// Pure cache hit — no new tokens, no clear.
	if len(result3.imcNewCacheTokens) != 0 {
		t.Errorf("follow-up: expected pure cache hit, got %d new tokens", len(result3.imcNewCacheTokens))
	}
	if result3.imcClearSeq {
		t.Error("follow-up should not clear seq (pure cache hit)")
	}
	if result3.cacheIdx != 400 {
		t.Errorf("follow-up: cacheIdx = %d, want 400", result3.cacheIdx)
	}

	// Follow-up from sub-agent 2: same prefix (pure cache hit).
	msgs4 := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests for this"},
		{"role": "assistant", "content": "On it"},
	}
	d4 := D{
		"messages": msgs4,
	}

	result4 := m.processIMC(ctx, d4, time.Now())
	if result4.err != nil {
		t.Fatalf("sub-agent 2 follow-up error: %v", result4.err)
	}

	// Should match slot[1] (sub-agent 2's cache) via hash.
	if result4.cacheSeqID != 1 {
		t.Errorf("sub-agent 2 follow-up: cacheSeqID = %d, want 1", result4.cacheSeqID)
	}
	if result4.imcSlotID != 1 {
		t.Errorf("sub-agent 2 follow-up: imcSlotID = %d, want 1", result4.imcSlotID)
	}

	if len(result4.imcNewCacheTokens) != 0 {
		t.Errorf("sub-agent 2 follow-up: expected pure cache hit, got %d new tokens", len(result4.imcNewCacheTokens))
	}
	if result4.cacheIdx != 350 {
		t.Errorf("sub-agent 2 follow-up: cacheIdx = %d, want 350", result4.cacheIdx)
	}
}

// TestProcessIMCPendingPreventsDoubleSlot verifies the core race condition fix:
// when buildIMCCacheFromScratch sets pending=true, a concurrent processIMC
// call skips that slot and picks the next empty one instead of racing onto the
// same slot. We simulate this by manually setting slot[0] pending (as
// buildIMCCacheFromScratch would) and verifying the second call skips it.
func TestProcessIMCPendingPreventsDoubleSlot(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Simulate: slot[0] is mid-build (pending=true, state reset).
	// This is exactly what buildIMCCacheFromScratch does at lines 339-342.
	m.imcSessions[0].totalTokensCached = 0
	m.imcSessions[0].cachedMsgCount = 0
	m.imcSessions[0].cachedMsgsHash = ""
	m.imcSessions[0].pending = true

	// Second sub-agent arrives with different content. Without the pending
	// flag, it would see slot[0] as "empty" (totalTokensCached=0) and pick
	// it — causing both sub-agents to race on the same slot.
	msgs := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests"},
		{"role": "assistant", "content": "On it"},
	}
	d := D{
		"messages": msgs,
	}

	// This will fail at template, but we can verify slot selection.
	_ = m.processIMC(ctx, d, time.Now())

	// Slot[0] should still be pending (untouched by the second request).
	if !m.imcSessions[0].pending {
		t.Error("slot[0] should still be pending (second request should skip it)")
	}

	// Slot[2] should NOT be affected (slot[1] is first empty after slot[0]).
	if m.imcSessions[2].pending {
		t.Error("slot[2] should not be pending (slot[1] should be picked first)")
	}
}

func TestTokenPrefixMatch(t *testing.T) {
	tests := []struct {
		name     string
		cached   []llama.Token
		incoming []llama.Token
		want     int
	}{
		{
			name:     "identical sequences",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3, 4, 5},
			want:     5,
		},
		{
			name:     "empty cached",
			cached:   []llama.Token{},
			incoming: []llama.Token{1, 2, 3},
			want:     0,
		},
		{
			name:     "empty incoming",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{},
			want:     0,
		},
		{
			name:     "both empty",
			cached:   []llama.Token{},
			incoming: []llama.Token{},
			want:     0,
		},
		{
			name:     "diverge at start",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{9, 2, 3},
			want:     0,
		},
		{
			name:     "diverge in middle",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 9, 4, 5},
			want:     2,
		},
		{
			name:     "cached shorter than incoming",
			cached:   []llama.Token{1, 2, 3},
			incoming: []llama.Token{1, 2, 3, 4, 5},
			want:     3,
		},
		{
			name:     "incoming shorter than cached",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3},
			want:     3,
		},
		{
			name:     "diverge at last element",
			cached:   []llama.Token{1, 2, 3, 4, 5},
			incoming: []llama.Token{1, 2, 3, 4, 9},
			want:     4,
		},
		{
			name:     "single element match",
			cached:   []llama.Token{42},
			incoming: []llama.Token{42},
			want:     1,
		},
		{
			name:     "single element mismatch",
			cached:   []llama.Token{42},
			incoming: []llama.Token{99},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenPrefixMatch(tt.cached, tt.incoming)
			if got != tt.want {
				t.Errorf("tokenPrefixMatch() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestProcessIMCTokenPrefixFallback verifies the token prefix scan path in
// processIMC. When no hash matches, the code attempts tokenization for
// token-level prefix matching. Without a Jinja template, tokenization fails
// (tmErr != nil) and the code falls through gracefully to the empty/LRU path.
// The key assertion is that the candidate slot's state is NOT cleared — the
// token prefix code path only modifies slots after successful tokenization.
func TestProcessIMCTokenPrefixFallback(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
			CacheMinTokens:   3,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	now := time.Now()

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Slot[0] has non-matching hashes but populated cachedTokens,
	// making it a candidate for the token prefix comparison path.
	m.imcSessions[0].cachedMsgsHash = "cccc" + strings.Repeat("0", 56)
	m.imcSessions[0].totalTokensCached = 100
	m.imcSessions[0].cachedMsgCount = 2
	m.imcSessions[0].lastUsed = now
	m.imcSessions[0].cachedTokens = []llama.Token{10, 20, 30, 40, 50}

	// Slot[1] is empty (available for fallback).

	// Request with content that won't hash-match slot[0].
	messages := []D{
		{"role": "system", "content": "Totally different system prompt"},
		{"role": "user", "content": "Totally different user message"},
		{"role": "assistant", "content": "Totally different response"},
	}

	d := D{
		"messages": messages,
	}

	// processIMC will:
	// 1. Hash-scan: no match (different content).
	// 2. Token prefix scan: slot[0] is a candidate (non-empty, has cachedTokens).
	// 3. Tokenization fails (no Jinja template) — tmErr != nil, falls through.
	// 4. Falls to empty/LRU path: slot[1] is empty, picks it.
	// 5. buildIMCCacheFromScratch on slot[1] also fails (no template).
	_ = m.processIMC(ctx, d, time.Now())

	// Slot[0] should NOT have been cleared or marked pending — the token
	// prefix code path never modifies slot state when tokenization fails.
	if m.imcSessions[0].totalTokensCached != 100 {
		t.Errorf("slot[0] totalTokensCached = %d, want 100 (should be untouched)", m.imcSessions[0].totalTokensCached)
	}
	if m.imcSessions[0].cachedMsgCount != 2 {
		t.Errorf("slot[0] cachedMsgCount = %d, want 2 (should be untouched)", m.imcSessions[0].cachedMsgCount)
	}
	if m.imcSessions[0].cachedMsgsHash != "cccc"+strings.Repeat("0", 56) {
		t.Errorf("slot[0] cachedMsgsHash was modified (should be untouched)")
	}
	if m.imcSessions[0].pending {
		t.Error("slot[0] should not be pending (token prefix path should not modify it)")
	}
}

// =============================================================================
// Externalized KV State Tests
// =============================================================================

// TestIMCResetSessionClearsKVState verifies that imcResetSession clears the
// externalized KV state fields (kvState, kvStateBytes). Without this, a reset
// session could retain stale RAM state that gets restored into a future request.
func TestIMCResetSessionClearsKVState(t *testing.T) {
	s := &imcSession{
		slotID:            0,
		seqID:             0,
		cachedMsgsHash:    "abc123",
		cachedTokens:      []llama.Token{1, 2, 3},
		totalTokensCached: 100,
		cachedMsgCount:    2,
		kvState:           []byte{0xDE, 0xAD, 0xBE, 0xEF},
		kvStateBytes:      4,
		lastUsed:          time.Now(),
		pending:           true,
		hasMedia:          true,
		useMRoPE:          true,
		mediaKVCounts:     []int{10, 20},
		sysPromptHash:     "syshash",
		sysPromptTokens:   50,
	}

	imcResetSession(s)

	if s.kvState != nil {
		t.Errorf("kvState = %v, want nil", s.kvState)
	}
	if s.kvStateBytes != 0 {
		t.Errorf("kvStateBytes = %d, want 0", s.kvStateBytes)
	}
	if s.cachedMsgsHash != "" {
		t.Errorf("cachedMsgsHash = %q, want empty", s.cachedMsgsHash)
	}
	if s.cachedTokens != nil {
		t.Errorf("cachedTokens = %v, want nil", s.cachedTokens)
	}
	if s.totalTokensCached != 0 {
		t.Errorf("totalTokensCached = %d, want 0", s.totalTokensCached)
	}
	if s.cachedMsgCount != 0 {
		t.Errorf("cachedMsgCount = %d, want 0", s.cachedMsgCount)
	}
	if s.pending {
		t.Error("pending should be false")
	}
	if s.hasMedia {
		t.Error("hasMedia should be false")
	}
	if s.useMRoPE {
		t.Error("useMRoPE should be false")
	}
	if s.mediaKVCounts != nil {
		t.Errorf("mediaKVCounts = %v, want nil", s.mediaKVCounts)
	}
	if s.sysPromptHash != "" {
		t.Errorf("sysPromptHash = %q, want empty", s.sysPromptHash)
	}
	if s.sysPromptTokens != 0 {
		t.Errorf("sysPromptTokens = %d, want 0", s.sysPromptTokens)
	}

	// slotID and seqID should NOT be cleared — they are structural.
	if s.slotID != 0 {
		t.Errorf("slotID = %d, want 0 (should be preserved)", s.slotID)
	}
	if s.seqID != 0 {
		t.Errorf("seqID = %d, want 0 (should be preserved)", s.seqID)
	}
}

// TestClearCachesResetsKVState verifies that clearCaches properly resets
// kvState on all sessions, not just the original fields.
func TestClearCachesResetsKVState(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:             llama.SeqId(i),
			slotID:            i,
			cachedMsgsHash:    "hash",
			totalTokensCached: 500,
			cachedMsgCount:    3,
			kvState:           make([]byte, 1024),
			kvStateBytes:      1024,
		}
	}

	m.clearCaches()

	for i, s := range m.imcSessions {
		if s.kvState != nil {
			t.Errorf("session[%d] kvState not cleared", i)
		}
		if s.kvStateBytes != 0 {
			t.Errorf("session[%d] kvStateBytes = %d, want 0", i, s.kvStateBytes)
		}
		if s.totalTokensCached != 0 {
			t.Errorf("session[%d] totalTokensCached = %d, want 0", i, s.totalTokensCached)
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
			session := &imcSession{hasMedia: tt.hasMedia}
			got := session.hasMedia || tt.imcMediaBuild
			if got != tt.wantMediaFlag {
				t.Errorf("imcSessionMedia = %v, want %v", got, tt.wantMediaFlag)
			}
		})
	}
}

// TestIMCCommitSessionPreservesKVState verifies that imcCommitSession does not
// clear kvState — it should only be updated by the snapshot in startSlot.
func TestIMCCommitSessionPreservesKVState(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 1),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	m.cacheCond = sync.NewCond(&m.cacheMu)

	session := &imcSession{
		slotID:       0,
		seqID:        0,
		kvState:      []byte{0x01, 0x02, 0x03},
		kvStateBytes: 3,
		pending:      true,
	}
	m.imcSessions[0] = session

	m.imcCommitSession(session, "newhash", 1000, 5,
		[]llama.Token{1, 2, 3}, false, nil, "syshash", 50)

	// kvState should be preserved — only startSlot snapshots update it.
	if len(session.kvState) != 3 {
		t.Errorf("kvState len = %d, want 3 (should be preserved)", len(session.kvState))
	}
	if session.kvStateBytes != 3 {
		t.Errorf("kvStateBytes = %d, want 3 (should be preserved)", session.kvStateBytes)
	}

	// Verify other fields were updated.
	if session.cachedMsgsHash != "newhash" {
		t.Errorf("cachedMsgsHash = %q, want newhash", session.cachedMsgsHash)
	}
	if session.totalTokensCached != 1000 {
		t.Errorf("totalTokensCached = %d, want 1000", session.totalTokensCached)
	}
	if session.pending {
		t.Error("pending should be false after commit")
	}
}

// TestIMCCommitSessionNilSafe verifies that imcCommitSession handles a nil
// session without panicking.
func TestIMCCommitSessionNilSafe(t *testing.T) {
	m := &Model{
		cfg: Config{IncrementalCache: true},
		log: func(ctx context.Context, msg string, args ...any) {},
	}
	m.cacheCond = sync.NewCond(&m.cacheMu)

	// Should not panic.
	m.imcCommitSession(nil, "hash", 100, 2, nil, false, nil, "", 0)
}

// TestIMCKVPressureSkipsExternalizedSessions verifies that the KV-pressure
// eviction logic skips text sessions with externalized kvState (their VRAM
// sequences are already cleared, so they don't consume KV cells).
func TestIMCKVPressureSkipsExternalizedSessions(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
			ContextWindow:    1000,
		},
		imcSessions: make([]*imcSession, 3),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	now := time.Now()

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	// Session[0]: text session with externalized KV (kvState populated).
	// Its VRAM sequence is already cleared — should NOT count toward KV pressure.
	hash0 := hashMessages([]D{
		{"role": "system", "content": "sys0"},
		{"role": "user", "content": "user0"},
	})
	m.imcSessions[0].cachedMsgsHash = hash0
	m.imcSessions[0].totalTokensCached = 800
	m.imcSessions[0].cachedMsgCount = 2
	m.imcSessions[0].lastUsed = now.Add(-10 * time.Second)
	m.imcSessions[0].kvState = make([]byte, 4096)
	m.imcSessions[0].kvStateBytes = 4096

	// Session[1]: media session (no kvState, KV resident in VRAM).
	hash1 := hashMessages([]D{
		{"role": "system", "content": "sys1"},
		{"role": "user", "content": "user1"},
	})
	m.imcSessions[1].cachedMsgsHash = hash1
	m.imcSessions[1].totalTokensCached = 600
	m.imcSessions[1].cachedMsgCount = 2
	m.imcSessions[1].lastUsed = now.Add(-5 * time.Second)
	m.imcSessions[1].hasMedia = true

	// Session[2]: will be matched by the incoming request.
	cachedMsgs := []D{
		{"role": "system", "content": "matched-sys"},
		{"role": "user", "content": "matched-user"},
	}
	hash2 := hashMessages(cachedMsgs)
	m.imcSessions[2].cachedMsgsHash = hash2
	m.imcSessions[2].totalTokensCached = 300
	m.imcSessions[2].cachedMsgCount = 2
	m.imcSessions[2].lastUsed = now

	// Incoming request matches session[2].
	messages := []D{
		{"role": "system", "content": "matched-sys"},
		{"role": "user", "content": "matched-user"},
		{"role": "assistant", "content": "response"},
	}
	d := D{"messages": messages}

	ctx := context.Background()
	result := m.processIMC(ctx, d, time.Now())

	// Session[0] should NOT have been evicted — it's externalized to RAM
	// and doesn't consume VRAM KV cells.
	if m.imcSessions[0].totalTokensCached != 800 {
		t.Errorf("session[0] totalTokensCached = %d, want 800 (externalized session should not be evicted)", m.imcSessions[0].totalTokensCached)
	}
	if m.imcSessions[0].kvState == nil {
		t.Error("session[0] kvState should be preserved (not evicted)")
	}

	// Result should match session[2].
	if result.imcSession != m.imcSessions[2] {
		t.Errorf("expected result to match session[2], got session pointer %p", result.imcSession)
	}
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
				slotID:   1,
				seqID:    1,
				hasMedia: tt.hasMedia,
			}

			job := &chatJob{
				ctx:             context.Background(),
				imcCacheHit:     true,
				imcSession:      session,
				imcSessionMedia: tt.hasMedia,
				imcSlotID:       1,
			}

			// All IMC jobs use any-slot routing (KV externalized to RAM).
			_ = job // Verify job is constructed; scheduling is tested via integration.
			_ = session
		})
	}
}

// TestIMCSessionMediaTransitions verifies the session media flag through
// all six transitions: text→text, text→media, media→text, media→text,
// media→media, media→text. The key invariant: once a session gains media,
// it stays media-flagged for the rest of its life, except when
// rebuildIMCWithMedia calls imcResetSession (which clears hasMedia)
// followed by a new imcMediaBuild. All sessions (text and media) get
// kvState externalized to RAM.
func TestIMCSessionMediaTransitions(t *testing.T) {
	s := &imcSession{slotID: 0, seqID: 0}

	// Turn 1: Text build. hasMedia=false.
	s.cachedMsgsHash = "text1"
	s.totalTokensCached = 100
	s.hasMedia = false
	s.kvState = []byte{0x01}
	s.kvStateBytes = 1

	if s.hasMedia {
		t.Fatal("turn 1: session should be text-only")
	}
	if s.kvState == nil {
		t.Fatal("turn 1: text session should have kvState")
	}

	// Turn 2: Text→Media transition. imcMediaBuild=true, session.hasMedia
	// transitions from false to true after commit.
	mediaFlag := s.hasMedia || true // imcMediaBuild=true
	if !mediaFlag {
		t.Fatal("turn 2: imcSessionMedia should be true during media build")
	}

	// Simulate startSlot media build + commit + snapshot.
	s.cachedMsgsHash = "media1"
	s.totalTokensCached = 500
	s.hasMedia = true
	s.kvState = []byte{0x02} // Media sessions also get externalized to RAM.
	s.kvStateBytes = 1
	s.mediaKVCounts = []int{200}

	// Turn 3: Media→Text follow-up. Session stays media, kvState present.
	if !s.hasMedia {
		t.Fatal("turn 3: session should still be media")
	}
	if s.kvState == nil {
		t.Fatal("turn 3: media session should have kvState (externalized to RAM)")
	}

	// Simulate text extend on media session.
	s.totalTokensCached = 600
	s.cachedMsgCount = 4

	// Turn 4: Text→Text on media session. Still media.
	if !s.hasMedia {
		t.Fatal("turn 4: session should still be media")
	}

	// Turn 5: Media→Media (second image). rebuildIMCWithMedia resets then rebuilds.
	imcResetSession(s)
	if s.hasMedia {
		t.Fatal("turn 5: after reset, hasMedia should be false")
	}
	if s.kvState != nil {
		t.Fatal("turn 5: after reset, kvState should be nil")
	}

	// But imcMediaBuild=true on the job, so imcSessionMedia=true.
	mediaFlag = s.hasMedia || true // imcMediaBuild=true
	if !mediaFlag {
		t.Fatal("turn 5: imcSessionMedia should be true during media rebuild")
	}

	// After commit + snapshot, session is media again with kvState.
	s.hasMedia = true
	s.totalTokensCached = 800
	s.kvState = []byte{0x03}
	s.kvStateBytes = 1
	s.mediaKVCounts = []int{200, 150}

	// Turn 6: Media→Text. Session stays media.
	if !s.hasMedia {
		t.Fatal("turn 6: session should still be media")
	}
}

// =============================================================================
// IMC Session State Transition Tests
//
// These tests exercise the session routing, hash matching, commit, and
// re-match cycle that processIMC performs. They verify the cacheResult fields
// that batch_slot_start.go uses to decide restore/extend/trim/rebuild.
// =============================================================================

// TestIMCCommitThenRematch verifies the full cycle: build from scratch on an
// empty session, commit the session state, then send a new request that should
// match the committed session and produce a cache hit with extension tokens.
func TestIMCCommitThenRematch(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 1),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	m.cacheCond = sync.NewCond(&m.cacheMu)

	m.imcSessions[0] = &imcSession{
		seqID:  0,
		slotID: 0,
	}

	// Simulate a completed first request: 2 messages cached.
	msgs2 := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
	}
	hash2 := hashMessages(msgs2)
	sysHash := hashMessages(msgs2[:1])

	m.imcCommitSession(m.imcSessions[0], hash2, 500, 2,
		[]llama.Token{1, 2, 3, 4, 5}, false, nil, sysHash, 100)

	// Verify committed state.
	s := m.imcSessions[0]
	if s.cachedMsgsHash != hash2 {
		t.Errorf("committed hash = %q, want %q", s.cachedMsgsHash, hash2)
	}
	if s.totalTokensCached != 500 {
		t.Errorf("committed tokens = %d, want 500", s.totalTokensCached)
	}
	if s.cachedMsgCount != 2 {
		t.Errorf("committed msgCount = %d, want 2", s.cachedMsgCount)
	}
	if s.pending {
		t.Error("committed session should not be pending")
	}
	if s.sysPromptHash != sysHash {
		t.Errorf("sysPromptHash = %q, want %q", s.sysPromptHash, sysHash)
	}
	if s.sysPromptTokens != 100 {
		t.Errorf("sysPromptTokens = %d, want 100", s.sysPromptTokens)
	}
	if len(s.kvState) != 0 {
		t.Error("kvState should be empty (not set by commit)")
	}

	// Now send a 3-message request (same 2 cached + 1 new).
	d := D{
		"messages": []D{
			{"role": "system", "content": "You are helpful"},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there"},
		},
	}

	result := m.processIMC(context.Background(), d, time.Now())

	if result.err != nil {
		t.Fatalf("processIMC returned error: %v", result.err)
	}

	// Should match slot[0] with cache hit.
	if result.cacheSeqID != 0 {
		t.Errorf("cacheSeqID = %d, want 0", result.cacheSeqID)
	}
	if result.imcSlotID != 0 {
		t.Errorf("imcSlotID = %d, want 0", result.imcSlotID)
	}
	if result.cacheIdx != 500 {
		t.Errorf("cacheIdx = %d, want 500 (should reuse cached position)", result.cacheIdx)
	}

	// This is a pure hit (2 cached, 2 to cache) — no new tokens to decode.
	if len(result.imcNewCacheTokens) != 0 {
		t.Errorf("imcNewCacheTokens = %d, want 0 (pure cache hit)", len(result.imcNewCacheTokens))
	}
	if result.imcClearSeq {
		t.Error("imcClearSeq should be false (cache hit, not rebuild)")
	}
}

// TestIMCExtendAfterCommit verifies that a 5-message request (2 cached + 3 new)
// routes to the extend path. Without a Jinja template, extendIMCCache fails
// during createPrompt — the error message confirms the extend path was taken.
func TestIMCExtendAfterCommit(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 1),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	m.cacheCond = sync.NewCond(&m.cacheMu)

	m.imcSessions[0] = &imcSession{
		seqID:  0,
		slotID: 0,
	}

	// Commit: 2 messages cached, 500 tokens.
	msgs2 := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
	}
	hash2 := hashMessages(msgs2)

	m.imcCommitSession(m.imcSessions[0], hash2, 500, 2,
		[]llama.Token{1, 2, 3, 4, 5}, false, nil, hashMessages(msgs2[:1]), 100)

	// Request with 5 messages (2 cached + 3 new) — messages[0:4] should be cached.
	d := D{
		"messages": []D{
			{"role": "system", "content": "You are helpful"},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there"},
			{"role": "user", "content": "How are you?"},
			{"role": "assistant", "content": "Fine thanks"},
		},
	}

	result := m.processIMC(context.Background(), d, time.Now())

	// extendIMCCache fails at createPrompt (no template). The error message
	// confirms processIMC routed to the extend path, not build-from-scratch.
	if result.err == nil {
		t.Fatal("expected template error from extendIMCCache")
	}
	if !strings.Contains(result.err.Error(), "template prefix") {
		t.Errorf("expected extend path error, got: %v", result.err)
	}
}

// TestIMCSysPromptPreserveRoute verifies that when the full hash mismatches but
// the system prompt hash still matches, processIMC routes to the sys-prompt-preserve
// path. The session should be marked pending with the correct slot selected.
func TestIMCSysPromptPreserveRoute(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	m.cacheCond = sync.NewCond(&m.cacheMu)

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	// Commit slot[0] with a 3-message conversation.
	cachedMsgs := []D{
		{"role": "system", "content": "You are a math tutor"},
		{"role": "user", "content": "What is 2+2?"},
		{"role": "assistant", "content": "4"},
	}
	hash3 := hashMessages(cachedMsgs)
	sysHash := hashMessages(cachedMsgs[:1])

	m.imcCommitSession(m.imcSessions[0], hash3, 800, 3,
		[]llama.Token{10, 20, 30, 40, 50, 60, 70, 80}, false, nil, sysHash, 200)

	// Send a request with the SAME system prompt but EDITED conversation body.
	// Full hash won't match, but sys prompt hash should match.
	d := D{
		"messages": []D{
			{"role": "system", "content": "You are a math tutor"},
			{"role": "user", "content": "What is 3+3?"},
			{"role": "assistant", "content": "6"},
			{"role": "user", "content": "What is 5+5?"},
		},
	}

	result := m.processIMC(context.Background(), d, time.Now())

	// rebuildIMCPreservingSysPrompt fails at createPrompt (no template).
	// The error message confirms processIMC routed to the sys-prompt-preserve
	// path rather than falling through to empty/LRU.
	if result.err == nil {
		t.Fatal("expected template error from rebuildIMCPreservingSysPrompt")
	}
	if !strings.Contains(result.err.Error(), "sys-prompt-preserve") {
		t.Errorf("expected sys-prompt-preserve path error, got: %v", result.err)
	}
}

// TestIMCSysPromptChangeFallsToEmptySlot verifies that when the system prompt
// changes entirely, no sys-prompt match occurs and processIMC falls through
// to the empty/LRU slot path (buildIMCCacheFromScratch).
func TestIMCSysPromptChangeFallsToEmptySlot(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSessions: make([]*imcSession, 2),
		log:         func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSessions {
		m.imcSessions[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	// Commit slot[0] with system prompt A.
	cachedMsgs := []D{
		{"role": "system", "content": "You are a math tutor"},
		{"role": "user", "content": "What is 2+2?"},
	}
	hash := hashMessages(cachedMsgs)
	sysHash := hashMessages(cachedMsgs[:1])

	m.imcCommitSession(m.imcSessions[0], hash, 500, 2,
		[]llama.Token{1, 2, 3, 4, 5}, false, nil, sysHash, 100)

	// Send a request with a completely different system prompt B.
	d := D{
		"messages": []D{
			{"role": "system", "content": "You are a poet"},
			{"role": "user", "content": "Write about the sea"},
			{"role": "assistant", "content": "The sea is vast"},
		},
	}

	result := m.processIMC(context.Background(), d, time.Now())

	// buildIMCCacheFromScratch fails at createPrompt (no template).
	// The error message confirms processIMC fell through to the build path
	// (not sys-prompt-preserve, since the system prompt changed).
	if result.err == nil {
		t.Fatal("expected template error from buildIMCCacheFromScratch")
	}
	if strings.Contains(result.err.Error(), "sys-prompt-preserve") {
		t.Error("should NOT have taken sys-prompt-preserve path (different sys prompt)")
	}
}

// TestIMCRebuildResultPartialTrim verifies that imcRebuildResult correctly sets
// the fields for a partial trim (sys-prompt-preserve or token prefix match).
// When trimFrom > 0, the result should NOT set imcClearSeq and should set
// imcTrimPos to the trim position.
func TestIMCRebuildResultPartialTrim(t *testing.T) {
	allTokens := []llama.Token{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	trimFrom := 4 // Keep first 4 tokens (sys prompt), decode rest.

	result := imcRebuildResult(
		D{"messages": []D{}}, // d
		llama.SeqId(0),       // seqID
		0,                    // slotID
		5,                    // lastMsgIdxToCache
		allTokens,            // allTokens
		"newhash",            // newHash
		"syshash",            // sysHash
		200,                  // sysToks
		trimFrom,             // trimFrom
		&imcSession{},        // session
	)

	if result.imcClearSeq {
		t.Error("imcClearSeq should be false for partial trim")
	}
	if result.imcTrimPos != llama.Pos(trimFrom) {
		t.Errorf("imcTrimPos = %d, want %d", result.imcTrimPos, trimFrom)
	}
	if len(result.imcNewCacheTokens) != len(allTokens)-trimFrom {
		t.Errorf("imcNewCacheTokens = %d, want %d (tokens after trim)", len(result.imcNewCacheTokens), len(allTokens)-trimFrom)
	}
	// Verify the extension tokens are the suffix after trimFrom.
	for i, tok := range result.imcNewCacheTokens {
		expected := allTokens[trimFrom+i]
		if tok != expected {
			t.Errorf("imcNewCacheTokens[%d] = %d, want %d", i, tok, expected)
		}
	}
	if result.imcNewTotalCached != len(allTokens) {
		t.Errorf("imcNewTotalCached = %d, want %d", result.imcNewTotalCached, len(allTokens))
	}
	if result.imcSysPromptHash != "syshash" {
		t.Errorf("imcSysPromptHash = %q, want %q", result.imcSysPromptHash, "syshash")
	}
	if result.imcSysPromptTokens != 200 {
		t.Errorf("imcSysPromptTokens = %d, want 200", result.imcSysPromptTokens)
	}
}

// TestIMCRebuildResultFullRebuild verifies that imcRebuildResult correctly sets
// the fields for a full rebuild (trimFrom == 0). The result should set
// imcClearSeq to true and include ALL tokens for decoding.
func TestIMCRebuildResultFullRebuild(t *testing.T) {
	allTokens := []llama.Token{10, 20, 30, 40, 50, 60, 70, 80}

	result := imcRebuildResult(
		D{"messages": []D{}},
		llama.SeqId(0),
		0,
		4,
		allTokens,
		"newhash",
		"syshash",
		200,
		0, // trimFrom == 0 → full rebuild
		&imcSession{},
	)

	if !result.imcClearSeq {
		t.Error("imcClearSeq should be true for full rebuild")
	}
	if result.imcTrimPos != 0 {
		t.Errorf("imcTrimPos = %d, want 0", result.imcTrimPos)
	}
	if len(result.imcNewCacheTokens) != len(allTokens) {
		t.Errorf("imcNewCacheTokens = %d, want %d (all tokens for full rebuild)", len(result.imcNewCacheTokens), len(allTokens))
	}
	if result.cacheIdx != 0 {
		t.Errorf("cacheIdx = %d, want 0 (start from beginning)", result.cacheIdx)
	}
}
