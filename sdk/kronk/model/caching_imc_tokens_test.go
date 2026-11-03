package model

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestTokensHavePrefix(t *testing.T) {
	tests := []struct {
		name   string
		tokens []llama.Token
		prefix []llama.Token
		want   bool
	}{
		{name: "exact", tokens: []llama.Token{1, 2}, prefix: []llama.Token{1, 2}, want: true},
		{name: "append", tokens: []llama.Token{1, 2, 3}, prefix: []llama.Token{1, 2}, want: true},
		{name: "divergence", tokens: []llama.Token{1, 9, 3}, prefix: []llama.Token{1, 2}, want: false},
		{name: "longer prefix", tokens: []llama.Token{1}, prefix: []llama.Token{1, 2}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokensHavePrefix(tt.tokens, tt.prefix); got != tt.want {
				t.Errorf("tokensHavePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommonTokenPrefixLen(t *testing.T) {
	tests := []struct {
		name string
		a    []llama.Token
		b    []llama.Token
		want int
	}{
		{name: "empty", a: nil, b: []llama.Token{1}, want: 0},
		{name: "diverges immediately", a: []llama.Token{1}, b: []llama.Token{2}, want: 0},
		{name: "divergent prefix", a: []llama.Token{1, 2, 3, 9}, b: []llama.Token{1, 2, 3, 4}, want: 3},
		{name: "shorter complete", a: []llama.Token{1, 2}, b: []llama.Token{1, 2, 3}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commonTokenPrefixLen(tt.a, tt.b); got != tt.want {
				t.Errorf("commonTokenPrefixLen() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProcessIMCTokenPlanPublishesVerifiedSystemBoundary(t *testing.T) {
	session := &imcSession{id: 0, kvState: &imcCheckpoint{}}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(2)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
	bindTestTokenCaches(&m)
	target := []llama.Token{1, 2, 3, 4, 5, 6}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "system", "content": "rules"}, {"role": "user", "content": "hello"}}}, append(slices.Clone(target), 9), target, target[:2], time.Now())

	if result.cacheIdx != 0 || result.imcSystemBoundaryTokens != 2 {
		t.Errorf("plan restored/system tokens = %d/%d, want 0/2", result.cacheIdx, result.imcSystemBoundaryTokens)
	}
}

func TestProcessIMCTokenPlanRejectsUnverifiedSystemBoundary(t *testing.T) {
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(2)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{{id: 0, kvState: &imcCheckpoint{}}},
	}
	bindTestTokenCaches(&m)
	target := []llama.Token{1, 2, 3, 4, 5}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "system", "content": "rules"}}}, append(slices.Clone(target), 9), target, []llama.Token{1, 9}, time.Now())

	if result.imcSystemBoundaryTokens != 0 {
		t.Errorf("imcSystemBoundaryTokens = %d, want 0", result.imcSystemBoundaryTokens)
	}
}

func TestProcessIMCTokenPlanSelectsLongestCompletePrefix(t *testing.T) {
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		imcSessions: []*imcSession{
			{id: 0, cache: &tokenCache{cachedTokens: []llama.Token{1}, totalTokensCached: 1}, kvState: populatedTestSessionStore(t)},
			{id: 1, cache: &tokenCache{cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)},
			{id: 2, cache: &tokenCache{cachedTokens: []llama.Token{1, 9}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)},
		},
	}
	bindTestTokenCaches(&m)

	actual := []llama.Token{1, 2, 3, 4}
	stable := []llama.Token{1, 2, 3}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, actual, stable, nil, time.Now())

	if result.imcSessionID != 1 {
		t.Errorf("imcSessionID = %d, want 1", result.imcSessionID)
	}
	if result.imcMatchKind != "append" {
		t.Errorf("imcMatchKind = %q, want %q", result.imcMatchKind, "append")
	}
	if len(result.imcNewCacheTokens) != 1 || result.imcNewCacheTokens[0] != 3 {
		t.Errorf("imcNewCacheTokens = %v, want [3]", result.imcNewCacheTokens)
	}
	if len(result.imcTailTokens) != 1 || result.imcTailTokens[0] != 4 {
		t.Errorf("imcTailTokens = %v, want [4]", result.imcTailTokens)
	}
}

