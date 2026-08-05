// Package kvstorage defines the storage contract used for externalized IMC
// session state.
package kvstorage

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
