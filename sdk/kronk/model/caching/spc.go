package caching

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// spcSession holds the state for a single SPC (System Prompt Cache) session.
type spcSession struct {
	sysPromptHash   string
	sysPromptTokens int
	sysPromptLen    int
	seqID           llama.SeqId
	lastUsed        time.Time
	kvState         []byte
}

// SPCCache implements Cacher using System Prompt Caching.
type SPCCache struct {
	deps       Deps
	cfg        Config
	mu         sync.RWMutex
	session    *spcSession
	cacheSeqID llama.SeqId
}

// NewSPCCache creates a new System Prompt Cache.
func NewSPCCache(deps Deps, cfg Config) *SPCCache {
	return &SPCCache{
		deps:       deps,
		cfg:        cfg,
		cacheSeqID: cfg.SPCSeqID,
	}
}

// ProcessCache implements Cacher. It examines the first message and either
// caches it (if it's a system message) or reuses an existing cache.
func (c *SPCCache) ProcessCache(ctx context.Context, d D, _ time.Time) Result {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return Result{ModifiedD: d}
	}

	role, ok := messages[0]["role"].(string)
	if !ok {
		return Result{ModifiedD: d}
	}

	switch role {
	case RoleSystem:
		content := ExtractMessageContent(messages[0])
		if content == "" {
			return Result{ModifiedD: d}
		}

		sysMsg := CacheableMessage{
			Index:   0,
			Role:    role,
			Content: content,
		}

		result := c.performSPC(ctx, d, messages, sysMsg)
		if result.Err != nil {
			return result
		}

		if result.SPC != nil && result.SPC.CacheIdx > 0 {
			d = RemoveMessagesAtIndices(d, []int{0})
			result.ModifiedD = d
			return result
		}

		return Result{ModifiedD: d}

	default:
		c.mu.RLock()
		session := c.session
		c.mu.RUnlock()

		if session != nil && session.sysPromptTokens > 0 {
			c.deps.Log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "tokens", session.sysPromptTokens)
			return Result{
				ModifiedD: d,
				SPC: &SPCResult{
					CacheIdx: llama.Pos(session.sysPromptTokens),
				},
			}
		}
	}

	return Result{ModifiedD: d}
}