func TestProcessIMCTokenPlanRejectsNonPrefixRender(t *testing.T) {
	m := Model{cfg: Config{PtrCacheMinTokens: new(1)}}
	result := m.processIMCTokenPlan(context.Background(), nil, []llama.Token{1, 2}, []llama.Token{1, 9}, nil, time.Now())
	if result.imcTokenPlan {
		t.Fatal("imcTokenPlan = true, want false")
	}
}

func TestProcessIMCTokenPlanUsesSystemCacheAfterCurrentDiverges(t *testing.T) {
	current := []llama.Token{1, 2, 8}
	system := []llama.Token{1, 2}
	session := &imcSession{
		id:      0,
		cache:   &tokenCache{cachedTokens: current, totalTokensCached: len(current)},
		kvState: populatedTestSessionStore(t),
	}
	cache := &imcSystemCache{id: 0, cachedTokens: system, kvState: populatedTestSessionStore(t)}
	m := Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: applog.DiscardLogger, imcSessions: []*imcSession{session}, imcSystemCaches: []*imcSystemCache{cache}}
	bindTestTokenCaches(&m)
	stable := []llama.Token{1, 2, 3, 4}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "system", "content": "rules"}}}, append(slices.Clone(stable), 9), stable, system, time.Now())

	if result.imcMatchKind != "append" {
		t.Errorf("imcMatchKind = %q, want append", result.imcMatchKind)
	}
	if result.cacheIdx != llama.Pos(len(system)) || result.imcSystemCache != cache {
		t.Errorf("cacheIdx/system cache = %d/%p, want %d/%p", result.cacheIdx, result.imcSystemCache, len(system), cache)
	}
	if cache.activeRestores != 1 || cache.restoreCount != 1 {
		t.Errorf("system cache restores = %d/%d, want 1/1", cache.activeRestores, cache.restoreCount)
	}
	m.imcReleaseSystemCache(cache)
	m.imcReleaseReservation(session.id)
	if session.reserved || cache.activeRestores != 0 {
		t.Error("failed request ownership was not released")
	}
}

func TestProcessIMCTokenPlanSkipsSystemCacheBeingRebuilt(t *testing.T) {
	session := &imcSession{id: 0, kvState: &imcCheckpoint{}}
	cache := &imcSystemCache{
		id:           0,
		cachedTokens: []llama.Token{1, 2},
		kvState:      populatedTestSessionStore(t),
		building:     true,
	}
	m := Model{
		cfg:             Config{PtrCacheMinTokens: new(1)},
		log:             applog.DiscardLogger,
		imcSessions:     []*imcSession{session},
		imcSystemCaches: []*imcSystemCache{cache},
	}
	bindTestTokenCaches(&m)
	stable := []llama.Token{1, 2, 3, 4}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "system", "content": "rules"}}}, append(slices.Clone(stable), 9), stable, []llama.Token{1, 2}, time.Now())

	if result.imcSystemCache != nil || cache.activeRestores != 0 {
		t.Error("System cache was restored while its allocation was being rebuilt")
	}
	m.imcReleaseReservation(session.id)
}

func TestProcessIMCTokenPlanChecksAllCurrentCachesBeforeSystem(t *testing.T) {
	current := &imcSession{
		id:      0,
		cache:   &tokenCache{cachedTokens: []llama.Token{1}, totalTokensCached: 1},
		kvState: populatedTestSessionStore(t),
	}
	destination := &imcSession{id: 1, cache: &tokenCache{cachedTokens: []llama.Token{8}, totalTokensCached: 1}, kvState: populatedTestSessionStore(t)}
	checkpoint := &imcSystemCache{id: 0, cachedTokens: []llama.Token{1, 2, 3}, kvState: populatedTestSessionStore(t)}
	m := Model{
		cfg:             Config{PtrCacheMinTokens: new(1)},
		log:             applog.DiscardLogger,
		imcSessions:     []*imcSession{current, destination},
		imcSystemCaches: []*imcSystemCache{checkpoint},
	}
	bindTestTokenCaches(&m)

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, []llama.Token{1, 2, 3, 4, 5}, []llama.Token{1, 2, 3, 4}, nil, time.Now())

	if result.imcSessionID != current.id || result.cacheIdx != 1 {
		t.Errorf("selected session/cacheIdx = %d/%d, want %d/1", result.imcSessionID, result.cacheIdx, current.id)
	}
	if result.imcSystemCache != nil || checkpoint.activeRestores != 0 {
		t.Error("System cache selected while a Current cache matched")
	}
}

