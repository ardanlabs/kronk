package model

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestIMCPublishSystemCacheDeduplicates(t *testing.T) {
	cache := &imcSystemCache{id: 0, cachedTokens: []llama.Token{1, 2}, kvState: populatedTestSessionStore()}
	incoming := populatedTrackingStore()
	m := Model{log: applog.DiscardLogger, imcSystemCaches: []*imcSystemCache{cache}}

	if m.imcPublishSystemCache(context.Background(), []llama.Token{1, 2}, incoming, nil, nil) {
		t.Fatal("publish = true, want duplicate to reuse existing entry")
	}
	if !incoming.closed {
		t.Error("duplicate store was not closed")
	}
	if cache.restoreCount != 0 || !slices.Equal(cache.cachedTokens, []llama.Token{1, 2}) {
		t.Error("duplicate publish changed the existing entry")
	}
}

func TestIMCPublishSystemCacheReplacesLeastRecentlyUsedAvailable(t *testing.T) {
	now := time.Now()
	oldActive := populatedTrackingStore()
	oldAvailable := populatedTrackingStore()
	newerAvailable := populatedTrackingStore()
	caches := []*imcSystemCache{
		{id: 0, cachedTokens: []llama.Token{1}, kvState: oldActive, activeRestores: 1, lastUsed: now.Add(-3 * time.Minute)},
		{id: 1, cachedTokens: []llama.Token{2}, kvState: oldAvailable, lastUsed: now.Add(-2 * time.Minute)},
		{id: 2, cachedTokens: []llama.Token{3}, kvState: newerAvailable, lastUsed: now.Add(-time.Minute)},
	}
	m := Model{log: applog.DiscardLogger, imcSystemCaches: caches}
	incoming := populatedTrackingStore()

	if !m.imcPublishSystemCache(context.Background(), []llama.Token{9}, incoming, nil, nil) {
		t.Fatal("publish = false, want LRU replacement")
	}
	if caches[1].kvState != incoming || !slices.Equal(caches[1].cachedTokens, []llama.Token{9}) {
		t.Error("least-recently-used available entry was not replaced")
	}
	if !oldAvailable.closed {
		t.Error("replaced store was not closed")
	}
	if oldActive.closed || newerAvailable.closed {
		t.Error("publish closed an entry that was not replaced")
	}
}

func TestIMCPublishSystemCacheDoesNotEvictActiveEntries(t *testing.T) {
	cache := &imcSystemCache{
		id:             0,
		cachedTokens:   []llama.Token{1},
		kvState:        populatedTrackingStore(),
		activeRestores: 1,
	}
	m := Model{log: applog.DiscardLogger, imcSystemCaches: []*imcSystemCache{cache}}
	incoming := populatedTrackingStore()

	if m.imcPublishSystemCache(context.Background(), []llama.Token{2}, incoming, nil, nil) {
		t.Fatal("publish = true, want retention miss while every entry is active")
	}
	if !incoming.closed {
		t.Error("unretained store was not closed")
	}
	if !slices.Equal(cache.cachedTokens, []llama.Token{1}) {
		t.Error("active entry was replaced")
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

type trackingStore struct {
	SessionStore
	closed bool
}

func populatedTrackingStore() *trackingStore {
	return &trackingStore{SessionStore: populatedTestSessionStore()}
}

func (s *trackingStore) Close() error {
	s.closed = true
	return s.SessionStore.Close()
}