// performSPC checks for a cache hit on the system prompt. On a miss, it
// templates, tokenizes, and decodes the system prompt into the dedicated
// cache sequence.
func (c *SPCCache) performSPC(ctx context.Context, d D, messages []D, msgInfo CacheableMessage) Result {
	if msgInfo.Role != RoleSystem {
		c.deps.Log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.Role)
		return Result{ModifiedD: d}
	}

	contentLen := len(msgInfo.Content)

	// Check for cache hit (fast path with read lock).
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	newHash := HashMessage(msgInfo)

	if session != nil && session.sysPromptLen == contentLen && session.sysPromptHash == newHash && session.sysPromptTokens > 0 {
		c.deps.Log(ctx, "spc", "status", "cache hit", "tokens", session.sysPromptTokens)
		return Result{
			SPC: &SPCResult{
				CacheIdx: llama.Pos(session.sysPromptTokens),
			},
		}
	}

	// Cache miss — template and cache the message.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	session = c.session
	if session != nil && session.sysPromptLen == contentLen && session.sysPromptHash == newHash && session.sysPromptTokens > 0 {
		c.deps.Log(ctx, "spc", "status", "cache hit (after lock)", "tokens", session.sysPromptTokens)
		return Result{
			SPC: &SPCResult{
				CacheIdx: llama.Pos(session.sysPromptTokens),
			},
		}
	}

	msgsToCache := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	systemMsgPrompt, _, err := c.deps.CreatePrompt(ctx, msgsToCache)
	if err != nil {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: failed to template system prompt: %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-spc",
		attribute.String("cache-type", "spc"),
	)

	tokens := c.deps.TokenizeString(systemMsgPrompt)
	nTokens := len(tokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", nTokens))
	tokenSpan.End()

	if nTokens == 0 {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: system prompt tokenized to zero tokens")}
	}

	if nTokens < c.cfg.MinTokens {
		c.deps.Log(ctx, "spc", "status", "cache skip (too short)", "tokens", nTokens, "min", c.cfg.MinTokens)
		return Result{ModifiedD: d}
	}

	// Invalidate session before clearing KV so a failed rebuild can't serve
	// stale metadata pointing at an empty/partial sequence.
	c.session = nil

	// Clear any existing cache in the dedicated sequence.
	c.deps.ClearSequence(c.cacheSeqID)

	// Decode tokens into the temporary cache sequence.
	if err := c.deps.DecodeTokensIntoCache(ctx, tokens, c.cacheSeqID, 0); err != nil {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: decoding system prompt into cache: %w", err)}
	}

	// Extract the KV state into an external buffer so the sequence can be freed.
	kvState, nExtracted, err := c.deps.ExtractKVState(c.cacheSeqID)
	if err != nil {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: extracting KV state from seq %d: %w", c.cacheSeqID, err)}
	}

	// Free the sequence — KV entries are now in the external buffer.
	c.deps.ClearSequence(c.cacheSeqID)

	if nExtracted == 0 {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: failed to extract KV state from seq %d", c.cacheSeqID)}
	}

	if nExtracted > len(kvState) {
		return Result{ModifiedD: d, Err: fmt.Errorf("spc: extracted KV byte count (%d) exceeds buffer size (%d)", nExtracted, len(kvState))}
	}

	c.session = &spcSession{
		sysPromptHash:   newHash,
		sysPromptTokens: nTokens,
		sysPromptLen:    contentLen,
		seqID:           c.cacheSeqID,
		lastUsed:        time.Now(),
		kvState:         kvState[:nExtracted],
	}

	c.deps.Log(ctx, "spc", "tokens", nTokens, "hash", newHash[:8], "kv_bytes", nExtracted, "status", "tokens saved (externalized)")

	return Result{
		ModifiedD: d,
		SPC: &SPCResult{
			CacheIdx: llama.Pos(nTokens),
		},
	}
}

// ClearCaches resets all cached state.
func (c *SPCCache) ClearCaches() {
	c.mu.Lock()
	c.session = nil
	c.mu.Unlock()
}

// ClearPending is a no-op for SPC (no slot concept).
func (c *SPCCache) ClearPending(_ int) {}

// CommitSession is a no-op for SPC (no slot concept).
func (c *SPCCache) CommitSession(_ Commit) {}

// InvalidateSlot is a no-op for SPC (no slot concept).
func (c *SPCCache) InvalidateSlot(_ int) {}

// SnapshotSlot returns false for SPC (no slot concept).
func (c *SPCCache) SnapshotSlot(_ int) (SlotSnapshot, bool) {
	return SlotSnapshot{}, false
}

// RestoreSPCToSeq restores the externalized SPC KV state into the destination
// sequence via the Deps RestoreKVState operation.
func (c *SPCCache) RestoreSPCToSeq(dstSeqID llama.SeqId) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil || len(session.kvState) == 0 {
		return fmt.Errorf("restore-spc: no cached KV state available")
	}

	nRead, err := c.deps.RestoreKVState(session.kvState, dstSeqID)
	if err != nil {
		return fmt.Errorf("restore-spc: %w", err)
	}

	if nRead == 0 {
		return fmt.Errorf("restore-spc: StateSeqSetData failed for seq %d", dstSeqID)
	}

	if nRead < len(session.kvState) {
		return fmt.Errorf("restore-spc: partial restore for seq %d: read %d of %d bytes", dstSeqID, nRead, len(session.kvState))
	}

	return nil
}

// HasCachedSlot always returns false for SPC (no slot concept).
func (c *SPCCache) HasCachedSlot(_ int) bool {
	return false
}

// SetSlotMRoPE is a no-op for SPC (no slot concept).
func (c *SPCCache) SetSlotMRoPE(_ int, _ bool) {}
