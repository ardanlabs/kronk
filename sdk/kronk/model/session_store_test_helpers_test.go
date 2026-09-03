package model

import (
	"context"
	"testing"
)

type resetErrorStore struct {
	err error
}

func (s *resetErrorStore) Len() int                                     { return 0 }
func (s *resetErrorStore) Cap() int                                     { return 0 }
func (s *resetErrorStore) Bytes(context.Context) ([]byte, error)        { return nil, nil }
func (s *resetErrorStore) Prepare(context.Context, int) ([]byte, error) { return nil, nil }
func (s *resetErrorStore) Commit(context.Context, int) error            { return nil }
func (s *resetErrorStore) Reset(context.Context) error                  { return s.err }
func (s *resetErrorStore) Close() error                                 { return nil }

type shortPrepareStore struct {
	resetErrorStore
}

func (*shortPrepareStore) Prepare(_ context.Context, size int) ([]byte, error) {
	return make([]byte, max(size-1, 0)), nil
}

func prepareTestStore(t testing.TB, store SessionStore, size int) []byte {
	t.Helper()
	buf, err := store.Prepare(context.Background(), size)
	if err != nil {
		t.Fatalf("Prepare(%d) error = %v, want nil", size, err)
	}
	return buf
}

func commitTestStore(t testing.TB, store SessionStore, n int) {
	t.Helper()
	if err := store.Commit(context.Background(), n); err != nil {
		t.Fatalf("Commit(%d) error = %v, want nil", n, err)
	}
}

func resetTestStore(t testing.TB, store SessionStore) {
	t.Helper()
	if err := store.Reset(context.Background()); err != nil {
		t.Fatalf("Reset() error = %v, want nil", err)
	}
}

func bytesTestStore(t testing.TB, store SessionStore) []byte {
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