func TestProcessIMCTokenPlanPrefersLongerCurrentSnapshotOverCheckpoint(t *testing.T) {
	session := &imcSession{
		id:      0,
		cache:   &tokenCache{cachedTokens: []llama.Token{1, 2, 3}, totalTokensCached: 3},
		kvState: populatedTestSessionStore(t),
	}
	m := Model{
		cfg:             Config{PtrCacheMinTokens: new(1)},
		log:             applog.DiscardLogger,
		imcSessions:     []*imcSession{session},
		imcSystemCaches: []*imcSystemCache{{id: 0, cachedTokens: []llama.Token{1, 2}, kvState: populatedTestSessionStore(t)}},
	}
	bindTestTokenCaches(&m)

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, []llama.Token{1, 2, 3, 4, 5}, []llama.Token{1, 2, 3, 4}, []llama.Token{1, 2}, time.Now())

	if result.cacheIdx != 3 {
		t.Errorf("cacheIdx = %d, want longer current prefix 3", result.cacheIdx)
	}
	if !slices.Equal(session.cache.cachedTokens, []llama.Token{1, 2, 3}) {
		t.Error("planner swapped in shorter checkpoint instead of retaining current state")
	}
	if result.imcSystemBoundaryTokens != 0 {
		t.Errorf("imcSystemBoundaryTokens = %d, want immutable existing System cache", result.imcSystemBoundaryTokens)
	}
}

func TestProcessIMCTokenPlanReservesExactMatch(t *testing.T) {
	d := D{"messages": []D{{"role": "user", "content": "x"}}}
	session := &imcSession{
		id:      0,
		cache:   &tokenCache{cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, cachedMsgCount: 1},
		kvState: populatedTestSessionStore(t),
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
	bindTestTokenCaches(&m)
	session.cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

	result := m.processIMCTokenPlan(context.Background(), d, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())

	if result.imcMatchKind != "exact" {
		t.Errorf("imcMatchKind = %q, want exact", result.imcMatchKind)
	}
	if !result.imcPureHitSkipSnapshot {
		t.Error("imcPureHitSkipSnapshot = false, want true")
	}
	if !session.reserved {
		t.Error("session.reserved = false, want true")
	}
}

func TestProcessIMCTokenPlanPreservesCompletePrompt(t *testing.T) {
	tests := []struct {
		name      string
		cacheMin  int
		cached    []llama.Token
		stable    []llama.Token
		actual    []llama.Token
		wantMatch string
	}{
		{name: "exact", cacheMin: 1, cached: []llama.Token{1, 2}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "exact"},
		{name: "append", cacheMin: 1, cached: []llama.Token{1}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "append"},
		{name: "rebuild after divergence", cacheMin: 1, cached: []llama.Token{7}, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: "rebuild"},
		{name: "below minimum", cacheMin: 10, cached: nil, stable: []llama.Token{1, 2}, actual: []llama.Token{1, 2, 9}, wantMatch: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "x"}}}
			sessions := []*imcSession{
				{id: 0, cache: &tokenCache{cachedTokens: slices.Clone(tt.cached), totalTokensCached: len(tt.cached)}, kvState: populatedTestSessionStore(t)},
				{id: 1, kvState: &imcCheckpoint{}},
			}
			m := Model{
				cfg:         Config{PtrCacheMinTokens: &tt.cacheMin},
				log:         applog.DiscardLogger,
				imcSessions: sessions,
			}
			bindTestTokenCaches(&m)
			sessions[0].cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

			result := m.processIMCTokenPlan(context.Background(), d, tt.actual, tt.stable, nil, time.Now())
			if result.imcMatchKind != tt.wantMatch {
				t.Errorf("imcMatchKind = %q, want %q", result.imcMatchKind, tt.wantMatch)
			}
			if !slices.Equal(result.imcSamplerPromptTokens, tt.actual) {
				t.Errorf("imcSamplerPromptTokens = %v, want complete prompt %v", result.imcSamplerPromptTokens, tt.actual)
			}

			got := slices.Clone(tt.actual[:result.cacheIdx])
			got = append(got, result.imcNewCacheTokens...)
			got = append(got, result.imcTailTokens...)
			if !slices.Equal(got, tt.actual) {
				t.Errorf("restored prefix + extension + tail = %v, want %v", got, tt.actual)
			}
		})
	}
}

