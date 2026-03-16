package caching

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// =============================================================================
// Incremental Message Cache (IMC) — Core Algorithm
//
// IMC has two matching strategies, automatically selected based on template
// behavior:
//
//   - Deterministic:     Hash-based prefix matching for models with consistent
//     templates. Fastest path. Used by most models.
//   - Non-Deterministic: Token-level prefix fallback for models with variable
//     templates (GPT-OSS, GLM). Activated when hash matching fails.
//
// The matching strategy is independent of the model type (Dense, MoE, Hybrid).
// All model types use the same caching functions. What changes per model type
// is how the batch engine manages state between requests.
// =============================================================================

// imcSession holds the state for a single IMC session.
type imcSession struct {
	cachedMsgsHash    string
	cachedTokens      []llama.Token
	totalTokensCached int
	cachedMsgCount    int
	seqID             llama.SeqId
	slotID            int
	lastUsed          time.Time
	pending           bool
	hasMedia          bool
	useMRoPE          bool
	mediaKVCounts     []int
}

func (s *imcSession) invalidate() {
	s.cachedMsgsHash = ""
	s.cachedTokens = nil
	s.totalTokensCached = 0
	s.cachedMsgCount = 0
	s.pending = false
	s.hasMedia = false
	s.useMRoPE = false
	s.mediaKVCounts = nil
}

func (s *imcSession) resetAll() {
	s.invalidate()
	s.lastUsed = time.Time{}
}

// imcSlotSnapshot holds a point-in-time copy of an imcSession's metadata.
type imcSlotSnapshot struct {
	slotID            int
	seqID             llama.SeqId
	cachedMsgsHash    string
	cachedTokens      []llama.Token
	totalTokensCached int
	cachedMsgCount    int
	lastUsed          time.Time
	pending           bool
	empty             bool
	hasMedia          bool
}

// IMCCache implements Cacher using Incremental Message Caching.
type IMCCache struct {
	deps  Deps
	cfg   Config
	mu    sync.RWMutex
	cond  *sync.Cond
	slots []*imcSession
}

// NewIMCCache creates a new Incremental Message Cache.
func NewIMCCache(deps Deps, cfg Config) *IMCCache {
	c := &IMCCache{
		deps:  deps,
		cfg:   cfg,
		slots: make([]*imcSession, cfg.NumSlots),
	}
	c.cond = sync.NewCond(&c.mu)

	for i := range c.slots {
		c.slots[i] = &imcSession{
			seqID:  llama.SeqId(i),
			slotID: i,
		}
	}

	return c
}

// ProcessCache implements Cacher.
func (c *IMCCache) ProcessCache(ctx context.Context, d D, requestStart time.Time) Result {
	return c.processIMC(ctx, d, requestStart)
}

// ClearCaches resets all cached state.
func (c *IMCCache) ClearCaches() {
	c.mu.Lock()
	for _, slot := range c.slots {
		slot.resetAll()
	}
	c.mu.Unlock()
	c.notifySlotAvailable()
}

// ClearPending clears a slot's pending flag and notifies waiters.
func (c *IMCCache) ClearPending(slotID int) {
	c.mu.Lock()
	if slotID < len(c.slots) {
		c.slots[slotID].pending = false
	}
	c.mu.Unlock()
	c.notifySlotAvailable()
}

// CommitSession updates a slot's session metadata after a successful
// cache build/extend/rebuild and clears the pending flag.
func (c *IMCCache) CommitSession(commit Commit) {
	c.mu.Lock()
	if commit.SlotID < len(c.slots) {
		slot := c.slots[commit.SlotID]
		slot.cachedMsgsHash = commit.Hash
		slot.totalTokensCached = commit.TotalCached
		slot.cachedMsgCount = commit.CachedMsgCount
		slot.lastUsed = time.Now()
		slot.pending = false
		slot.hasMedia = commit.HasMedia
		if len(commit.MediaKVCounts) > 0 {
			slot.mediaKVCounts = slices.Clone(commit.MediaKVCounts)
		} else {
			slot.mediaKVCounts = nil
		}
		if !commit.HasMedia {
			slot.useMRoPE = false
		}
		if commit.UseMRoPE {
			slot.useMRoPE = true
		}
		switch {
		case commit.HasMedia:
			slot.cachedTokens = nil
		case len(commit.CachedTokens) > 0:
			slot.cachedTokens = slices.Clone(commit.CachedTokens)
		}
	}
	c.mu.Unlock()
	c.notifySlotAvailable()
}

