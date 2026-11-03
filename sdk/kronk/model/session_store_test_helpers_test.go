package model

import (
	"context"
	"testing"
)

func prepareTestStore(t testing.TB, store *imcCheckpoint, size int) []byte {
	t.Helper()
	buf, err := store.Prepare(context.Background(), size)
	if err != nil {
		t.Fatalf("Prepare(%d) error = %v, want nil", size, err)
	}
	return buf
}

func commitTestStore(t testing.TB, store *imcCheckpoint, n int) {
	t.Helper()
	if err := store.Commit(context.Background(), n); err != nil {
		t.Fatalf("Commit(%d) error = %v, want nil", n, err)
	}
}

func resetTestStore(t testing.TB, store *imcCheckpoint) {
	t.Helper()
	if err := store.Reset(context.Background()); err != nil {
		t.Fatalf("Reset() error = %v, want nil", err)
	}
}

func bytesTestStore(t testing.TB, store *imcCheckpoint) []byte {
	t.Helper()
	buf, err := store.Bytes(context.Background())
	if err != nil {
		t.Fatalf("Bytes() error = %v, want nil", err)
	}
	return buf
}

func resetTestSession(t testing.TB, session *imcSession) {
	t.Helper()
	if err := resetSessionStores(context.Background(), session); err != nil {
		t.Fatalf("resetSessionStores() error = %v, want nil", err)
	}
	resetSessionMetadata(session)
}

// bindTestTokenCaches assembles the independent cache collection for fixtures.
// Callers can reorder the collection afterward without changing resident order.
func bindTestTokenCaches(m *Model) {
	m.tokenCaches = make([]*tokenCache, 0, len(m.imcSessions))
	for _, session := range m.imcSessions {
		cache := session.cache
		if cache == nil {
			cache = &tokenCache{id: session.id}
			session.cache = cache
		}
		cache.session = session
		m.tokenCaches = append(m.tokenCaches, cache)
	}
}
