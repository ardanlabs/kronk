package model

import (
	"context"
	"strings"
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
		imcSlots: make([]*imcSession, 2),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	// Verify slot initialization.
	if m.imcSlots[0].seqID != 0 {
		t.Errorf("slot 0 seqID = %d, want 0", m.imcSlots[0].seqID)
	}
	if m.imcSlots[1].seqID != 1 {
		t.Errorf("slot 1 seqID = %d, want 1", m.imcSlots[1].seqID)
	}

	// Simulate cache build on slot 0.
	m.imcSlots[0].cachedMsgsHash = "abc123"
	m.imcSlots[0].totalTokensCached = 1000
	m.imcSlots[0].lastMsgIdxCached = 2

	// Verify state persists.
	if m.imcSlots[0].cachedMsgsHash != "abc123" {
		t.Error("hash not persisted")
	}
	if m.imcSlots[0].totalTokensCached != 1000 {
		t.Error("tokens not persisted")
	}
	if m.imcSlots[0].lastMsgIdxCached != 2 {
		t.Error("msgCount not persisted")
	}

	// Verify slot 1 is independent.
	if m.imcSlots[1].totalTokensCached != 0 {
		t.Error("slot 1 should be empty")
	}
}

func TestClearCaches(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache:  true,
			SystemPromptCache: true,
		},
		imcSlots:   make([]*imcSession, 2),
		spcSession: &spcSession{
			sysPromptHash:   "testhash",
			sysPromptTokens: 100,
			sysPromptLen:    50,
			seqID:           llama.SeqId(2),
		},
		log: func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:             llama.SeqId(i),
			slotID:            i,
			cachedMsgsHash:    "hash",
			totalTokensCached: 500,
			lastMsgIdxCached:  3,
		}
	}

	if m.spcSession == nil {
		t.Fatal("expected spcSession to be set before clearing")
	}

	// Clear caches.
	m.clearCaches()

	// Verify IMC slots cleared.
	for i, slot := range m.imcSlots {
		if slot.totalTokensCached != 0 {
			t.Errorf("slot %d totalTokensCached = %d, want 0", i, slot.totalTokensCached)
		}
		if slot.lastMsgIdxCached != 0 {
			t.Errorf("slot %d lastMsgIdxCached = %d, want 0", i, slot.lastMsgIdxCached)
		}
		if slot.cachedMsgsHash != "" {
			t.Errorf("slot %d cachedMsgsHash = %q, want empty", i, slot.cachedMsgsHash)
		}
	}

	// Verify SPC session cleared.
	if m.spcSession != nil {
		t.Errorf("spcSession not cleared, got %+v", m.spcSession)
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
		imcSlots: make([]*imcSession, 3),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Simulate: slot[0] has a build in-flight (pending=true).
	m.imcSlots[0].pending = true

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
	_ = m.processIMC(ctx, d)

	// Slot[0] should still be pending (untouched — it was skipped).
	if !m.imcSlots[0].pending {
		t.Error("slot[0] should still be pending (was skipped during scan)")
	}

	// Slot[2] should NOT be pending (scan picks first empty = slot[1]).
	if m.imcSlots[2].pending {
		t.Error("slot[2] should not be pending (slot[1] was first empty)")
	}
}

// TestProcessIMCScanAllPending verifies that when all slots are pending,
// processIMC returns an error (no slots available).
func TestProcessIMCScanAllPending(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSlots: make([]*imcSession, 2),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:   llama.SeqId(i),
			slotID:  i,
			pending: true,
		}
	}

	ctx := context.Background()

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{
		"messages": messages,
	}

	result := m.processIMC(ctx, d)

	if result.err == nil {
		t.Error("expected error when all slots are pending")
	}
}