// InvalidateSlot clears all cached data for a slot.
func (c *IMCCache) InvalidateSlot(slotID int) {
	c.mu.Lock()
	if slotID < len(c.slots) {
		c.slots[slotID].invalidate()
	}
	c.mu.Unlock()
	c.notifySlotAvailable()
}

// SnapshotSlot returns a point-in-time copy of a slot's metadata.
func (c *IMCCache) SnapshotSlot(slotID int) (SlotSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if slotID >= len(c.slots) {
		return SlotSnapshot{}, false
	}

	slot := c.slots[slotID]
	return SlotSnapshot{
		SlotID:            slot.slotID,
		SeqID:             slot.seqID,
		CachedMsgsHash:    slot.cachedMsgsHash,
		CachedTokens:      slices.Clone(slot.cachedTokens),
		TotalTokensCached: slot.totalTokensCached,
		CachedMsgCount:    slot.cachedMsgCount,
		Pending:           slot.pending,
		Empty:             slot.totalTokensCached == 0,
		HasMedia:          slot.hasMedia,
		UseMRoPE:          slot.useMRoPE,
		MediaKVCounts:     slices.Clone(slot.mediaKVCounts),
	}, true
}

// RestoreSPCToSeq returns an error since IMC doesn't use SPC.
func (c *IMCCache) RestoreSPCToSeq(_ llama.SeqId) error {
	return fmt.Errorf("imc-cache: SPC restore not supported")
}

// HasCachedSlot returns true if the given slot has cached content.
func (c *IMCCache) HasCachedSlot(slotID int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if slotID >= len(c.slots) {
		return false
	}
	return c.slots[slotID].totalTokensCached > 0 || c.slots[slotID].pending
}

// SetSlotMRoPE sets the M-RoPE flag for a slot.
func (c *IMCCache) SetSlotMRoPE(slotID int, useMRoPE bool) {
	c.mu.Lock()
	if slotID < len(c.slots) {
		c.slots[slotID].useMRoPE = useMRoPE
	}
	c.mu.Unlock()
}

func (c *IMCCache) notifySlotAvailable() {
	if c.cond != nil {
		c.cond.Broadcast()
	}
}

// =============================================================================
// processIMC — Core Algorithm
// =============================================================================