func TestProcessIMCTokenPlanRebuildsExactTokensWhenRenderFingerprintChanges(t *testing.T) {
	priorD := D{
		"messages":             []D{{"role": "user", "content": "x"}},
		"chat_template_kwargs": D{"custom_mode": "a"},
	}
	currentD := priorD.Clone()
	currentD["chat_template_kwargs"] = D{"custom_mode": "b"}

	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		imcSessions: []*imcSession{
			{id: 0, cache: &tokenCache{cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)},
		},
	}
	bindTestTokenCaches(&m)
	m.imcSessions[0].cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(priorD, dMessages(priorD))

	result := m.processIMCTokenPlan(context.Background(), currentD, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())
	if result.imcMatchKind != "rebuild" {
		t.Errorf("imcMatchKind: got %q, want %q", result.imcMatchKind, "rebuild")
	}
	if result.err != nil {
		t.Errorf("err: got %v, want nil", result.err)
	}
}

func TestProcessIMCTokenPlanUsesCacheCollectionBindings(t *testing.T) {
	for _, exact := range []bool{false, true} {
		name := "append"
		if exact {
			name = "exact"
		}
		t.Run(name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "x"}}}
			log, events := capturePlanLogs(t)
			m := Model{
				cfg: Config{PtrCacheMinTokens: new(1)},
				log: log,
				imcSessions: []*imcSession{
					{id: 0, cache: &tokenCache{id: 71, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, cachedMsgsHash: "first"}, kvState: populatedTestSessionStore(t)},
					{id: 1, cache: &tokenCache{id: 29, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, cachedMsgsHash: "selected", cachedMsgCount: 3}, kvState: populatedTestSessionStore(t)},
					{id: 2, cache: &tokenCache{id: 103, cachedTokens: []llama.Token{1, 2, 3}, totalTokensCached: 3}, kvState: populatedTestSessionStore(t), reserved: true},
				},
			}
			bindTestTokenCaches(&m)
			first, selected, busy := m.imcSessions[0], m.imcSessions[1], m.imcSessions[2]
			m.tokenCaches = []*tokenCache{selected.cache, busy.cache, first.cache}
			for _, cache := range m.tokenCaches {
				cache.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))
			}
			stable := []llama.Token{1, 2, 3, 4}
			if exact {
				stable = stable[:2]
			}
			actual := append(slices.Clone(stable), 9)

			result := m.processIMCTokenPlan(context.Background(), d, actual, stable, nil, time.Now())

			assertPlanLogFields(t, events["plan-ready"], map[string]any{
				"token_cache_id": 29, "imc_cache_entry": 1, "selection": "matched-prefix", "previous_cached_tokens": 0,
				"candidates_matching": 2, "candidates_reserved": 1, "match_kind": name,
			})
			if result.err != nil {
				t.Fatalf("planner error: got %v, want nil", result.err)
			}
			// Equal-length matches must resolve in cache collection order, not
			// resident order, and cache IDs must never act as resident indices.
			if result.imcSession != selected || result.imcSessionID != selected.id {
				t.Fatalf("selected session: got %p/%d, want %p/%d", result.imcSession, result.imcSessionID, selected, selected.id)
			}
			if result.imcMatchKind != name || result.cacheIdx != 2 || result.imcClearSeq {
				t.Errorf("plan: got kind %q prefix %d clear %t, want %q/2/false", result.imcMatchKind, result.cacheIdx, result.imcClearSeq, name)
			}
			if result.imcExpectedHash != "selected" || result.imcExpectedCachedMsgs != 3 || result.imcExpectedTokens != 2 || result.imcExpectedPosition != 2 {
				t.Errorf("expected metadata: got %q/%d/%d/%d, want selected/3/2/2", result.imcExpectedHash, result.imcExpectedCachedMsgs, result.imcExpectedTokens, result.imcExpectedPosition)
			}
			if !selected.reserved || first.reserved || !busy.reserved {
				t.Errorf("reservations: got %t/%t/%t, want true/false/true", selected.reserved, first.reserved, busy.reserved)
			}
			if !slices.Equal(result.imcNewCacheTokens, stable[2:]) || !slices.Equal(result.imcTailTokens, []llama.Token{9}) {
				t.Errorf("extension/tail: got %v/%v, want %v/[9]", result.imcNewCacheTokens, result.imcTailTokens, stable[2:])
			}
		})
	}
}

