// Package imc defines persistence contracts for Incremental Message Cache
// sessions. Applications supply a Store through model.WithStorage; this package
// does not provide a disk backend.
//
// RAM remains the owner of working checkpoints. Storage persists complete
// snapshots and returns candidates; the model owns exact prefix verification,
// compatibility checks, RAM admission, and native sequence restoration. A SHA
// match alone never authorizes restoration.
//
// Persistence is best effort. A storage failure must not fail inference or
// invalidate a usable RAM checkpoint. The latest successful save is retained
// per logical session; historical snapshot retention is not part of this
// contract. Compression, integrity checks, and physical layout belong to the
// backend, not the model.
package imc

import (
	"context"
	"errors"
)

var (
	// ErrConflict indicates that a save would replace the same or a newer
	// revision. The caller must not blindly retry it with a larger revision.
	ErrConflict = errors.New("imc: conflicting session revision")

	// ErrCorrupt indicates that saved data failed integrity or length checks.
	ErrCorrupt = errors.New("imc: corrupt session")

	// ErrVersion indicates that a record schema is unsupported. Native model
	// compatibility is checked separately by the model, not by the backend.
	ErrVersion = errors.New("imc: unsupported record version")
)

// Store persists the latest complete snapshot for each Key. Implementations
// must support concurrent calls and distinguish sessions by Model and ID.
// Applications manage namespacing and access isolation in their supplied Store.
//
// The application owns the Store's lifetime. A concrete backend may provide its
// own Close method; model unload must neither close a shared Store nor delete
// saved sessions. This contract does not require multi-process access: a disk
// backend may instead reject a second process opening its storage directory.
//
// Missing sessions or revisions return errors matching fs.ErrNotExist. Other
// errors preserve wrapped I/O and context causes. Invalid queries return an
// error rather than silently widening the search or choosing an unbounded limit.
type Store interface {
	// Save synchronously consumes one complete snapshot. All slices and JSON
	// documents are borrowed read-only until Save returns and must not be
	// retained. The caller protects them from concurrent mutation. A backend
	// must not require another checkpoint-sized RAM copy; it may stream and
	// compress the existing bytes directly into storage.
	//
	// Revisions are positive and strictly increasing for a Key, including
	// across reloads. Save rejects a revision no newer than the current one
	// with ErrConflict. Keys are never reused for unrelated conversations.
	//
	// Record, target, optional draft, and media are published atomically.
	// Never overwrite the only valid snapshot in place. A failure before
	// publication preserves the previous save; incomplete data is never
	// discoverable. After publication, existing readers remain valid while
	// old data is reclaimed. Post-publication errors can have an uncertain
	// commit outcome; callers can Open the attempted revision to inspect it.
	// Successful return means the generation has met the backend's documented
	// crash-durability guarantees, not merely been queued for a later write.
	Save(ctx context.Context, snapshot Snapshot) error

	// Find returns at most query.Limit eligible candidate summaries, ordered by
	// SavedAt descending and ID ascending for ties. It must not read checkpoint
	// blobs, complete requests, or media payloads. The model uses each summary's
	// prefix length and SHA to match incoming input locally, then Opens and
	// exactly verifies matching candidates before reuse. Storage does not
	// perform prefix matching or select the winning session.
	Find(ctx context.Context, query FindQuery) ([]Summary, error)

	// Open pins exactly ref.Revision, never silently substituting a newer
	// snapshot. It may return fs.ErrNotExist if the latest-only snapshot was
	// replaced or deleted since discovery. The caller closes every handle.
	Open(ctx context.Context, ref Ref) (Session, error)

	// List returns a page of conversation summaries, not checkpoint history.
	// It must not load checkpoint blobs, full requests, or media payloads.
	List(ctx context.Context, query ListQuery) (Page, error)

	// Delete removes a session from discovery and future opens. Deleting an
	// absent Key succeeds. Already opened handles remain readable until closed.
	// Deletion does not cancel in-flight saves; the application must coordinate
	// those if it requires the session to remain deleted.
	Delete(ctx context.Context, key Key) error
}

// Session is a read-only handle to one pinned, complete saved revision. It is
// not an execution slot, mutable RAM buffer, or native model context. Methods
// are called serially on each handle; different handles may be used concurrently.
type Session interface {
	// Record returns owned metadata, including messages and assistant text,
	// without reading checkpoint blobs. Mutating the result must not change
	// stored data or later Record results. Conversation readback does not
	// require loading a model or accepting its native checkpoint compatibility.
	Record() Record

	// Sizes returns uncompressed byte lengths without reading payloads. Target
	// is positive; draft is zero if absent. Media sizes are in saved input order
	// and the returned slice is caller-owned. Before allocating, callers check
	// sizes against application limits and platform bounds.
	Sizes() (target, draft int64, media []int64)

	// Read fills caller-owned buffers with the complete target, optional draft,
	// and media. Buffer lengths and media count must match Sizes. Empty draft
	// means no independent draft state, including shared-target-KV MTP.
	// The backend verifies integrity and returns errors for short or corrupt
	// data, incorrect buffer sizes, and cancellation. Callers discard all
	// buffers on error. Storage does not retain them or allocate another
	// full-sized checkpoint buffer.
	Read(ctx context.Context, target, draft []byte, media [][]byte) error

	// Close releases the pinned revision without deleting saved session data.
	// All reads must finish before Close; no method is used afterward.
	Close() error
}