func (c *IMCCache) processIMC(ctx context.Context, d D, requestStart time.Time) Result {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return Result{ModifiedD: d}
	}

	totalMsgs := len(messages)
	if totalMsgs < 2 {
		return Result{ModifiedD: d}
	}

	lastMsgIdxToCache := totalMsgs - 1

	c.deps.Log(ctx, "imc", "status", "scanning slots", "total-msgs", totalMsgs, "msgs-to-cache", lastMsgIdxToCache, "total-slots", len(c.slots))

	c.mu.RLock()

	snapshots := make([]imcSlotSnapshot, len(c.slots))
	for i, slot := range c.slots {
		snapshots[i] = imcSlotSnapshot{
			slotID:            slot.slotID,
			seqID:             slot.seqID,
			cachedMsgsHash:    slot.cachedMsgsHash,
			cachedTokens:      slot.cachedTokens,
			totalTokensCached: slot.totalTokensCached,
			cachedMsgCount:    slot.cachedMsgCount,
			lastUsed:          slot.lastUsed,
			pending:           slot.pending,
			empty:             slot.totalTokensCached == 0,
			hasMedia:          slot.hasMedia,
		}
	}

	c.mu.RUnlock()

	// -------------------------------------------------------------------------
	// Step 1: Hash-based slot scan (Deterministic path).

	var bestSlot *imcSession
	var bestCachedMsgsHash string
	var bestTotalTokensCached int
	var bestCachedMsgCount int
	var emptySlots []*imcSession
	var lruSlot *imcSession

	for i, snap := range snapshots {
		if snap.pending {
			c.deps.Log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] pending (build in-flight)", snap.slotID))
			continue
		}

		if snap.empty {
			c.deps.Log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] empty", snap.slotID))
			emptySlots = append(emptySlots, c.slots[i])
			continue
		}

		if lruSlot == nil || snap.lastUsed.Before(snapshots[lruSlot.slotID].lastUsed) {
			lruSlot = c.slots[i]
		}

		if totalMsgs <= snap.cachedMsgCount {
			c.deps.Log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] skip (cached-msgs[%d] >= total-msgs[%d])", snap.slotID, snap.cachedMsgCount, totalMsgs))
			continue
		}

		prefixHash := HashMessages(messages[:snap.cachedMsgCount])
		if prefixHash != snap.cachedMsgsHash {
			c.deps.Log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] mismatch (cached-msgs[%d] tokens[%d] hash[%s..] != [%s..])",
				snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8], prefixHash[:8]))
			continue
		}

		c.deps.Log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] MATCH (cached-msgs[%d] tokens[%d] hash[%s..])",
			snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8]))

		if bestSlot == nil || snap.cachedMsgCount > bestCachedMsgCount {
			bestSlot = c.slots[i]
			bestCachedMsgsHash = snap.cachedMsgsHash
			bestTotalTokensCached = snap.totalTokensCached
			bestCachedMsgCount = snap.cachedMsgCount
		}
	}

	// -------------------------------------------------------------------------
	// Step 2: Handle matched slot — extend or pure hit.

	if bestSlot != nil {
		c.deps.Log(ctx, "imc", "status", "slot matched", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestCachedMsgCount, "cached-tokens", bestTotalTokensCached, "msgs-to-cache", lastMsgIdxToCache)

		if bestCachedMsgCount < lastMsgIdxToCache {
			return c.extendIMCCache(ctx, d, messages, bestSlot, bestCachedMsgCount, lastMsgIdxToCache, bestTotalTokensCached)
		}

		c.deps.Log(ctx, "imc", "status", "cache hit", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestCachedMsgCount, "current-total-tokens-cached", bestTotalTokensCached,
			"hash", bestCachedMsgsHash[:8])

		return Result{
			ModifiedD: RemoveFirstNMessages(d, bestCachedMsgCount),
			IMC: &IMCResult{
				SlotID:         bestSlot.slotID,
				SeqID:          bestSlot.seqID,
				CacheIdx:       llama.Pos(bestTotalTokensCached),
				ExpectedHash:   bestCachedMsgsHash,
				CachedMsgCount: bestCachedMsgCount,
			},
		}
	}

	// -------------------------------------------------------------------------
	// Step 3: Token prefix fallback (Non-Deterministic mode).

	c.deps.Log(ctx, "imc", "status", "no slot matched, trying token prefix match", "total-msgs", totalMsgs)

	requestHasMedia := slices.ContainsFunc(messages[:lastMsgIdxToCache], MessageHasMedia)

	var tokenMatchCandidates []int
	if !requestHasMedia {
		for i, snap := range snapshots {
			if !snap.pending && !snap.empty && !snap.hasMedia && len(snap.cachedTokens) > 0 && totalMsgs > snap.cachedMsgCount {
				tokenMatchCandidates = append(tokenMatchCandidates, i)
			}
		}
	}

	if len(tokenMatchCandidates) > 0 {
		msgs := messages[:lastMsgIdxToCache]

		tokenMatchD := D{
			"messages":              msgs,
			"add_generation_prompt": false,
		}

		if tools, ok := d["tools"]; ok {
			tokenMatchD["tools"] = tools
		}

		tokenMatchPrompt, _, tmErr := c.deps.CreatePrompt(ctx, tokenMatchD)
		if tmErr == nil {
			_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-prefix-match",
				attribute.String("cache-type", "imc-prefix-match"),
			)

			incomingTokens := c.deps.TokenizeString(tokenMatchPrompt)

			tokenSpan.SetAttributes(attribute.Int("tokens", len(incomingTokens)))
			tokenSpan.End()

			var bestPartialSlotIdx int
			var bestPartialLen int

			for _, idx := range tokenMatchCandidates {
				snap := snapshots[idx]
				commonLen := TokenPrefixMatch(snap.cachedTokens, incomingTokens)

				pct := 0
				if snap.totalTokensCached > 0 {
					pct = commonLen * 100 / snap.totalTokensCached
				}

				c.deps.Log(ctx, "imc", "token-match", fmt.Sprintf("slot[%d] common-prefix %d/%d tokens (%d%% salvageable)",
					snap.slotID, commonLen, snap.totalTokensCached, pct))

				if commonLen > bestPartialLen {
					bestPartialLen = commonLen
					bestPartialSlotIdx = idx
				}
			}

			if bestPartialLen >= c.cfg.MinTokens {
				partialSlot := c.slots[bestPartialSlotIdx]
				discarded := snapshots[bestPartialSlotIdx].totalTokensCached - bestPartialLen
				saved := len(incomingTokens) - bestPartialLen

				c.deps.Log(ctx, "imc", "status", "token prefix match found",
					"slot", partialSlot.slotID,
					"common-prefix", bestPartialLen,
					"discarded-cached", discarded,
					"new-tokens-to-decode", saved,
					"total-incoming", len(incomingTokens))

				return c.rebuildIMCFromPartialPrefix(ctx, d, messages, partialSlot, lastMsgIdxToCache, incomingTokens, bestPartialLen)
			}

			c.deps.Log(ctx, "imc", "status", "no usable token prefix match",
				"best-prefix", bestPartialLen, "min-required", c.cfg.MinTokens)
		}
	}

	// -------------------------------------------------------------------------
	// Step 4: No match — pick an empty slot or evict LRU.

	for _, slot := range emptySlots {
		c.deps.Log(ctx, "imc", "status", "trying empty slot", "slot", slot.slotID)

		result := c.buildIMCCacheFromScratch(ctx, d, messages, slot, lastMsgIdxToCache)
		if result.IMC == nil || !result.IMC.Pending {
			return result
		}

		c.deps.Log(ctx, "imc", "status", "empty slot pending, trying next", "slot", slot.slotID)
	}

	if lruSlot != nil {
		c.deps.Log(ctx, "imc", "status", "evicting LRU slot", "slot", lruSlot.slotID,
			"evicted-msgs", lruSlot.cachedMsgCount, "evicted-tokens", lruSlot.totalTokensCached)

		return c.buildIMCCacheFromScratch(ctx, d, messages, lruSlot, lastMsgIdxToCache)
	}

	c.deps.Log(ctx, "imc", "status", "all slots pending, waiting for slot")

	if err := c.waitForIMCSlot(ctx, requestStart); err != nil {
		return Result{ModifiedD: d, Err: err}
	}

	c.deps.Log(ctx, "imc", "status", "slot became available, retrying scan")

	return c.processIMC(ctx, d, requestStart)
}

