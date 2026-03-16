package caching

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestIMCSlotState(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 2})

	// Verify slot initialization.
	if c.slots[0].seqID != 0 {
		t.Errorf("slot 0 seqID = %d, want 0", c.slots[0].seqID)
	}
	if c.slots[1].seqID != 1 {
		t.Errorf("slot 1 seqID = %d, want 1", c.slots[1].seqID)
	}

	// Simulate cache build on slot 0.
	c.slots[0].cachedMsgsHash = "abc123"
	c.slots[0].totalTokensCached = 1000
	c.slots[0].cachedMsgCount = 2

	// Verify state persists.
	if c.slots[0].cachedMsgsHash != "abc123" {
		t.Error("hash not persisted")
	}
	if c.slots[0].totalTokensCached != 1000 {
		t.Error("tokens not persisted")
	}
	if c.slots[0].cachedMsgCount != 2 {
		t.Error("msgCount not persisted")
	}

	// Verify slot 1 is independent.
	if c.slots[1].totalTokensCached != 0 {
		t.Error("slot 1 should be empty")
	}
}

func TestIMCClearCaches(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 2})

	for i, slot := range c.slots {
		slot.cachedMsgsHash = "hash"
		slot.totalTokensCached = 500
		slot.cachedMsgCount = 3
		_ = i
	}

	c.ClearCaches()

	for i, slot := range c.slots {
		if slot.totalTokensCached != 0 {
			t.Errorf("slot %d totalTokensCached = %d, want 0", i, slot.totalTokensCached)
		}
		if slot.cachedMsgCount != 0 {
			t.Errorf("slot %d cachedMsgCount = %d, want 0", i, slot.cachedMsgCount)
		}
		if slot.cachedMsgsHash != "" {
			t.Errorf("slot %d cachedMsgsHash = %q, want empty", i, slot.cachedMsgsHash)
		}
	}
}

func TestProcessIMCScanSkipsPendingSlots(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 3})
	ctx := context.Background()

	// Simulate: slot[0] has a build in-flight (pending=true).
	c.slots[0].pending = true

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{"messages": messages}

	_ = c.processIMC(ctx, d, time.Now())

	if !c.slots[0].pending {
		t.Error("slot[0] should still be pending (was skipped during scan)")
	}

	if c.slots[2].pending {
		t.Error("slot[2] should not be pending (slot[1] was first empty)")
	}
}

func TestProcessIMCScanAllPending(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 2, SlotTimeout: 100 * time.Millisecond})

	for _, slot := range c.slots {
		slot.pending = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{"messages": messages}

	result := c.processIMC(ctx, d, time.Now())

	if result.Err == nil {
		t.Error("expected error when all slots are pending and context is canceled")
	}
}

func TestProcessIMCSlotMatchByHash(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 3})
	ctx := context.Background()

	cachedMsgs := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
	}
	cachedHash := HashMessages(cachedMsgs)

	c.slots[1].cachedMsgsHash = cachedHash
	c.slots[1].totalTokensCached = 500
	c.slots[1].cachedMsgCount = 2

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there"},
	}

	d := D{"messages": messages}

	result := c.processIMC(ctx, d, time.Now())

	if result.Err != nil {
		t.Fatalf("processIMC returned error: %v", result.Err)
	}

	imc := result.IMC
	if imc == nil {
		t.Fatal("expected IMC result")
	}

	if imc.SeqID != 1 {
		t.Errorf("SeqID = %d, want 1", imc.SeqID)
	}
	if imc.SlotID != 1 {
		t.Errorf("SlotID = %d, want 1", imc.SlotID)
	}
	if imc.CacheIdx != 500 {
		t.Errorf("CacheIdx = %d, want 500", imc.CacheIdx)
	}
	if len(imc.NewCacheTokens) != 0 {
		t.Errorf("NewCacheTokens = %d, want 0 (pure cache hit)", len(imc.NewCacheTokens))
	}
}

