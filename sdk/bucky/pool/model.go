package pool

import "time"

// Model status values surfaced to BUI/observability. Values mirror
// the kronk pool's status strings so callers can render mixed
// kronk/bucky listings without switching on backend.
const (
	ModelStatusLoaded  = "loaded"
	ModelStatusLoading = "loading"
)

// ModelDetail describes a single bucky (whisper) model from the pool's
// point of view. Field semantics intentionally match
// sdk/kronk/pool.ModelDetail so the BUI's "Loaded Models" table can
// render both backends through a single response shape.
//
// Bucky has no separate KV cache slot the way llama.cpp does, and its
// concurrency is a single in-process state pool rather than configurable
// "slots", so KVCache stays zero and Slots is reported as 1 for
// display parity.
type ModelDetail struct {
	ID            string
	Backend       string
	Size          int64
	VRAMTotal     int64
	ExpiresAt     time.Time
	ActiveStreams int
	Status        string

	// ModelType reflects whisper's reported architecture ("tiny",
	// "base", "small", "large-v3", …). Surfaced under ModelFamily on
	// the wire so the BUI can show something meaningful in the
	// Family column.
	ModelType string

	// Multilingual reports whether the underlying ggml file accepts
	// languages other than English. The BUI exposes this through
	// the Family/notes column so users can tell base-en apart from
	// base.
	Multilingual bool
}
