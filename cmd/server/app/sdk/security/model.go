package security

import "time"

// Key represents a key in the system.
type Key struct {
	ID      string
	Created time.Time
}
