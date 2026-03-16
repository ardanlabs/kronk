package caching

import (
	"context"
	"fmt"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// NoopCache is a Cacher that does nothing. Used when caching is disabled.
type NoopCache struct{}

// NewNoop creates a NoopCache.
func NewNoop() *NoopCache {
	return &NoopCache{}
}

// ProcessCache returns the document unchanged.
func (c *NoopCache) ProcessCache(_ context.Context, d D, _ time.Time) Result {
	return Result{ModifiedD: d}
}

// ClearCaches is a no-op.
func (c *NoopCache) ClearCaches() {}

// ClearPending is a no-op.
func (c *NoopCache) ClearPending(_ int) {}

// CommitSession is a no-op.
func (c *NoopCache) CommitSession(_ Commit) {}

// InvalidateSlot is a no-op.
func (c *NoopCache) InvalidateSlot(_ int) {}

// SnapshotSlot returns an empty snapshot and false.
func (c *NoopCache) SnapshotSlot(_ int) (SlotSnapshot, bool) {
	return SlotSnapshot{}, false
}

// RestoreSPCToSeq returns an error since noop has no cached state.
func (c *NoopCache) RestoreSPCToSeq(_ llama.SeqId) error {
	return fmt.Errorf("noop-cache: no cached KV state available")
}

// HasCachedSlot always returns false.
func (c *NoopCache) HasCachedSlot(_ int) bool {
	return false
}

// SetSlotMRoPE is a no-op.
func (c *NoopCache) SetSlotMRoPE(_ int, _ bool) {}
