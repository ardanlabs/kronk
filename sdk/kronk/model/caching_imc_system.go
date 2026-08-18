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
		if !cache.building && cache.kvState != nil && len(cache.kvState.Bytes()) > 0 && slices.Equal(cache.cachedTokens, tokens) {
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

func (m *Model) imcSystemCacheStore(cache *imcSystemCache, draft bool) (SessionStore, error) {
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

	store, err := newSystemCacheStore()
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

func (m *Model) imcAbortSystemCache(cache *imcSystemCache) {
	if cache == nil {
		return
	}

	if cache.kvState != nil {
		cache.kvState.Reset()
	}
	if cache.draftKVState != nil {
		cache.draftKVState.Reset()
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
}

func (m *Model) imcPublishSystemCache(ctx context.Context, cache *imcSystemCache, pendingH []float32) bool {
	if cache == nil || cache.kvState == nil || cache.kvState.Len() == 0 {
		m.imcAbortSystemCache(cache)
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