// =============================================================================
// extendIMCCache
// =============================================================================

func (c *IMCCache) extendIMCCache(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int) Result {

	if c.cfg.SupportsMedia {
		newMsgsHaveMedia := false
		for i := currentCachedMsgCount; i < lastMsgIdxToCache; i++ {
			if MessageHasMedia(messages[i]) {
				newMsgsHaveMedia = true
				break
			}
		}

		if newMsgsHaveMedia {
			c.mu.RLock()
			slotHasMedia := session.hasMedia
			c.mu.RUnlock()

			if slotHasMedia {
				c.deps.Log(ctx, "imc", "status", "extend requires media rebuild (new media, slot has media)",
					"slot", session.slotID, "cached-msgs", currentCachedMsgCount,
					"target-msgs", lastMsgIdxToCache)
				return c.rebuildIMCWithMedia(ctx, d, messages, session, lastMsgIdxToCache)
			}

			return c.extendIMCTextCacheWithMedia(ctx, d, messages, session, lastMsgIdxToCache, currentTotalTokensCached)
		}

		c.mu.RLock()
		slotHasMedia := session.hasMedia
		slotMediaKVCounts := session.mediaKVCounts
		c.mu.RUnlock()

		if slotHasMedia {
			return c.extendIMCMediaSlotWithText(ctx, d, messages, session,
				currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached,
				slotMediaKVCounts)
		}
	}

	c.mu.Lock()

	if session.cachedMsgCount != currentCachedMsgCount || session.totalTokensCached != currentTotalTokensCached {
		c.deps.Log(ctx, "imc", "status", "extend fallback (state changed)", "slot", session.slotID,
			"expected-msgs", currentCachedMsgCount, "actual-msgs", session.cachedMsgCount,
			"expected-tokens", currentTotalTokensCached, "actual-tokens", session.totalTokensCached)
		c.mu.Unlock()
		return c.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	if session.pending {
		c.deps.Log(ctx, "imc", "status", "extend fallback (slot pending)", "slot", session.slotID)
		c.mu.Unlock()
		return c.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID
	currentHash := session.cachedMsgsHash

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending (extend)", "slot", slotID, "seq", seqID)

	msgs := messages[:lastMsgIdxToCache]

	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}

	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	promptToCache, _, err := c.deps.CreatePrompt(ctx, msgsToCache)
	if err != nil {
		c.ClearPending(slotID)
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: failed to template prefix: %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-extend",
		attribute.String("cache-type", "imc-extend"),
	)

	allTokens := c.deps.TokenizeString(promptToCache)
	totalTokens := len(allTokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", totalTokens))
	tokenSpan.End()

	if totalTokens <= currentTotalTokensCached {
		c.deps.Log(ctx, "imc", "status", "extend (no new tokens)", "slot", slotID, "cached", currentTotalTokensCached, "total", totalTokens)
		c.ClearPending(slotID)

		return Result{
			ModifiedD: RemoveFirstNMessages(d, currentCachedMsgCount),
			IMC: &IMCResult{
				SlotID:         slotID,
				SeqID:          seqID,
				CacheIdx:       llama.Pos(currentTotalTokensCached),
				ExpectedHash:   currentHash,
				CachedMsgCount: currentCachedMsgCount,
			},
		}
	}

	extensionTokens := allTokens[currentTotalTokensCached:]

	c.deps.Log(ctx, "imc", "status", "extending cache (deferred)", "slot", slotID, "new-tokens", len(extensionTokens))

	newHash := HashMessages(msgs)

	c.deps.Log(ctx, "imc", "status", "cache extend prepared", "slot", slotID, "seq", seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentCachedMsgCount, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, len(extensionTokens)))

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:            slotID,
			SeqID:             seqID,
			CacheIdx:          llama.Pos(currentTotalTokensCached),
			ExpectedHash:      newHash,
			CachedMsgCount:    lastMsgIdxToCache,
			NewCacheTokens:    extensionTokens,
			NewTotalCached:    totalTokens,
			NewCachedMsgCount: lastMsgIdxToCache,
			NewMsgsHash:       newHash,
			NewCachedTokens:   allTokens,
		},
	}
}

