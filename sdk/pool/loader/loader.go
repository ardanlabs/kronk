// Package loader defines the contracts each Kronk inference backend must
// satisfy to plug into the generic pool core.
//
// The pool core (sdk/pool/internal/core) handles caching, eviction,
// budget reservation, and concurrent-load deduplication generically over
// any handle type. The Loader interface defined here is the seam: a
// backend provides Plan + Load + Display, and the core does the rest.
//
// This package has no dependencies on any concrete backend; concrete
// backends import it.
package loader

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

// Handle is a backend-specific live model handle the pool tracks in its
// cache. The pool itself is opaque about what the handle does — it only
// needs to know when the handle is in active use (so eviction does not
// pull the rug out from under in-flight requests) and how to release
// the underlying runtime resources.
type Handle interface {
	// ActiveStreams returns the number of in-flight requests currently
	// using this handle. The pool refuses to evict handles whose
	// ActiveStreams value is non-zero.
	ActiveStreams() int

	// Unload releases the underlying runtime resources. Called by the
	// pool's eviction callback on TTL expiry, capacity overflow, or
	// explicit invalidation.
	Unload(ctx context.Context) error
}

// LoadRequest is what the pool passes to the loader for both planning
// and loading.
type LoadRequest struct {
	// ModelID is the canonical model id used for catalog lookups (VRAM
	// calculation, file resolution). Always set.
	ModelID string

	// Key is the cache and reservation key. For catalog acquisitions
	// Key equals ModelID; for "custom" acquisitions (e.g. the llama
	// playground) Key is a composite "<modelID>/playground/<session>".
	Key string

	// Custom carries an optional backend-specific configuration object
	// for "custom" acquisitions. For catalog-driven acquisitions Custom
	// is nil and the loader resolves config itself from ModelID.
	Custom any
}

// Display is the per-handle observability data ModelStatus surfaces.
// Backends with no concept of a particular field return 0 for that
// field (Slots returns 1).
type Display struct {
	// KVCache is the per-slot KV cache size in bytes. Backends without
	// a KV cache (whisper) return 0.
	KVCache int64

	// VRAMTotal is the total VRAM (or unified-memory) footprint in
	// bytes the handle is currently occupying.
	VRAMTotal int64

	// Slots is the maximum number of concurrent sequences this handle
	// can serve. Backends without a slot concept return 1.
	Slots int
}

// Loader is a backend's pool-facing surface. Each backend (llama,
// whisper, …) implements this interface in its high-level SDK package.
// The type parameter H is the concrete handle the backend produces
// (e.g. *kronk.Kronk for llama). Tying the loader to a concrete handle
// type at the interface boundary keeps the generic pool core free of
// runtime type assertions.
type Loader[H Handle] interface {
	// Plan predicts the memory footprint required to load the model
	// and returns a resman.PlanRequest. The pool calls Plan once per
	// Acquire before reserving budget with the resource manager.
	Plan(ctx context.Context, req LoadRequest) (resman.PlanRequest, error)

	// Load constructs and initializes the backend-specific handle. It
	// is called after a successful budget reservation; the loader must
	// not perform its own budget accounting.
	Load(ctx context.Context, req LoadRequest) (H, error)

	// Display returns the per-handle observability data ModelStatus
	// surfaces. Called on every status query; implementations should
	// be cheap.
	Display(h H, modelID string) Display
}
