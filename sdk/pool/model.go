package pool

import "time"

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
}