// =============================================================================
// Media extension helpers
// =============================================================================

func (c *IMCCache) extendIMCMediaSlotWithText(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int, mediaKVCounts []int) Result {
	c.mu.Lock()

	if session.cachedMsgCount != currentCachedMsgCount || session.totalTokensCached != currentTotalTokensCached {
		c.mu.Unlock()
		return c.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	if session.pending {
		c.mu.Unlock()
		return c.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending (media text extend)", "slot", slotID, "seq", seqID)

	mediaMarkerTokens := c.deps.MediaMarkerTokens(ctx)

	var totalMediaKV int
	for _, kv := range mediaKVCounts {
		totalMediaKV += kv
	}
	totalMarkerTokens := len(mediaKVCounts) * mediaMarkerTokens
	kvTokenDelta := totalMediaKV - totalMarkerTokens
	cachedTextTokens := currentTotalTokensCached - kvTokenDelta

	msgs := messages[:lastMsgIdxToCache]
	newHash := HashMessages(msgs)

	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	promptToCache, _, err := c.deps.CreatePrompt(ctx, msgsToCache)
	if err != nil {
		c.ClearPending(slotID)
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: failed to template prefix (media text extend): %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-media-text-extend",
		attribute.String("cache-type", "imc-media-text-extend"),
	)

	allTokens := c.deps.TokenizeString(promptToCache)
	totalTextTokens := len(allTokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", totalTextTokens))
	tokenSpan.End()

	if totalTextTokens <= cachedTextTokens {
		c.deps.Log(ctx, "imc", "status", "media text extend (no new tokens)",
			"slot", slotID, "cached_text_tokens", cachedTextTokens, "total_text_tokens", totalTextTokens)

		c.ClearPending(slotID)

		return Result{
			ModifiedD: RemoveFirstNMessages(d, currentCachedMsgCount),
			IMC: &IMCResult{
				SlotID:         slotID,
				SeqID:          seqID,
				CacheIdx:       llama.Pos(currentTotalTokensCached),
				ExpectedHash:   session.cachedMsgsHash,
				CachedMsgCount: currentCachedMsgCount,
			},
		}
	}

	extensionTokens := allTokens[cachedTextTokens:]
	newTotalCached := currentTotalTokensCached + len(extensionTokens)

	c.deps.Log(ctx, "imc", "status", "media text extend prepared", "slot", slotID, "seq", seqID,
		"cached_kv", currentTotalTokensCached, "cached_text_tokens", cachedTextTokens,
		"kv_token_delta", kvTokenDelta, "extension_tokens", len(extensionTokens),
		"new_total_kv", newTotalCached)

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:            slotID,
			SeqID:             seqID,
			CacheIdx:          llama.Pos(currentTotalTokensCached),
			ExpectedHash:      newHash,
			CachedMsgCount:    lastMsgIdxToCache,
			NewCacheTokens:    extensionTokens,
			NewTotalCached:    newTotalCached,
			NewCachedMsgCount: lastMsgIdxToCache,
			NewMsgsHash:       newHash,
			MediaKVCounts:     mediaKVCounts,
		},
	}
}

func (c *IMCCache) extendIMCTextCacheWithMedia(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache, currentTotalTokensCached int) Result {
	c.mu.Lock()

	if session.pending {
		c.mu.Unlock()
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending (media extend from text)",
		"slot", slotID, "seq", seqID, "skip_text_tokens", currentTotalTokensCached)

	msgsToCache := messages[:lastMsgIdxToCache]
	newHash := HashMessages(msgsToCache)

	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	c.deps.Log(ctx, "imc", "status", "media extend prepared", "slot", slotID, "seq", seqID,
		"msgs", lastMsgIdxToCache, "hash", newHash[:8], "skip_text_tokens", currentTotalTokensCached)

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:              slotID,
			SeqID:               seqID,
			CacheIdx:            0,
			ExpectedHash:        newHash,
			CachedMsgCount:      lastMsgIdxToCache,
			NewCachedMsgCount:   lastMsgIdxToCache,
			NewMsgsHash:         newHash,
			MediaBuild:          true,
			MediaCacheD:         prefixD,
			MediaSkipTextTokens: currentTotalTokensCached,
		},
	}
}