func TestProcessIMCTokenPlanRebuildResetsBoundCache(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "LRU"
		if empty {
			name = "empty before LRU"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			m := Model{
				cfg: Config{PtrCacheMinTokens: new(1)},
				log: applog.DiscardLogger,
				imcSessions: []*imcSession{
					{id: 0, cache: &tokenCache{id: 81, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, cachedMsgsHash: "busy"}, kvState: populatedTestSessionStore(t), reserved: true, lastUsed: now.Add(-3 * time.Hour)},
					{id: 1, cache: &tokenCache{id: 37, cachedTokens: []llama.Token{8}, totalTokensCached: 1, cachedMsgsHash: "retain"}, kvState: populatedTestSessionStore(t), lastUsed: now.Add(-time.Hour)},
					{id: 2, cache: &tokenCache{
						id: 92, cachedTokens: []llama.Token{7}, totalTokensCached: 1,
						cachedMsgsHash: "discard", cachedMsgCount: 4, nextLogicalPos: 9,
						hasMedia: true, useMRoPE: true, useNonCausal: true,
						mediaKVCounts: []int{5}, mediaNativeChunks: []imcMediaChunk{{kind: imcMediaChunkImage, nTokens: 5, nPos: 2}},
						promptPlan: mediaPlan(promptUnit{isMedia: true}), samplerPromptTokens: []llama.Token{7}, cachedRenderInputHash: "old-render",
					}, kvState: populatedTestSessionStore(t), draftKVState: populatedTestSessionStore(t), pendingH: []float32{1, 2}, lastUsed: now.Add(-2 * time.Hour)},
				},
			}
			bindTestTokenCaches(&m)
			busy, retained, victim := m.imcSessions[0], m.imcSessions[1], m.imcSessions[2]
			if empty {
				victim.cache.totalTokensCached = 0
				victim.lastUsed = now
			}
			cache := victim.cache
			m.tokenCaches = []*tokenCache{retained.cache, cache, busy.cache}
			retainedMetadata, busyMetadata := *retained.cache, *busy.cache
			d := D{"messages": []D{{"role": "user", "content": "x"}}}
			stable := []llama.Token{1, 2, 3}

			result := m.processIMCTokenPlan(context.Background(), d, []llama.Token{1, 2, 3, 9}, stable, nil, now)

			if result.err != nil {
				t.Fatalf("planner error: got %v, want nil", result.err)
			}
			if result.imcSession != victim || result.imcSessionID != victim.id || result.imcMatchKind != "rebuild" || result.cacheIdx != 0 || !result.imcClearSeq {
				t.Fatalf("rebuild: got session %p/%d kind %q prefix %d clear %t, want %p/%d rebuild/0/true", result.imcSession, result.imcSessionID, result.imcMatchKind, result.cacheIdx, result.imcClearSeq, victim, victim.id)
			}
			wantCache := tokenCache{id: 92, session: victim}
			if victim.cache != cache || m.tokenCaches[1] != cache || !reflect.DeepEqual(*cache, wantCache) {
				t.Errorf("reset cache: got %+v, want %+v with original collection binding", *cache, wantCache)
			}
			if !reflect.DeepEqual(*retained.cache, retainedMetadata) || !reflect.DeepEqual(*busy.cache, busyMetadata) || retained.kvState.Len() != 1 || busy.kvState.Len() != 1 {
				t.Error("rebuild changed unrelated cache metadata or resident snapshots")
			}
			if victim.kvState.Len() != 0 || victim.draftKVState.Len() != 0 || len(victim.pendingH) != 0 || !victim.reserved || retained.reserved || !busy.reserved {
				t.Error("rebuild did not reset only the selected resident while preserving reservations")
			}
			if result.imcExpectedHash != "" || result.imcExpectedTokens != 0 || result.imcExpectedCachedMsgs != 0 || result.imcExpectedPosition != 0 {
				t.Error("rebuild retained invalidated expected metadata")
			}
			if !slices.Equal(result.imcNewCacheTokens, stable) || !slices.Equal(result.imcNewCachedTokens, stable) || !slices.Equal(result.imcTailTokens, []llama.Token{9}) {
				t.Errorf("rebuild tokens: got extension %v cached %v tail %v, want %v/%v/[9]", result.imcNewCacheTokens, result.imcNewCachedTokens, result.imcTailTokens, stable, stable)
			}
		})
	}
}

