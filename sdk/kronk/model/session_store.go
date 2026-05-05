package model

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/disk"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
)

// SessionStoreKind values name the available SessionStore backends.
// Listed under sdk/kronk/kvstorage/<kind>/, mirroring the parser-plugin
// layout under sdk/kronk/parsers/.
const (
	// SessionStoreKindRAM keeps each session's externalized KV cache
	// bytes in a single Go-allocated []byte per session, with
	// lazy-grow / never-shrink semantics. Default backend; zero
	// configuration. Implementation: kvstorage/ram.
	SessionStoreKindRAM = "ram"

	// SessionStoreKindDisk persists each session's externalized KV
	// cache bytes to a per-session file under Config.SessionStoreDir.
	// Trades RAM for disk I/O on snapshot/restore — useful when the
	// RAM footprint of (NSeqMax × peak-conversation-KV) exceeds what
	// the host can spare. Files are removed on Model.Unload; on a
	// crash the per-session files are leaked and must be cleaned up
	// out-of-band. Implementation: kvstorage/disk.
	SessionStoreKindDisk = "disk"
)

// defaultSessionStoreKind is the backend used when Config.SessionStoreKind
// is the empty string. RAM is the only fully implemented backend today;
// future kinds (disk, nvme, network) will plug in here.
const defaultSessionStoreKind = SessionStoreKindRAM

// SessionStore externalizes a single IMC session's KV cache bytes.
//
// One instance is owned by each imcSession. Implementations are NOT
// required to be safe for concurrent use; callers serialize access via
// the per-session pending invariant (at most one in-flight request
// touches a given session's store at a time).
//
// The default implementation is the in-process RAM buffer in the
// kvstorage/ram subpackage. Future implementations (kvstorage/disk,
// kvstorage/nvme, kvstorage/network, …) may externalize state to a
// slower medium; each implementation is its own subpackage under
// sdk/kronk/kvstorage/, mirroring the parser-plugin layout under
// sdk/kronk/parsers/. The active backend is selected per-model via
// Config.SessionStoreKind.
//
// Lifetime contract for Bytes() and Prepare():
//
//   - Prepare(size) returns a writable []byte of length size into which
//     the caller (typically cgo, via llama.StateSeqGetData) fills bytes.
//     The returned slice is valid until the next Prepare/Commit/Reset
//     call on this store.
//
//   - Bytes() returns the most recently committed snapshot bytes for
//     read access (typically passed to llama.StateSeqSetData). For RAM
//     stores the returned slice aliases the internal buffer; callers
//     must not retain it past the next Prepare/Commit/Reset call.
//
// Stores that need to page bytes in from a slower medium (disk, network)
// will need a different lifetime contract; that is deferred to a later
// phase along with the first non-RAM implementation.
type SessionStore interface {
	// Len returns the number of valid bytes currently held by the store
	// (i.e., the size of the most recently committed snapshot).
	Len() int

	// Cap returns the current backing capacity. Useful for diagnostics
	// and to verify the never-shrink invariant of the RAM impl.
	Cap() int

	// Bytes returns the valid byte slice for read access. See the
	// lifetime contract on the interface doc.
	Bytes() []byte

	// Prepare returns a slice of length size, ready to be filled. See
	// the lifetime contract on the interface doc.
	Prepare(size int) []byte

	// Commit truncates the store to the actual length n after a fill
	// operation. n is clamped to [0, Cap()].
	Commit(n int)

	// Reset clears the valid contents (Len becomes 0). Implementations
	// may retain or release backing storage as appropriate; the RAM
	// impl retains the backing array for reuse on the next Prepare.
	Reset()

	// Close releases any backing storage held by the store (file
	// descriptors, on-disk files, network handles). Called once at
	// Model.Unload time, never on the per-request hot path. The RAM
	// impl is a no-op; the disk impl removes its per-session file.
	// After Close the store must not be used again.
	Close() error
}

// Compile-time assertions that the kvstorage subpackages satisfy
// SessionStore. They live in this package (rather than in each
// kvstorage subpackage) so the subpackages do not have to import
// model and can stay decoupled from the model runtime.
var (
	_ SessionStore = (*ram.Store)(nil)
	_ SessionStore = (*disk.Store)(nil)
)

// newSessionStore constructs the SessionStore backend selected by cfg.
//
// Production callers pass m.cfg so the per-model SessionStoreKind is
// honored. Tests that don't care about backend selection pass a
// zero-value Config, which falls through to defaultSessionStoreKind
// (RAM).
//
// Returns an error when the configured kind cannot be initialized
// (e.g. a future disk backend whose directory is missing or
// unwritable). Today the only supported kind is RAM, which never fails;
// the error return is reserved for Phase C and beyond.
func newSessionStore(cfg Config) (SessionStore, error) {
	switch kind := cfg.sessionStoreKind(); kind {
	case SessionStoreKindRAM:
		return ram.New(), nil
	case SessionStoreKindDisk:
		store, err := disk.New(cfg.SessionStoreDir)
		if err != nil {
			return nil, fmt.Errorf("session-store: disk: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("session-store: unknown kind %q (valid: %q, %q)", kind, SessionStoreKindRAM, SessionStoreKindDisk)
	}
}