// =============================================================================
// buildIMCCacheFromScratch
// =============================================================================

func (c *IMCCache) buildIMCCacheFromScratch(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) Result {
	c.mu.Lock()

	if session.cachedMsgCount > 0 && session.totalTokensCached > 0 && session.cachedMsgCount <= len(messages) {
		prefixHash := HashMessages(messages[:session.cachedMsgCount])
		if prefixHash == session.cachedMsgsHash {
			c.deps.Log(ctx, "imc", "status", "cache hit (after-lock)", "slot", session.slotID, "seq", session.seqID,
				"cached-msgs", session.cachedMsgCount, "total-tokens-cached", session.totalTokensCached)

			lastMsgIdx := session.cachedMsgCount
			totalTokens := session.totalTokensCached
			seqID := session.seqID
			sID := session.slotID
			hash := session.cachedMsgsHash

			c.mu.Unlock()

			return Result{
				ModifiedD: RemoveFirstNMessages(d, lastMsgIdx),
				IMC: &IMCResult{
					SlotID:         sID,
					SeqID:          seqID,
					CacheIdx:       llama.Pos(totalTokens),
					ExpectedHash:   hash,
					CachedMsgCount: lastMsgIdx,
				},
			}
		}
	}

	if session.pending {
		c.deps.Log(ctx, "imc", "status", "build-from-scratch skipped (slot pending)", "slot", session.slotID)
		c.mu.Unlock()

		return Result{
			ModifiedD: d,
			Err:       fmt.Errorf("imc: slot %d pending, retry request", session.slotID),
			IMC:       &IMCResult{Pending: true},
		}
	}

	session.totalTokensCached = 0
	session.cachedMsgCount = 0
	session.cachedMsgsHash = ""
	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending", "slot", slotID, "seq", seqID)

	msgsToCache := messages[:lastMsgIdxToCache]
	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}

	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	if c.cfg.SupportsMedia {
		if slices.ContainsFunc(msgsToCache, MessageHasMedia) {
			newHash := HashMessages(msgsToCache)

			c.deps.Log(ctx, "imc", "status", "media cache build prepared", "slot", slotID, "seq", seqID,
				"msgs", lastMsgIdxToCache, "hash", newHash[:8])

			return Result{
				ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
				IMC: &IMCResult{
					SlotID:            slotID,
					SeqID:             seqID,
					CacheIdx:          0,
					ExpectedHash:      newHash,
					CachedMsgCount:    lastMsgIdxToCache,
					NewCachedMsgCount: lastMsgIdxToCache,
					NewMsgsHash:       newHash,
					ClearSeq:          true,
					MediaBuild:        true,
					MediaCacheD:       prefixD,
				},
			}
		}
	}

	dataToCache, _, err := c.deps.CreatePrompt(ctx, prefixD)
	if err != nil {
		c.ClearPending(slotID)
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: failed to template messages: %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-scratch",
		attribute.String("cache-type", "imc-build"),
	)

	tokens := c.deps.TokenizeString(dataToCache)
	nTokens := len(tokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", nTokens))
	tokenSpan.End()

	if nTokens == 0 {
		c.ClearPending(slotID)
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < c.cfg.MinTokens {
		c.deps.Log(ctx, "imc", "status", "skip (too short)", "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", c.cfg.MinTokens)
		c.ClearPending(slotID)
		return Result{ModifiedD: d}
	}

	newHash := HashMessages(msgsToCache)

	c.deps.Log(ctx, "imc", "status", "cache build prepared", "slot", slotID, "seq", seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:            slotID,
			SeqID:             seqID,
			CacheIdx:          0,
			ExpectedHash:      newHash,
			CachedMsgCount:    lastMsgIdxToCache,
			NewCacheTokens:    tokens,
			NewTotalCached:    nTokens,
			NewCachedMsgCount: lastMsgIdxToCache,
			NewMsgsHash:       newHash,
			ClearSeq:          true,
			NewCachedTokens:   tokens,
		},
	}
}

