package model

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// A read-only request that extends a session's prefix reuses it as an append,
// marked read-only so the slot neither commits nor snapshots the extension.
func TestProcessIMCTokenPlanReadOnlyAppend(t *testing.T) {
	session := &imcSession{id: 0, cache: &imcPrefix{cachedTokens: []llama.Token{1, 2}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{session},
	}
	publishTestIMCPrefixes(&m)

	d := D{"messages": []D{{"role": "user", "content": "x"}}}
	result := m.processIMCTokenPlan(context.Background(), d, []llama.Token{1, 2, 3, 4}, []llama.Token{1, 2, 3}, nil, time.Now(), true)

	if result.imcSession != session || result.imcMatchKind != "append" {
		t.Fatalf("session %v, match %q: want session 0 as an append", result.imcSession, result.imcMatchKind)
	}
	if !result.imcCacheReadOnly {
		t.Error("imcCacheReadOnly = false, want true")
	}
	if !session.reserved {
		t.Error("the session isn't reserved for the request")
	}
	if !slices.Equal(session.cache.cachedTokens, []llama.Token{1, 2}) || session.cache.totalTokensCached != 2 {
		t.Errorf("the plan changed the session: %v, %d", session.cache.cachedTokens, session.cache.totalTokensCached)
	}
}

// Without a complete-prefix match a read-only request runs without the cache:
// no session is reserved or reset, so another conversation's cache survives.
func TestProcessIMCTokenPlanReadOnlyMissBypasses(t *testing.T) {
	other := &imcSession{id: 0, cache: &imcPrefix{cachedTokens: []llama.Token{7, 8}, totalTokensCached: 2}, kvState: populatedTestSessionStore(t)}
	m := Model{
		cfg:         Config{PtrCacheMinTokens: new(1)},
		log:         applog.DiscardLogger,
		imcSessions: []*imcSession{other},
	}
	publishTestIMCPrefixes(&m)

	actual := []llama.Token{1, 2, 3, 4}
	d := D{"messages": []D{{"role": "user", "content": "x"}}}
	result := m.processIMCTokenPlan(context.Background(), d, actual, []llama.Token{1, 2, 3}, nil, time.Now(), true)

	if result.err != nil {
		t.Fatalf("err = %v", result.err)
	}
	if result.imcSession != nil || len(result.imcNewCacheTokens) != 0 {
		t.Errorf("a session was assigned: %v, extension %v", result.imcSession, result.imcNewCacheTokens)
	}
	if !result.imcTokenPlan || !slices.Equal(result.imcTailTokens, actual) {
		t.Errorf("the whole prompt should be prefilled as the tail: plan %v, tail %v", result.imcTokenPlan, result.imcTailTokens)
	}
	if other.reserved || !slices.Equal(other.cache.cachedTokens, []llama.Token{7, 8}) || other.kvState.Len() == 0 {
		t.Errorf("the other session was taken or reset: reserved %v, tokens %v", other.reserved, other.cache.cachedTokens)
	}

	// The same request without read-only rebuilds that session, as before.
	result = m.processIMCTokenPlan(context.Background(), d, actual, []llama.Token{1, 2, 3}, nil, time.Now(), false)
	if result.imcSession != other || result.imcMatchKind != "rebuild" || result.imcCacheReadOnly {
		t.Errorf("without read-only: session %v, match %q, read-only %v", result.imcSession, result.imcMatchKind, result.imcCacheReadOnly)
	}
}

// A read-only request never saves its session to storage: the saved snapshot
// would pair the conversation's prefix with the side request's messages.
func TestIMCSaveCompletedSkipsReadOnly(t *testing.T) {
	store := &imcTestStorage{}
	m := storageTestModel(store)
	job, _ := publishStorageTestSession(t, m, false)
	job.imcCacheReadOnly = true

	saveStorageTestSession(t, m, job, "a suggestion")

	if len(store.snapshots) != 0 {
		t.Errorf("saved %d snapshots for a read-only request, want 0", len(store.snapshots))
	}
}

func TestParseParamsCacheReadOnly(t *testing.T) {
	m := Model{log: noopLog}
	for _, tt := range []struct {
		value   any
		want    bool
		wantErr bool
	}{
		{value: true, want: true},
		{value: "true", want: true},
		{value: false, want: false},
		{value: "maybe", wantErr: true},
	} {
		d := D{"messages": []D{{"role": "user", "content": "hello"}}, "cache_read_only": tt.value}
		params, err := m.parseParams(t.Context(), d)
		if (err != nil) != tt.wantErr {
			t.Errorf("cache_read_only=%v: err = %v, want error %v", tt.value, err, tt.wantErr)
			continue
		}
		if err == nil && params.CacheReadOnly != tt.want {
			t.Errorf("cache_read_only=%v: CacheReadOnly = %v, want %v", tt.value, params.CacheReadOnly, tt.want)
		}
	}

	params, err := m.parseParams(t.Context(), D{"messages": []D{{"role": "user", "content": "hello"}}})
	if err != nil || params.CacheReadOnly {
		t.Errorf("default: CacheReadOnly = %v, err %v; want false", params.CacheReadOnly, err)
	}
}