func populatedTestSessionStore(t testing.TB) *imcCheckpoint {
	t.Helper()
	store := &imcCheckpoint{}
	buf := prepareTestStore(t, store, 1)
	buf[0] = 1
	commitTestStore(t, store, 1)
	return store
}

// capturePlanLogs checks the structured log boundary, not planner call order.
func capturePlanLogs(t *testing.T) (applog.Logger, map[string]map[string]any) {
	t.Helper()
	events := make(map[string]map[string]any)
	return func(_ context.Context, _ string, args ...any) {
		t.Helper()
		if len(args)%2 != 0 {
			t.Fatalf("log arguments: got %d, want key/value pairs", len(args))
		}
		fields := make(map[string]any)
		for i := 0; i < len(args); i += 2 {
			key, ok := args[i].(string)
			if !ok {
				t.Fatalf("log key: got %T, want string", args[i])
			}
			switch key {
			case "tokens", "messages", "content", "prompt", "cached_tokens", "cached_messages":
				t.Errorf("log contains private payload field %q", key)
			}
			switch args[i+1].(type) {
			case []llama.Token, []D, D, promptPlan:
				t.Errorf("log field %q contains private payload type %T", key, args[i+1])
			}
			fields[key] = args[i+1]
		}
		if status, ok := fields["status"].(string); ok {
			events[status] = fields
		}
	}, events
}

func assertPlanLogFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			t.Errorf("log %s: got %v (%T), want %v (%T)", key, got[key], got[key], value, value)
		}
	}
}

