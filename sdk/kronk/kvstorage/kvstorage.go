// Package kvstorage defines the storage contract used for externalized IMC
// session state.
package kvstorage

import "fmt"

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
// serializes access to each store and owns it until Close.
type Store interface {
	// Len returns the number of valid bytes in the committed snapshot.
	Len() int

	// Cap returns the current backing capacity.
	Cap() int

	// Bytes returns the committed snapshot for read access. The returned slice
	// is valid until the next Prepare, Commit, Reset, or Close call.
	Bytes() []byte

	// Prepare returns a writable slice of length size. The returned slice is
	// valid until the next Prepare, Commit, Reset, or Close call.
	Prepare(size int) []byte

	// Commit publishes the first n bytes written to the prepared slice.
	Commit(n int)

	// Reset clears the committed snapshot.
	Reset()

	// Close releases the store's resources. The store must not be used again.
	Close() error
}

// Factory constructs an independent Store. Kronk invokes the factory for each
// target, draft, or checkpoint store it needs and owns each returned store.
type Factory func() (Store, error)