// =============================================================================
// rebuildIMCWithMedia
// =============================================================================

func (c *IMCCache) rebuildIMCWithMedia(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) Result {
	c.mu.Lock()

	if session.pending {
		c.mu.Unlock()
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	session.totalTokensCached = 0
	session.cachedMsgCount = 0
	session.cachedMsgsHash = ""
	session.hasMedia = false
	session.useMRoPE = false
	seqID := session.seqID
	slotID := session.slotID

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending (media rebuild)", "slot", slotID, "seq", seqID)

	msgsToCache := messages[:lastMsgIdxToCache]
	newHash := HashMessages(msgsToCache)

	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	c.deps.Log(ctx, "imc", "status", "media rebuild prepared", "slot", slotID, "seq", seqID,
		"msgs", lastMsgIdxToCache, "hash", newHash[:8])

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:            slotID,
			SeqID:             seqID,
			CacheIdx:          0,
			ExpectedHash:      newHash,
			CachedMsgCount:    lastMsgIdxToCache,
			NewCachedMsgCount: lastMsgIdxToCache,
			NewMsgsHash:       newHash,
			ClearSeq:          true,
			MediaBuild:        true,
			MediaCacheD:       prefixD,
		},
	}
}

// =============================================================================
// rebuildIMCFromPartialPrefix
// =============================================================================