func TestProcessIMCBestPrefixCoverage(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 3})
	ctx := context.Background()

	messages := []D{
		{"role": "system", "content": "You are helpful"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi"},
		{"role": "user", "content": "How are you?"},
		{"role": "assistant", "content": "Fine"},
	}

	hash2 := HashMessages(messages[:2])
	c.slots[0].cachedMsgsHash = hash2
	c.slots[0].totalTokensCached = 300
	c.slots[0].cachedMsgCount = 2

	hash4 := HashMessages(messages[:4])
	c.slots[1].cachedMsgsHash = hash4
	c.slots[1].totalTokensCached = 800
	c.slots[1].cachedMsgCount = 4

	d := D{"messages": messages}

	result := c.processIMC(ctx, d, time.Now())

	if result.Err != nil {
		t.Fatalf("processIMC returned error: %v", result.Err)
	}

	imc := result.IMC
	if imc == nil {
		t.Fatal("expected IMC result")
	}

	if imc.SeqID != 1 {
		t.Errorf("SeqID = %d, want 1 (best prefix coverage)", imc.SeqID)
	}
	if imc.SlotID != 1 {
		t.Errorf("SlotID = %d, want 1 (best prefix coverage)", imc.SlotID)
	}
	if imc.CacheIdx != 800 {
		t.Errorf("CacheIdx = %d, want 800", imc.CacheIdx)
	}
}

func TestProcessIMCLRUEviction(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 2})
	now := time.Now()
	ctx := context.Background()

	c.slots[0].cachedMsgsHash = "aaaa" + strings.Repeat("0", 56)
	c.slots[0].totalTokensCached = 500
	c.slots[0].cachedMsgCount = 2
	c.slots[0].lastUsed = now.Add(-10 * time.Second) // Older (LRU candidate).

	c.slots[1].cachedMsgsHash = "bbbb" + strings.Repeat("0", 56)
	c.slots[1].totalTokensCached = 300
	c.slots[1].cachedMsgCount = 1
	c.slots[1].lastUsed = now // More recent.

	messages := []D{
		{"role": "system", "content": "Something completely different"},
		{"role": "user", "content": "New conversation"},
		{"role": "assistant", "content": "New response"},
	}

	d := D{"messages": messages}

	result := c.processIMC(ctx, d, time.Now())

	if result.Err == nil {
		t.Fatal("expected template error from buildIMCCacheFromScratch")
	}

	if c.slots[1].totalTokensCached != 300 {
		t.Errorf("slot[1] totalTokensCached = %d, want 300 (should be untouched)", c.slots[1].totalTokensCached)
	}
	if c.slots[1].cachedMsgCount != 1 {
		t.Errorf("slot[1] cachedMsgCount = %d, want 1 (should be untouched)", c.slots[1].cachedMsgCount)
	}
}

