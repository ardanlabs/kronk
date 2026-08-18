package model

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestIMCReserveSystemCacheDeduplicates(t *testing.T) {
	cache := &imcSystemCache{id: 0, cachedTokens: []llama.Token{1, 2}, kvState: populatedTestSessionStore()}
	m := Model{imcSystemCaches: []*imcSystemCache{cache}}

	if got := m.imcReserveSystemCache([]llama.Token{1, 2}); got != nil {
		t.Fatalf("reserve = %p, want nil for an existing System cache", got)
	}
	if cache.restoreCount != 0 || !slices.Equal(cache.cachedTokens, []llama.Token{1, 2}) {
		t.Error("duplicate reservation changed the existing entry")
	}
}

func TestIMCReserveSystemCacheReusesLeastRecentlyUsedAllocation(t *testing.T) {
	now := time.Now()
	oldActive := populatedTestSessionStore()
	oldAvailable := ramSessionStore()
	oldAvailable.Prepare(8)
	oldAvailable.Commit(1)
	newerAvailable := populatedTestSessionStore()
	caches := []*imcSystemCache{
		{id: 0, cachedTokens: []llama.Token{1}, kvState: oldActive, activeRestores: 1, lastUsed: now.Add(-3 * time.Minute)},
		{id: 1, cachedTokens: []llama.Token{2}, kvState: oldAvailable, lastUsed: now.Add(-2 * time.Minute)},
		{id: 2, cachedTokens: []llama.Token{3}, kvState: newerAvailable, lastUsed: now.Add(-time.Minute)},
	}
	m := Model{log: applog.DiscardLogger, imcSystemCaches: caches}

	selected := m.imcReserveSystemCache([]llama.Token{9})
	if selected != caches[1] || !selected.building {
		t.Fatalf("reserve = %p, want building LRU entry %p", selected, caches[1])
	}
	store, err := m.imcSystemCacheStore(selected, false)
	if err != nil {
		t.Fatalf("imcSystemCacheStore: %v", err)
	}
	store.Reset()
	store.Prepare(1)[0] = 9
	store.Commit(1)
	if !m.imcPublishSystemCache(context.Background(), selected, nil) {
		t.Fatal("publish = false, want reserved entry publication")
	}
	if selected.kvState != oldAvailable || selected.kvState.Cap() != 8 {
		t.Error("System cache did not retain and reuse its backing allocation")
	}
	if !slices.Equal(selected.cachedTokens, []llama.Token{9}) || selected.building {
		t.Error("published entry does not contain the reserved System prompt")
	}
}

func TestIMCReserveSystemCacheDoesNotEvictActiveEntries(t *testing.T) {
	cache := &imcSystemCache{
		id:             0,
		cachedTokens:   []llama.Token{1},
		kvState:        populatedTestSessionStore(),
		activeRestores: 1,
	}
	m := Model{imcSystemCaches: []*imcSystemCache{cache}}

	if got := m.imcReserveSystemCache([]llama.Token{2}); got != nil {
		t.Fatalf("reserve = %p, want nil while every entry is active", got)
	}
	if !slices.Equal(cache.cachedTokens, []llama.Token{1}) {
		t.Error("active entry was changed")
	}
}

func TestIMCAbortSystemCacheRetainsAllocations(t *testing.T) {
	target := ramSessionStore()
	target.Prepare(8)
	target.Commit(4)
	draft := ramSessionStore()
	draft.Prepare(6)
	draft.Commit(3)
	cache := &imcSystemCache{id: 0, cachedTokens: []llama.Token{1}, kvState: target, draftKVState: draft}
	m := Model{imcSystemCaches: []*imcSystemCache{cache}}

	if got := m.imcReserveSystemCache([]llama.Token{2}); got != cache {
		t.Fatalf("reserve = %p, want %p", got, cache)
	}
	m.imcAbortSystemCache(cache)

	if cache.building || len(cache.cachedTokens) != 0 || target.Len() != 0 || draft.Len() != 0 {
		t.Error("abort did not empty the reserved System cache")
	}
	if target.Cap() != 8 || draft.Cap() != 6 {
		t.Error("abort released a retained System cache allocation")
	}
	if got := m.IMCSystemCaches()[0].SnapshotBytes; got != 14 {
		t.Errorf("SnapshotBytes after abort = %d, want retained capacity 14", got)
	}
}

func TestIMCSystemCacheDetailsAndRelease(t *testing.T) {
	lastUsed := time.Now()
	cache := &imcSystemCache{
		id:               3,
		cachedTokens:     []llama.Token{1, 2},
		kvState:          populatedTestSessionStore(),
		allocatedContext: 4,
		restoreCount:     7,
		activeRestores:   1,
		lastUsed:         lastUsed,
	}
	m := Model{imcSystemCaches: []*imcSystemCache{cache, {id: 4}}}

	details := m.IMCSystemCaches()
	if len(details) != 2 {
		t.Fatalf("details length = %d, want pool capacity 2", len(details))
	}
	if got := details[0]; got.ID != 3 || got.Tokens != 2 || got.Allocated != 4 || got.SnapshotBytes != 1 || got.RestoreCount != 7 || got.ActiveRestores != 1 || !got.LastUsed.Equal(lastUsed) {
		t.Errorf("details = %+v, want populated cache values", got)
	}
	if got := details[1]; got.ID != 4 || got.Tokens != 0 || got.SnapshotBytes != 0 {
		t.Errorf("empty details = %+v, want empty pool entry", got)
	}

	m.imcReleaseSystemCache(cache)
	m.imcReleaseSystemCache(cache)
	if cache.activeRestores != 0 {
		t.Errorf("activeRestores = %d, want 0", cache.activeRestores)
	}
}

func TestChatJobReleaseIMCSystemCacheIsIdempotent(t *testing.T) {
	cache := &imcSystemCache{activeRestores: 1}
	m := Model{}
	job := chatJob{imcSystemCache: cache}

	job.releaseIMCSystemCache(&m)
	job.releaseIMCSystemCache(&m)
	if cache.activeRestores != 0 || job.imcSystemCache != nil {
		t.Errorf("release left active/job cache = %d/%p, want 0/nil", cache.activeRestores, job.imcSystemCache)
	}
}

func TestReleaseIMCReservationIfHeldReleasesSystemCache(t *testing.T) {
	session := &imcSession{id: 0, reserved: true}
	systemCache := &imcSystemCache{activeRestores: 1}
	m := Model{imcSessions: []*imcSession{session}}

	m.releaseIMCReservationIfHeld(cacheResult{
		imcSession:        session,
		imcSessionID:      session.id,
		imcNewCacheTokens: []llama.Token{1},
		imcSystemCache:    systemCache,
	})
	if session.reserved || systemCache.activeRestores != 0 {
		t.Errorf("release left working/System reservations = %t/%d, want false/0", session.reserved, systemCache.activeRestores)
	}
}