func TestTokenCachePlannerDiagnostics(t *testing.T) {
	for _, media := range []bool{false, true} {
		name := "text"
		if media {
			name = "media"
		}
		t.Run(name, func(t *testing.T) {
			for _, scenario := range []string{"matching", "empty", "lru-eviction", "busy", "reset-failed"} {
				t.Run(scenario, func(t *testing.T) {
					log, events := capturePlanLogs(t)
					m := Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: log}
					d := D{"messages": []D{{"role": "user", "content": "private message"}}}
					stableTokens := []llama.Token{1, 2, 3}
					actualTokens := []llama.Token{1, 2, 3, 9}
					base := mediaPlan(promptUnit{token: 1}, promptUnit{isMedia: true}, promptUnit{token: 2})
					stable := mediaPlan(append(slices.Clone(base.units), promptUnit{token: 3})...)
					actual := mediaPlan(append(slices.Clone(stable.units), promptUnit{token: 9})...)
					fingerprint, _ := m.imcRenderFingerprint(d, dMessages(d))
					add := func() *imcSession {
						id := len(m.imcSessions)
						cache := &tokenCache{id: 70 + id*13, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2}
						if media {
							cache.hasMedia = true
							cache.totalTokensCached = 12
							cache.nextLogicalPos = 5
							cache.useMRoPE = true
							cache.promptPlan = base
							cache.mediaKVCounts = []int{8}
							cache.samplerPromptTokens = []llama.Token{100, 1, 2}
						}
						session := &imcSession{id: id, cache: cache, kvState: populatedTestSessionStore(t), lastUsed: time.Now()}
						cache.session = session
						m.imcSessions = append(m.imcSessions, session)
						m.tokenCaches = append(m.tokenCaches, cache)
						return session
					}
					want := map[string]any{
						"candidates_unbound": 1, "candidates_reserved": 1, "candidates_empty": 0,
						"candidates_missing_checkpoint": 0, "candidates_incompatible": 0,
						"candidates_prefix_mismatch": 0, "candidates_fingerprint_mismatch": 0, "candidates_matching": 0,
					}
					// An unbound and an empty, checkpoint-less reserved cache each
					// count once, even though later rejection criteria also apply.
					m.tokenCaches = append(m.tokenCaches, &tokenCache{id: 999})
					busy := add()
					busy.reserved, busy.cache.totalTokensCached, busy.kvState = true, 0, nil
					var selected *imcSession
					if scenario != "busy" {
						missing := add()
						missing.kvState = nil
						missing.cache.cachedTokens = []llama.Token{8}
						want["candidates_missing_checkpoint"] = 1
						invalid := add()
						if media {
							invalid.cache.mediaKVCounts = nil
						} else {
							invalid.cache.hasMedia = true
						}
						want["candidates_incompatible"] = 1
						divergent := add()
						divergent.cache.cachedTokens = []llama.Token{8}
						divergent.cache.promptPlan = mediaPlan(promptUnit{token: 8}, promptUnit{isMedia: true})
						want["candidates_prefix_mismatch"] = 1
						changed := add()
						changed.cache.cachedTokens = slices.Clone(stableTokens)
						changed.cache.promptPlan = stable
						changed.cache.cachedRenderInputHash = fingerprint + "changed"
						want["candidates_fingerprint_mismatch"] = 1
						selected = add()
						selected.lastUsed = time.Now().Add(-time.Hour)
						switch scenario {
						case "matching":
							add() // All eligible matches count, not only the winner.
							want["candidates_matching"] = 2
						case "empty":
							selected.cache.totalTokensCached = 0
							selected.kvState = nil
							want["candidates_empty"] = 1
						default:
							selected.cache.cachedTokens = []llama.Token{8, 9}
							selected.cache.promptPlan = divergent.cache.promptPlan
							want["candidates_prefix_mismatch"] = 2
						}
					}
					ctx := context.Background()
					if scenario == "reset-failed" {
						var cancel context.CancelFunc
						ctx, cancel = context.WithCancel(ctx)
						cancel()
					}
					var result cacheResult
					if media {
						result = m.processIMCMediaPlans(ctx, d, d, actual, stable, []llama.Token{9}, nil, time.Now())
					} else {
						result = m.processIMCTokenPlan(ctx, d, actualTokens, stableTokens, nil, time.Now())
					}
					if scenario == "busy" {
						if result.err == nil {
							t.Error("busy error: got nil, want non-nil")
						}
						want["reason"] = "no-available-resident"
						assertPlanLogFields(t, events["plan-unavailable"], want)
						return
					}
					want["token_cache_id"], want["imc_cache_entry"] = selected.cache.id, selected.id
					want["selection"], want["previous_cached_tokens"] = scenario, 0
					if scenario == "matching" {
						want["selection"] = "matched-prefix"
					}
					if scenario == "lru-eviction" || scenario == "reset-failed" {
						want["previous_cached_tokens"] = 2
						if media {
							want["previous_cached_tokens"] = 12
						}
					}
					if scenario == "reset-failed" {
						want["selection"] = "lru-eviction"
						fields := events["plan-reset-failed"]
						for _, key := range []string{"token_cache_id", "imc_cache_entry", "selection", "previous_cached_tokens"} {
							assertPlanLogFields(t, fields, map[string]any{key: want[key]})
						}
						loggedErr, _ := fields["err"].(error)
						if !errors.Is(result.err, context.Canceled) || !errors.Is(loggedErr, context.Canceled) || selected.reserved {
							t.Errorf("reset failure: got result %v log %v reserved %t, want canceled/canceled/false", result.err, loggedErr, selected.reserved)
						}
						return
					}
					assertPlanLogFields(t, events["plan-ready"], want)
					if result.err != nil || result.imcSession != selected || !selected.reserved {
						t.Fatalf("plan: got error %v resident %p reserved %t, want nil/%p/true", result.err, result.imcSession, selected.reserved, selected)
					}
					wantKind, wantPrefix := "rebuild", llama.Pos(0)
					if scenario == "matching" {
						wantKind, wantPrefix = "append", 2
						if media {
							wantKind, wantPrefix = "anchor", 5
						}
					}
					if result.imcMatchKind != wantKind || result.cacheIdx != wantPrefix || !slices.Equal(result.imcTailTokens, []llama.Token{9}) {
						t.Errorf("matching semantics: got %s/%d/%v, want %s/%d/[9]", result.imcMatchKind, result.cacheIdx, result.imcTailTokens, wantKind, wantPrefix)
					}
				})
			}
		})
	}
}
