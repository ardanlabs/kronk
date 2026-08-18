package model

import (
	"context"
	"slices"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

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
		details = append(details, IMCSystemCacheDetail{
			ID:             cache.id,
			Tokens:         len(cache.cachedTokens),
			Allocated:      max(cache.allocatedContext, len(cache.cachedTokens)),
			SnapshotBytes:  imcSnapshotBytes(cache.kvState, cache.draftKVState, cache.pendingH),
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

// imcPublishSystemCache publishes an immutable system-only preload image.
// Ownership of targetStore and draftStore always transfers to this function.
func (m *Model) imcPublishSystemCache(ctx context.Context, tokens []llama.Token, targetStore, draftStore SessionStore, pendingH []float32) bool {
	m.cacheMu.Lock()
	for _, cache := range m.imcSystemCaches {
		if cache.kvState != nil && slices.Equal(cache.cachedTokens, tokens) {
			cache.lastUsed = time.Now()
			m.cacheMu.Unlock()
			closeSystemCacheStores(targetStore, draftStore)
			return false
		}
	}

	var selected *imcSystemCache
	for _, cache := range m.imcSystemCaches {
		if cache.kvState == nil {
			selected = cache
			break
		}
		if cache.activeRestores == 0 && (selected == nil || cache.lastUsed.Before(selected.lastUsed)) {
			selected = cache
		}
	}
	if selected == nil {
		m.cacheMu.Unlock()
		closeSystemCacheStores(targetStore, draftStore)
		return false
	}

	oldTarget := selected.kvState
	oldDraft := selected.draftKVState
	selected.cachedTokens = slices.Clone(tokens)
	selected.kvState = targetStore
	selected.draftKVState = draftStore
	selected.pendingH = slices.Clone(pendingH)
	selected.allocatedContext = len(tokens)
	selected.lastUsed = time.Now()
	selected.restoreCount = 0
	m.cacheMu.Unlock()

	closeSystemCacheStores(oldTarget, oldDraft)
	m.log(ctx, "imc", "status", "system-cache-published", "system_cache_entry", selected.id,
		"tokens", len(tokens), "snapshot_bytes", fmtBytes(uint64(targetStore.Len())))
	return true
}

func closeSystemCacheStores(target, draft SessionStore) {
	if target != nil {
		_ = target.Close()
	}
	if draft != nil {
		_ = draft.Close()
	}
}
