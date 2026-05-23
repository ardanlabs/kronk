package pool

import "time"

// Model status values surfaced to BUI/observability.
const (
	// ModelStatusLoaded means the model is fully loaded into the cache and
	// ready to serve requests.
	ModelStatusLoaded = "loaded"

	// ModelStatusLoading means the resource manager has reserved memory for
	// the model but the GGUF is still being read from disk and prepared by
	// llama.cpp. It is not yet servable.
	ModelStatusLoading = "loading"
)

// ModelDetail provides details for the models in the pool.
//
// Backend identifies which pool produced the entry ("kronk" for
// llama.cpp models, "bucky" for whisper models). The BUI uses it to
// tag rows and tailor the unload path.
type ModelDetail struct {
	ID            string
	Backend       string
	OwnedBy       string
	ModelFamily   string
	Size          int64
	VRAMTotal     int64
	KVCache       int64
	Slots         int
	ExpiresAt     time.Time
	ActiveStreams int
	Status        string
}