func TestProcessIMCParallelSubAgents(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 3})
	ctx := context.Background()

	agent1Cached := []D{
		{"role": "system", "content": "You are a code reviewer"},
		{"role": "user", "content": "Review this code"},
	}
	hash1 := HashMessages(agent1Cached)

	agent2Cached := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests for this"},
	}
	hash2 := HashMessages(agent2Cached)

	c.slots[0].cachedMsgsHash = hash1
	c.slots[0].totalTokensCached = 400
	c.slots[0].cachedMsgCount = 2
	c.slots[0].lastUsed = time.Now()

	c.slots[1].cachedMsgsHash = hash2
	c.slots[1].totalTokensCached = 350
	c.slots[1].cachedMsgCount = 2
	c.slots[1].lastUsed = time.Now()

	// Follow-up from sub-agent 1.
	msgs3 := []D{
		{"role": "system", "content": "You are a code reviewer"},
		{"role": "user", "content": "Review this code"},
		{"role": "assistant", "content": "Looking at it now"},
	}
	d3 := D{"messages": msgs3}

	result3 := c.processIMC(ctx, d3, time.Now())
	if result3.Err != nil {
		t.Fatalf("follow-up error: %v", result3.Err)
	}

	imc3 := result3.IMC
	if imc3 == nil {
		t.Fatal("expected IMC result for follow-up")
	}

	if imc3.SeqID != 0 {
		t.Errorf("follow-up: SeqID = %d, want 0", imc3.SeqID)
	}
	if imc3.SlotID != 0 {
		t.Errorf("follow-up: SlotID = %d, want 0", imc3.SlotID)
	}
	if len(imc3.NewCacheTokens) != 0 {
		t.Errorf("follow-up: expected pure cache hit, got %d new tokens", len(imc3.NewCacheTokens))
	}
	if imc3.ClearSeq {
		t.Error("follow-up should not clear seq (pure cache hit)")
	}
	if imc3.CacheIdx != 400 {
		t.Errorf("follow-up: CacheIdx = %d, want 400", imc3.CacheIdx)
	}

	// Follow-up from sub-agent 2.
	msgs4 := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests for this"},
		{"role": "assistant", "content": "On it"},
	}
	d4 := D{"messages": msgs4}

	result4 := c.processIMC(ctx, d4, time.Now())
	if result4.Err != nil {
		t.Fatalf("sub-agent 2 follow-up error: %v", result4.Err)
	}

	imc4 := result4.IMC
	if imc4 == nil {
		t.Fatal("expected IMC result for sub-agent 2")
	}

	if imc4.SeqID != 1 {
		t.Errorf("sub-agent 2 follow-up: SeqID = %d, want 1", imc4.SeqID)
	}
	if imc4.SlotID != 1 {
		t.Errorf("sub-agent 2 follow-up: SlotID = %d, want 1", imc4.SlotID)
	}
	if len(imc4.NewCacheTokens) != 0 {
		t.Errorf("sub-agent 2 follow-up: expected pure cache hit, got %d new tokens", len(imc4.NewCacheTokens))
	}
	if imc4.CacheIdx != 350 {
		t.Errorf("sub-agent 2 follow-up: CacheIdx = %d, want 350", imc4.CacheIdx)
	}
}

func TestProcessIMCPendingPreventsDoubleSlot(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 3})
	ctx := context.Background()

	c.slots[0].totalTokensCached = 0
	c.slots[0].cachedMsgCount = 0
	c.slots[0].cachedMsgsHash = ""
	c.slots[0].pending = true

	msgs := []D{
		{"role": "system", "content": "You are a test writer"},
		{"role": "user", "content": "Write tests"},
		{"role": "assistant", "content": "On it"},
	}
	d := D{"messages": msgs}

	_ = c.processIMC(ctx, d, time.Now())

	if !c.slots[0].pending {
		t.Error("slot[0] should still be pending (second request should skip it)")
	}

	if c.slots[2].pending {
		t.Error("slot[2] should not be pending (slot[1] should be picked first)")
	}
}

func TestProcessIMCTokenPrefixFallback(t *testing.T) {
	c := NewIMCCache(&fakeDeps{}, Config{Strategy: StrategyIMC, NumSlots: 2, MinTokens: 3})
	now := time.Now()
	ctx := context.Background()

	c.slots[0].cachedMsgsHash = "cccc" + strings.Repeat("0", 56)
	c.slots[0].totalTokensCached = 100
	c.slots[0].cachedMsgCount = 2
	c.slots[0].lastUsed = now
	c.slots[0].cachedTokens = []llama.Token{10, 20, 30, 40, 50}

	messages := []D{
		{"role": "system", "content": "Totally different system prompt"},
		{"role": "user", "content": "Totally different user message"},
		{"role": "assistant", "content": "Totally different response"},
	}

	d := D{"messages": messages}

	_ = c.processIMC(ctx, d, time.Now())

	if c.slots[0].totalTokensCached != 100 {
		t.Errorf("slot[0] totalTokensCached = %d, want 100 (should be untouched)", c.slots[0].totalTokensCached)
	}
	if c.slots[0].cachedMsgCount != 2 {
		t.Errorf("slot[0] cachedMsgCount = %d, want 2 (should be untouched)", c.slots[0].cachedMsgCount)
	}
	if c.slots[0].cachedMsgsHash != "cccc"+strings.Repeat("0", 56) {
		t.Errorf("slot[0] cachedMsgsHash was modified (should be untouched)")
	}
	if c.slots[0].pending {
		t.Error("slot[0] should not be pending (token prefix path should not modify it)")
	}
}
