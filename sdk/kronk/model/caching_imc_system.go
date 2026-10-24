package model

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// imcSystemTokens renders the same system-only candidate for text and media
// requests. Each planner must verify it against its complete stable rendering.
// stablePrompt is that rendering, used only when the template can't render the
// system messages alone (see imcSystemProbeTokens).
func (m *Model) imcSystemTokens(ctx context.Context, stableD D, stablePrompt string) []llama.Token {
	messages := dMessages(stableD)
	systemMessages := 0
	for systemMessages < len(messages) {
		role, _ := messages[systemMessages]["role"].(string)
		if role != "system" {
			break
		}
		systemMessages++
	}
	if systemMessages == 0 {
		return nil
	}
	systemD := maps.Clone(stableD)
	systemD["messages"] = messages[:systemMessages]
	prompt, media, err := m.createPrompt(ctx, systemD)
	if err != nil {
		return m.imcSystemProbeTokens(ctx, stableD, messages[:systemMessages], stablePrompt, err)
	}
	if len(media) > 0 {
		return nil
	}
	return llama.Tokenize(m.vocab, prompt, m.addBOSToken, true)
}

// imcSystemProbeContent is the placeholder user message rendered after the
// system messages when a template can't render them alone. Its text never
// reaches the model.
const imcSystemProbeContent = "."

// imcSystemProbeTokens finds the system boundary for templates that refuse to
// render system messages without a user message (Qwen3.5/3.8: "No user query
// found in messages"), which otherwise leaves the System tier unusable for
// them. It renders the system messages followed by a placeholder user message
// and takes the boundary from what that shares with the stable prompt.
func (m *Model) imcSystemProbeTokens(ctx context.Context, stableD D, system []D, stablePrompt string, renderErr error) []llama.Token {
	probeD := maps.Clone(stableD)
	probeD["messages"] = append(slices.Clone(system), D{"role": "user", "content": imcSystemProbeContent})
	probePrompt, media, err := m.createPrompt(ctx, probeD)
	if err != nil {
		m.log(ctx, "imc", "status", "system-cache-skipped", "reason", "render-failed", "err", renderErr, "probe_err", err)
		return nil
	}
	if len(media) > 0 {
		return nil
	}
	stable := llama.Tokenize(m.vocab, stablePrompt, m.addBOSToken, true)
	probe := llama.Tokenize(m.vocab, probePrompt, m.addBOSToken, true)
	isControl := func(t llama.Token) bool { return llama.VocabIsControl(m.vocab, t) }
	boundary := imcSystemBoundary(stable, probe, isControl)
	if boundary == 0 {
		m.log(ctx, "imc", "status", "system-cache-skipped", "reason", "probe-no-boundary")
		return nil
	}
	return stable[:boundary]
}

// imcSystemBoundary returns how many leading tokens of stable are the system
// part, given probe: the same system messages rendered with a placeholder user
// message. The two share the system turn and the opening of the user turn,
// then differ at the user's content. The shared prefix alone isn't a stable
// cut: whether the newline after the user role merges with the first word of
// the content varies with that word, so the same system prompt would get
// different boundaries on different requests and never match its own cache.
// Backing off to just after the last control token in the shared prefix (the
// user turn's start marker) gives the same cut for every request. Returns 0
// when there is no such cut, or when it would cover all of stable.
func imcSystemBoundary(stable, probe []llama.Token, isControl func(llama.Token) bool) int {
	n := 0
	for n < len(stable) && n < len(probe) && stable[n] == probe[n] {
		n++
	}
	for n > 0 && !isControl(stable[n-1]) {
		n--
	}
	if n >= len(stable) {
		return 0
	}
	return n
}

// imcFindSystemCache reserves an immutable preload for restoration. The caller
// holds cacheMu and has verified tokens against the request's stable prefix.
func (m *Model) imcFindSystemCache(tokens []llama.Token) *imcSystemCache {
	if len(tokens) == 0 {
		return nil
	}
	for _, candidate := range m.imcSystemCaches {
		if candidate != nil && !candidate.building && candidate.kvState != nil && candidate.kvState.Len() > 0 && slices.Equal(candidate.cachedTokens, tokens) {
			candidate.activeRestores++
			candidate.restoreCount++
			candidate.lastUsed = time.Now()
			return candidate
		}
	}
	return nil
}

// IMCSystemCacheDetail is a scalar snapshot of one System cache pool entry.
type IMCSystemCacheDetail struct {
	ID             int
	Tokens         int
	Allocated      int
	SnapshotBytes  int
	RestoreCount   uint64
	ActiveRestores int
	LastUsed       time.Time
}

