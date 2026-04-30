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
type ModelDetail struct {
	ID            string
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
