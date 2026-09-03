package model

import (
	"context"
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
	session := &imcSession{id: 0, kvState: ramSessionStore()}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(2)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
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
		imcSessions: []*imcSession{{id: 0, kvState: ramSessionStore()}},
	}
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
			{id: 0, cachedTokens: []llama.Token{1}, totalTokensCached: 1, kvState: populatedTestSessionStore(t)},
			{id: 1, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, kvState: populatedTestSessionStore(t)},
			{id: 2, cachedTokens: []llama.Token{1, 9}, totalTokensCached: 2, kvState: populatedTestSessionStore(t)},
		},
	}

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
		id:                0,
		cachedTokens:      current,
		totalTokensCached: len(current),
		kvState:           populatedTestSessionStore(t),
	}
	cache := &imcSystemCache{id: 0, cachedTokens: system, kvState: populatedTestSessionStore(t)}
	m := Model{cfg: Config{PtrCacheMinTokens: new(1)}, log: applog.DiscardLogger, imcSessions: []*imcSession{session}, imcSystemCaches: []*imcSystemCache{cache}}
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
	session := &imcSession{id: 0, kvState: ramSessionStore()}
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
	stable := []llama.Token{1, 2, 3, 4}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "system", "content": "rules"}}}, append(slices.Clone(stable), 9), stable, []llama.Token{1, 2}, time.Now())

	if result.imcSystemCache != nil || cache.activeRestores != 0 {
		t.Error("System cache was restored while its allocation was being rebuilt")
	}
	m.imcReleaseReservation(session.id)
}

func TestProcessIMCTokenPlanChecksAllCurrentCachesBeforeSystem(t *testing.T) {
	current := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1},
		totalTokensCached: 1,
		kvState:           populatedTestSessionStore(t),
	}
	destination := &imcSession{id: 1, cachedTokens: []llama.Token{8}, totalTokensCached: 1, kvState: populatedTestSessionStore(t)}
	checkpoint := &imcSystemCache{id: 0, cachedTokens: []llama.Token{1, 2, 3}, kvState: populatedTestSessionStore(t)}
	m := Model{
		cfg:             Config{PtrCacheMinTokens: new(1)},
		log:             applog.DiscardLogger,
		imcSessions:     []*imcSession{current, destination},
		imcSystemCaches: []*imcSystemCache{checkpoint},
	}

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
		id:                0,
		cachedTokens:      []llama.Token{1, 2, 3},
		totalTokensCached: 3,
		kvState:           populatedTestSessionStore(t),
	}
	m := Model{
		cfg:             Config{PtrCacheMinTokens: new(1)},
		log:             applog.DiscardLogger,
		imcSessions:     []*imcSession{session},
		imcSystemCaches: []*imcSystemCache{{id: 0, cachedTokens: []llama.Token{1, 2}, kvState: populatedTestSessionStore(t)}},
	}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, []llama.Token{1, 2, 3, 4, 5}, []llama.Token{1, 2, 3, 4}, []llama.Token{1, 2}, time.Now())

	if result.cacheIdx != 3 {
		t.Errorf("cacheIdx = %d, want longer current prefix 3", result.cacheIdx)
	}
	if !slices.Equal(session.cachedTokens, []llama.Token{1, 2, 3}) {
		t.Error("planner swapped in shorter checkpoint instead of retaining current state")
	}
	if result.imcSystemBoundaryTokens != 0 {
		t.Errorf("imcSystemBoundaryTokens = %d, want immutable existing System cache", result.imcSystemBoundaryTokens)
	}
}

func TestProcessIMCTokenPlanReservesExactMatch(t *testing.T) {
	d := D{"messages": []D{{"role": "user", "content": "x"}}}
	session := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1, 2},
		totalTokensCached: 2,
		cachedMsgCount:    1,
		kvState:           populatedTestSessionStore(t),
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
	session.cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

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
				{id: 0, cachedTokens: slices.Clone(tt.cached), totalTokensCached: len(tt.cached), kvState: populatedTestSessionStore(t)},
				{id: 1, kvState: ramSessionStore()},
			}
			m := Model{
				cfg:         Config{PtrCacheMinTokens: &tt.cacheMin},
				log:         applog.DiscardLogger,
				imcSessions: sessions,
			}
			sessions[0].cachedRenderInputHash, _ = m.imcRenderFingerprint(d, dMessages(d))

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
			{id: 0, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, kvState: populatedTestSessionStore(t)},
		},
	}
	m.imcSessions[0].cachedRenderInputHash, _ = m.imcRenderFingerprint(priorD, dMessages(priorD))

	result := m.processIMCTokenPlan(context.Background(), currentD, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())
	if result.imcMatchKind != "rebuild" {
		t.Errorf("imcMatchKind: got %q, want %q", result.imcMatchKind, "rebuild")
	}
	if result.err != nil {
		t.Errorf("err: got %v, want nil", result.err)
	}
}

func populatedTestSessionStore(t testing.TB) SessionStore {
	t.Helper()
	store := ramSessionStore()
	buf := prepareTestStore(t, store, 1)
	buf[0] = 1
	commitTestStore(t, store, 1)
	return store
}