// IMCSystemCaches returns every entry in the immutable System preload pool.
func (m *Model) IMCSystemCaches() []IMCSystemCacheDetail {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()

	details := make([]IMCSystemCacheDetail, 0, len(m.imcSystemCaches))
	for _, cache := range m.imcSystemCaches {
		if cache == nil {
			continue
		}
		snapshotBytes := cache.snapshotBytes
		if snapshotBytes == 0 && !cache.building {
			snapshotBytes = imcSnapshotBytes(cache.kvState, cache.draftKVState, cache.pendingH)
		}
		details = append(details, IMCSystemCacheDetail{
			ID:             cache.id,
			Tokens:         len(cache.cachedTokens),
			Allocated:      max(cache.allocatedContext, len(cache.cachedTokens)),
			SnapshotBytes:  snapshotBytes,
			RestoreCount:   cache.restoreCount,
			ActiveRestores: cache.activeRestores,
			LastUsed:       cache.lastUsed,
		})
	}

	return details
}

func (m *Model) imcReleaseSystemCache(cache *imcSystemCache) {
	if cache == nil {
		return
	}

	m.cacheMu.Lock()
	if cache.activeRestores > 0 {
		cache.activeRestores--
	}
	m.cacheMu.Unlock()
}

func (m *Model) imcReserveSystemCache(tokens []llama.Token) *imcSystemCache {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	for _, cache := range m.imcSystemCaches {
		if cache == nil {
			continue
		}
		if cache.building && slices.Equal(cache.buildingTokens, tokens) {
			return nil
		}
		if !cache.building && cache.kvState != nil && cache.kvState.Len() > 0 && slices.Equal(cache.cachedTokens, tokens) {
			cache.lastUsed = time.Now()
			return nil
		}
	}

	var selected *imcSystemCache
	for _, cache := range m.imcSystemCaches {
		if cache == nil || cache.building || cache.activeRestores > 0 {
			continue
		}
		if len(cache.cachedTokens) == 0 {
			selected = cache
			break
		}
		if selected == nil || cache.lastUsed.Before(selected.lastUsed) {
			selected = cache
		}
	}
	if selected == nil {
		return nil
	}

	selected.building = true
	selected.buildingTokens = append(selected.buildingTokens[:0], tokens...)
	return selected
}

func (m *Model) imcSystemCacheStore(ctx context.Context, cache *imcSystemCache, draft bool) (SessionStore, error) {
	m.cacheMu.Lock()
	var store SessionStore
	if draft {
		store = cache.draftKVState
	} else {
		store = cache.kvState
	}
	m.cacheMu.Unlock()
	if store != nil {
		return store, nil
	}

	store, err := newSystemCacheStore(ctx)
	if err != nil {
		return nil, err
	}

	m.cacheMu.Lock()
	if draft {
		cache.draftKVState = store
	} else {
		cache.kvState = store
	}
	m.cacheMu.Unlock()
	return store, nil
}

func (m *Model) imcAbortSystemCache(ctx context.Context, cache *imcSystemCache) error {
	if cache == nil {
		return nil
	}

	var errs []error
	if cache.kvState != nil {
		if err := cache.kvState.Reset(ctx); err != nil {
			errs = append(errs, fmt.Errorf("reset target store: %w", err))
		}
	}
	if cache.draftKVState != nil {
		if err := cache.draftKVState.Reset(ctx); err != nil {
			errs = append(errs, fmt.Errorf("reset draft store: %w", err))
		}
	}
	snapshotBytes := imcSnapshotBytes(cache.kvState, cache.draftKVState, cache.pendingH)

	m.cacheMu.Lock()
	cache.cachedTokens = cache.cachedTokens[:0]
	cache.buildingTokens = cache.buildingTokens[:0]
	cache.pendingH = cache.pendingH[:0]
	cache.allocatedContext = 0
	cache.snapshotBytes = snapshotBytes
	cache.restoreCount = 0
	cache.lastUsed = time.Time{}
	cache.building = false
	m.cacheMu.Unlock()

	return errors.Join(errs...)
}

func (m *Model) imcPublishSystemCache(ctx context.Context, cache *imcSystemCache, pendingH []float32) bool {
	if cache == nil || cache.kvState == nil || cache.kvState.Len() == 0 {
		if err := m.imcAbortSystemCache(ctx, cache); err != nil {
			m.log(ctx, "imc", "status", "system-cache-abort-failed", "err", err)
		}
		return false
	}

	m.cacheMu.Lock()
	if !cache.building {
		m.cacheMu.Unlock()
		return false
	}
	cache.cachedTokens = append(cache.cachedTokens[:0], cache.buildingTokens...)
	cache.buildingTokens = cache.buildingTokens[:0]
	cache.pendingH = append(cache.pendingH[:0], pendingH...)
	cache.allocatedContext = len(cache.cachedTokens)
	cache.snapshotBytes = imcSnapshotBytes(cache.kvState, cache.draftKVState, cache.pendingH)
	cache.lastUsed = time.Now()
	cache.restoreCount = 0
	cache.building = false
	entryID := cache.id
	tokens := len(cache.cachedTokens)
	snapshotBytes := cache.snapshotBytes
	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "system-cache-published", "system_cache_entry", entryID,
		"tokens", tokens, "snapshot_bytes", fmtBytes(uint64(snapshotBytes)))
	return true
}
