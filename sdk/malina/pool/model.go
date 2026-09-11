package pool

import "time"

// Model status values match the other typed model pools.
const (
	ModelStatusLoaded  = "loaded"
	ModelStatusLoading = "loading"
)

// ModelDetail describes a Malina model represented by the pool.
type ModelDetail struct {
	ID                string
	Backend           string
	ModelFamily       string
	Size              int64
	VRAMTotal         int64
	Slots             int
	ExpiresAt         time.Time
	ActiveGenerations int
	Status            string
}