// TestProcessIMCSlotMatchByHash verifies that processIMC finds a slot with a
// matching prefix hash and returns a cache hit (no new tokens to build).
func TestProcessIMCSlotMatchByHash(t *testing.T) {
	m := &Model{
		cfg: Config{
			IncrementalCache: true,
		},
		imcSlots: make([]*imcSession, 3),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
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
	m.imcSlots[1].cachedMsgsHash = cachedHash
	m.imcSlots[1].totalTokensCached = 500
	m.imcSlots[1].lastMsgIdxCached = 2

	// Request with same 2 messages + 1 new message (total=3, cache 2, generate from last).
	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{
		"messages": messages,
		
	}

	result := m.processIMC(ctx, d)

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

	// Pure cache hit: lastMsgIdxCached (2) == lastMsgIdxToCache (2).
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
		imcSlots: make([]*imcSession, 3),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
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
	m.imcSlots[0].cachedMsgsHash = hash2
	m.imcSlots[0].totalTokensCached = 300
	m.imcSlots[0].lastMsgIdxCached = 2

	// Slot[1] cached first 4 messages (better coverage).
	hash4 := hashMessages(messages[:4])
	m.imcSlots[1].cachedMsgsHash = hash4
	m.imcSlots[1].totalTokensCached = 800
	m.imcSlots[1].lastMsgIdxCached = 4

	d := D{
		"messages": messages,
		
	}

	result := m.processIMC(ctx, d)

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

	// Pure cache hit: lastMsgIdxCached (4) == lastMsgIdxToCache (4).
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
		imcSlots: make([]*imcSession, 2),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	now := time.Now()

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()
	

	// Both slots have data but with non-matching hashes.
	m.imcSlots[0].cachedMsgsHash = "aaaa" + strings.Repeat("0", 56)
	m.imcSlots[0].totalTokensCached = 500
	m.imcSlots[0].lastMsgIdxCached = 2
	m.imcSlots[0].lastUsed = now.Add(-10 * time.Second) // Older (LRU candidate).

	m.imcSlots[1].cachedMsgsHash = "bbbb" + strings.Repeat("0", 56)
	m.imcSlots[1].totalTokensCached = 300
	m.imcSlots[1].lastMsgIdxCached = 1
	m.imcSlots[1].lastUsed = now // More recent.

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
	result := m.processIMC(ctx, d)

	if result.err == nil {
		t.Fatal("expected template error from buildIMCCacheFromScratch")
	}

	// Slot[1] should NOT have been selected — its state should be untouched.
	if m.imcSlots[1].totalTokensCached != 300 {
		t.Errorf("slot[1] totalTokensCached = %d, want 300 (should be untouched)", m.imcSlots[1].totalTokensCached)
	}
	if m.imcSlots[1].lastMsgIdxCached != 1 {
		t.Errorf("slot[1] lastMsgIdxCached = %d, want 1 (should be untouched)", m.imcSlots[1].lastMsgIdxCached)
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
		imcSlots: make([]*imcSession, 3),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Each sub-agent has 3 messages: system + user + assistant.
	// With 3 total messages, lastMsgIdxToCache = 2 (cache first 2, generate from last).
	// We set lastMsgIdxCached = 2 so follow-ups with the same 3 messages are pure hits.

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
	

	m.imcSlots[0].cachedMsgsHash = hash1
	m.imcSlots[0].totalTokensCached = 400
	m.imcSlots[0].lastMsgIdxCached = 2
	m.imcSlots[0].lastUsed = time.Now()

	m.imcSlots[1].cachedMsgsHash = hash2
	m.imcSlots[1].totalTokensCached = 350
	m.imcSlots[1].lastMsgIdxCached = 2
	m.imcSlots[1].lastUsed = time.Now()

	// Follow-up from sub-agent 1: same prefix (pure cache hit).
	msgs3 := []D{
		{"role": "system", "content": "You are a code reviewer"},
		{"role": "user", "content": "Review this code"},
		{"role": "assistant", "content": "Looking at it now"},
	}
	d3 := D{
		"messages": msgs3,
		
	}

	result3 := m.processIMC(ctx, d3)
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

	result4 := m.processIMC(ctx, d4)
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
		imcSlots: make([]*imcSession, 3),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()
	

	// Simulate: slot[0] is mid-build (pending=true, state reset).
	// This is exactly what buildIMCCacheFromScratch does at lines 339-342.
	m.imcSlots[0].totalTokensCached = 0
	m.imcSlots[0].lastMsgIdxCached = 0
	m.imcSlots[0].cachedMsgsHash = ""
	m.imcSlots[0].pending = true

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
	_ = m.processIMC(ctx, d)

	// Slot[0] should still be pending (untouched by the second request).
	if !m.imcSlots[0].pending {
		t.Error("slot[0] should still be pending (second request should skip it)")
	}

	// Slot[2] should NOT be affected (slot[1] is first empty after slot[0]).
	if m.imcSlots[2].pending {
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
		imcSlots: make([]*imcSession, 2),
		log:      func(ctx context.Context, msg string, args ...any) {},
	}

	now := time.Now()

	for i := range m.imcSlots {
		m.imcSlots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	ctx := context.Background()

	// Slot[0] has non-matching hashes but populated cachedTokens,
	// making it a candidate for the token prefix comparison path.
	m.imcSlots[0].cachedMsgsHash = "cccc" + strings.Repeat("0", 56)
	m.imcSlots[0].totalTokensCached = 100
	m.imcSlots[0].lastMsgIdxCached = 2
	m.imcSlots[0].lastUsed = now
	m.imcSlots[0].cachedTokens = []llama.Token{10, 20, 30, 40, 50}

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
	_ = m.processIMC(ctx, d)

	// Slot[0] should NOT have been cleared or marked pending — the token
	// prefix code path never modifies slot state when tokenization fails.
	if m.imcSlots[0].totalTokensCached != 100 {
		t.Errorf("slot[0] totalTokensCached = %d, want 100 (should be untouched)", m.imcSlots[0].totalTokensCached)
	}
	if m.imcSlots[0].lastMsgIdxCached != 2 {
		t.Errorf("slot[0] lastMsgIdxCached = %d, want 2 (should be untouched)", m.imcSlots[0].lastMsgIdxCached)
	}
	if m.imcSlots[0].cachedMsgsHash != "cccc"+strings.Repeat("0", 56) {
		t.Errorf("slot[0] cachedMsgsHash was modified (should be untouched)")
	}
	if m.imcSlots[0].pending {
		t.Error("slot[0] should not be pending (token prefix path should not modify it)")
	}
}
