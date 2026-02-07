package cache

import "time"

// ModelDetail provides details for the models in the cache.
type ModelDetail struct {
	ID            string
	OwnedBy       string
	ModelFamily   string
	Size          int64
	VRAMTotal     int64
	SlotMemory    int64
	ExpiresAt     time.Time
	ActiveStreams int
}
