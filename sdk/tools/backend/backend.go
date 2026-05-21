// Package backend defines the contracts every Kronk inference backend
// (llama, whisper, …) must satisfy.
//
// Shared infrastructure — the CLI, the model server, and the resource
// pool — consumes these interfaces so a new backend can be added by
// implementing them and registering a factory, without modifying any
// cross-backend code.
//
// The package is intentionally interface-only and has no dependencies
// on any concrete backend. Concrete backends (sdk/tools/libs +
// sdk/tools/models for llama, sdk/tools/bucky/* for whisper, …) import
// this package; this package never imports them.
package backend

// Kind names a registered backend. New kinds are added as constants here
// when their concrete implementation lands. Catalog rows and config
// files carry the same string so generic dispatch code can look up the
// correct backend by name.
const (
	// KindLlama identifies the llama.cpp backend implemented by
	// sdk/tools/libs (libraries) and sdk/tools/models (catalog).
	KindLlama = "llama"

	// KindWhisper identifies the whisper.cpp backend implemented by
	// sdk/tools/bucky/libs (libraries) and sdk/tools/bucky/models
	// (catalog). The underlying FFI bindings and download primitives
	// live in github.com/ardanlabs/bucky.
	KindWhisper = "whisper"
)
