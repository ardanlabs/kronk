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

func TestProcessIMCTokenPlanSelectsLongestCompletePrefix(t *testing.T) {
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		log: applog.DiscardLogger,
		imcSessions: []*imcSession{
			{id: 0, cachedTokens: []llama.Token{1}, totalTokensCached: 1, kvState: populatedTestSessionStore()},
			{id: 1, cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2, kvState: populatedTestSessionStore()},
			{id: 2, cachedTokens: []llama.Token{1, 9}, totalTokensCached: 2, kvState: populatedTestSessionStore()},
		},
	}

	actual := []llama.Token{1, 2, 3, 4}
	stable := []llama.Token{1, 2, 3}
	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, actual, stable, time.Now())

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
	result := m.processIMCTokenPlan(context.Background(), nil, []llama.Token{1, 2}, []llama.Token{1, 9}, time.Now())
	if result.imcTokenPlan {
		t.Fatal("imcTokenPlan = true, want false")
	}
}

func TestProcessIMCTokenPlanReservesExactMatch(t *testing.T) {
	session := &imcSession{
		id:                0,
		cachedTokens:      []llama.Token{1, 2},
		totalTokensCached: 2,
		cachedMsgCount:    1,
		kvState:           populatedTestSessionStore(),
	}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, time.Now())

	if result.imcMatchKind != "exact" {
		t.Errorf("imcMatchKind = %q, want exact", result.imcMatchKind)
	}
	if !result.imcPureHitSkipSnapshot {
		t.Error("imcPureHitSkipSnapshot = false, want true")
	}
	if !session.pending {
		t.Error("session.pending = false, want true")
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
			sessions := []*imcSession{
				{id: 0, cachedTokens: slices.Clone(tt.cached), totalTokensCached: len(tt.cached), kvState: populatedTestSessionStore()},
				{id: 1, kvState: ramSessionStore()},
			}
			m := Model{
				cfg:         Config{PtrCacheMinTokens: &tt.cacheMin},
				log:         applog.DiscardLogger,
				imcSessions: sessions,
			}

			result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "x"}}}, tt.actual, tt.stable, time.Now())
			if result.imcMatchKind != tt.wantMatch {
				t.Errorf("imcMatchKind = %q, want %q", result.imcMatchKind, tt.wantMatch)
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

func populatedTestSessionStore() SessionStore {
	store := ramSessionStore()
	buf := store.Prepare(1)
	buf[0] = 1
	store.Commit(1)
	return store
}