func (c *IMCCache) rebuildIMCFromPartialPrefix(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int, allTokens []llama.Token, commonPrefixLen int) Result {
	c.mu.Lock()

	if session.pending {
		c.mu.Unlock()
		return Result{ModifiedD: d, Err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	c.mu.Unlock()

	c.deps.Log(ctx, "imc", "status", "slot marked pending (partial prefix)", "slot", slotID, "seq", seqID)

	extensionTokens := allTokens[commonPrefixLen:]
	totalTokens := len(allTokens)

	msgsToCache := messages[:lastMsgIdxToCache]
	newHash := HashMessages(msgsToCache)

	c.deps.Log(ctx, "imc", "status", "partial prefix rebuild prepared", "slot", slotID, "seq", seqID,
		"common-prefix", commonPrefixLen, "extension-tokens", len(extensionTokens),
		"total-tokens", totalTokens, "hash", newHash[:8])

	return Result{
		ModifiedD: RemoveFirstNMessages(d, lastMsgIdxToCache),
		IMC: &IMCResult{
			SlotID:            slotID,
			SeqID:             seqID,
			CacheIdx:          llama.Pos(commonPrefixLen),
			ExpectedHash:      newHash,
			CachedMsgCount:    lastMsgIdxToCache,
			NewCacheTokens:    extensionTokens,
			NewTotalCached:    totalTokens,
			NewCachedMsgCount: lastMsgIdxToCache,
			NewMsgsHash:       newHash,
			TrimPos:           llama.Pos(commonPrefixLen),
			NewCachedTokens:   allTokens,
		},
	}
}

// =============================================================================
// waitForIMCSlot
// =============================================================================

func (c *IMCCache) waitForIMCSlot(ctx context.Context, requestStart time.Time) error {
	remaining := c.cfg.SlotTimeout - time.Since(requestStart)
	if remaining <= 0 {
		return fmt.Errorf("server busy processing other requests, try again shortly")
	}

	deadline := time.After(remaining)

	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		// Check if any slot is available.
		for _, slot := range c.slots {
			if !slot.pending {
				return nil
			}
		}

		// Check context and deadline before waiting.
		select {
		case <-ctx.Done():
			return fmt.Errorf("imc: context canceled while waiting for slot: %w", ctx.Err())
		case <-deadline:
			return fmt.Errorf("server busy processing other requests, try again shortly")
		default:
		}

		// Wait for notification with periodic timeout to recheck deadline/ctx.
		// Use a short goroutine to wake us after a bounded interval so we can
		// recheck the deadline and context without holding the lock forever.
		waitDone := make(chan struct{})
		go func() {
			select {
			case <-time.After(50 * time.Millisecond):
			case <-waitDone:
			}
			c.cond.Broadcast()
		}()

		c.cond.Wait()
		close(waitDone)
	}
}
