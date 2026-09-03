// Package kvstorage defines the storage contract used for externalized IMC
// session state.
package kvstorage

import (
	"context"
	"fmt"
)

// Set of known session-storage kinds.
var kinds = make(map[string]Kind)

// The set of session-storage kinds that can be used.
var (
	// RAM identifies the built-in in-process RAM backend.
	RAM = newKind("ram")
)

// =============================================================================

// Kind identifies a session-storage backend in external configuration.
type Kind struct {
	value string
}

func newKind(value string) Kind {
	kind := Kind{value: value}
	kinds[value] = kind
	return kind
}

// String returns the name of the kind.
func (k Kind) String() string {
	return k.value
}

// Equal provides support for the go-cmp package and testing.
func (k Kind) Equal(k2 Kind) bool {
	return k.value == k2.value
}

// IsZero reports whether the kind is unset.
func (k Kind) IsZero() bool {
	return k.value == ""
}

// MarshalText provides support for logging and serialization.
func (k Kind) MarshalText() ([]byte, error) {
	return []byte(k.value), nil
}

// UnmarshalText parses serialized text into a known Kind.
func (k *Kind) UnmarshalText(data []byte) error {
	kind, err := Parse(string(data))
	if err != nil {
		return err
	}

	*k = kind
	return nil
}

// =============================================================================

// Parse parses value and returns the corresponding Kind when it exists.
func Parse(value string) (Kind, error) {
	kind, exists := kinds[value]
	if !exists {
		return Kind{}, fmt.Errorf("invalid session-store kind %q", value)
	}

	return kind, nil
}

// MustParse parses value and returns the corresponding Kind. It panics when
// value does not identify a known kind.
func MustParse(value string) Kind {
	kind, err := Parse(value)
	if err != nil {
		panic(err)
	}

	return kind
}

// =============================================================================

// Store externalizes a single IMC session's KV cache bytes.
//
// Implementations are not required to be safe for concurrent use. Kronk
// serializes access to each store and owns it until Close. Different stores
// returned by the same Factory can be used concurrently.
type Store interface {
	// Len returns the number of valid bytes in the committed snapshot. It must
	// be a cheap metadata operation and must not perform storage I/O.
	Len() int

	// Cap returns the current staging-buffer capacity. It must be a cheap
	// metadata operation and must not perform storage I/O.
	Cap() int

	// Bytes returns the committed snapshot as a borrowed, read-only slice. The
	// caller must not modify it or retain it after the next Prepare, Commit,
	// Reset, or Close call. Implementations may reuse the slice's storage.
	Bytes(ctx context.Context) ([]byte, error)

	// Prepare starts replacing the committed snapshot and returns a writable
	// slice of exactly size bytes. Until Commit succeeds, Len must return zero.
	// The slice is borrowed: the caller may modify it only until the next
	// Prepare, Commit, Reset, or Close call and must not retain it afterward.
	// Implementations may reuse its storage.
	Prepare(ctx context.Context, size int) ([]byte, error)

	// Commit publishes the first n bytes written to the prepared slice. It must
	// return an error when n is outside the prepared slice. After Prepare, a
	// Commit error must leave Len at zero so callers cannot restore a partial
	// snapshot.
	Commit(ctx context.Context, n int) error

	// Reset clears the committed snapshot.
	Reset(ctx context.Context) error

	// Close releases the store's resources. The store must not be used again.
	Close() error
}

// Factory constructs an independent Store. Kronk invokes the factory for each
// target, draft, or checkpoint store it needs and owns each returned store. A
// factory may share concurrency-safe backend resources such as connection
// pools, quotas, and eviction accounting across the stores it returns.
type Factory func(ctx context.Context) (Store, error)
