// Package core is the generic, backend-agnostic engine that powers
// every Kronk model pool.
//
// Core[H loader.Handle] owns the cache, eviction policy, budget
// reservations, ticket bookkeeping, and concurrent-load deduplication.
// It is plugged with a backend-specific loader.Loader[H] that knows how
// to predict a model's footprint, materialize the handle, and surface
// per-handle observability data.
//
// Public pool types (sdk/pool.Pool for llama, future sdk/bucky/pool.Pool
// for whisper) wrap a Core[H] for their concrete handle type and re-
// export only the operations their backend cares about. This package is
// internal so wrappers control the supported handle types.
package core

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/pool/loader"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
	"github.com/maypok86/otter/v2"
	"golang.org/x/sync/singleflight"
)

// ErrServerBusy is returned when every cached entry is busy with active
// streams and no idle victim is available for eviction.
var ErrServerBusy = errors.New("server busy: all model slots have active requests")

// Config carries the non-generic settings used to construct a Core.
//
// Loader is supplied as a separate argument to New so callers do not
// have to repeat the type parameter on Config.
type Config struct {
	// Log is the logger used for all core events. Required.
	Log applog.Logger

	// Resman is the resource manager the core charges reservations
	// against. Multiple cores can share one manager so a single
	// machine-wide budget covers every registered backend's pool.
	Resman *resman.Manager

	// MaxItems is the safety-net cap on the number of distinct entries
	// the core keeps, independent of the byte budget.
	MaxItems int

	// TTL is the duration an entry can live in the cache without being
	// accessed before the cache evicts it.
	TTL time.Duration
}

// Core is the generic pool engine.
type Core[H loader.Handle] struct {
	log         applog.Logger
	loader      loader.Loader[H]
	cache       *otter.Cache[string, H]
	itemsInPool atomic.Int32
	maxItems    int
	loadGroup   singleflight.Group
	resman      *resman.Manager
	ticketsMu   sync.Mutex
	tickets     map[string]resman.Ticket
}

// New constructs a Core wired to the supplied loader. The caller owns
// the resource manager passed in cfg.Resman and is responsible for its
// lifecycle.
func New[H loader.Handle](cfg Config, l loader.Loader[H]) (*Core[H], error) {
	if cfg.Log == nil {
		return nil, errors.New("core: new: log is required")
	}
	if cfg.Resman == nil {
		return nil, errors.New("core: new: resman is required")
	}
	if l == nil {
		return nil, errors.New("core: new: loader is required")
	}
	if cfg.MaxItems <= 0 {
		return nil, errors.New("core: new: max-items must be > 0")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("core: new: ttl must be > 0")
	}

	c := Core[H]{
		log:      cfg.Log,
		loader:   l,
		maxItems: cfg.MaxItems,
		resman:   cfg.Resman,
		tickets:  make(map[string]resman.Ticket),
	}

	opt := otter.Options[string, H]{
		MaximumSize:      cfg.MaxItems,
		ExpiryCalculator: otter.ExpiryAccessing[string, H](cfg.TTL),
		OnDeletion:       c.eviction,
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("core: new: constructing cache: %w", err)
	}

	c.cache = cache

	metrics.SetPoolMaxItemsInPool(cfg.MaxItems)
	c.PublishMetrics()

	return &c, nil
}

// ResourceManager returns the manager the core reserves against.
func (c *Core[H]) ResourceManager() *resman.Manager {
	return c.resman
}

// ItemsInPool reports how many entries are currently resident.
func (c *Core[H]) ItemsInPool() int {
	return int(c.itemsInPool.Load())
}

// MaxItems reports the configured items-in-pool cap.
func (c *Core[H]) MaxItems() int {
	return c.maxItems
}

// GetExisting returns the handle for key if it is currently cached,
// without creating one.
func (c *Core[H]) GetExisting(key string) (H, bool) {
	return c.cache.GetIfPresent(key)
}

// Invalidate removes a single entry from the cache, triggering unload
// asynchronously through the eviction callback. The resource
// reservation may not be released by the time this returns; use
// InvalidateSync when a consistent post-eviction view is required.
func (c *Core[H]) Invalidate(key string) {
	c.cache.Invalidate(key)
}

// InvalidateSync invalidates a cache entry and waits for the eviction
// callback to release the underlying resource manager reservation.
func (c *Core[H]) InvalidateSync(ctx context.Context, key string) error {
	const pollInterval = 25 * time.Millisecond
	const maxWait = 60 * time.Second

	c.cache.Invalidate(key)

	deadline := time.Now().Add(maxWait)
	for {
		if !c.hasTicket(key) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("invalidate-sync: timeout waiting for key[%s] to unload", key)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// Shutdown invalidates every entry and waits for all eviction
// callbacks to drain. Honors the supplied context deadline; if none is
// set, defaults to a 45s timeout to match prior behavior.
func (c *Core[H]) Shutdown(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}

	c.cache.InvalidateAll()

	for c.itemsInPool.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.NewTimer(100 * time.Millisecond).C:
		}
	}

	return nil
}

// Coldest returns an iterator that yields cached entries in LRU
// (coldest-first) order. Used by wrappers for ModelStatus listings.
func (c *Core[H]) Coldest() iter.Seq[otter.Entry[string, H]] {
	return c.cache.Coldest()
}

// Loader returns the loader bound to this core.
func (c *Core[H]) Loader() loader.Loader[H] {
	return c.loader
}

// =============================================================================

// storeTicket records a successful reservation so the eviction callback
// can release it when the handle is unloaded.
func (c *Core[H]) storeTicket(key string, t resman.Ticket) {
	c.ticketsMu.Lock()
	defer c.ticketsMu.Unlock()
	c.tickets[key] = t
}

// takeTicket removes and returns a stored ticket. The second return
// value is false when no ticket was found for the key.
func (c *Core[H]) takeTicket(key string) (resman.Ticket, bool) {
	c.ticketsMu.Lock()
	defer c.ticketsMu.Unlock()
	t, ok := c.tickets[key]
	if ok {
		delete(c.tickets, key)
	}
	return t, ok
}

// hasTicket reports whether a ticket is still tracked for key.
func (c *Core[H]) hasTicket(key string) bool {
	c.ticketsMu.Lock()
	defer c.ticketsMu.Unlock()
	_, ok := c.tickets[key]
	return ok
}

// activeTicketCount returns the number of currently tracked tickets.
// Used by metrics publishing to compute inflight loads.
func (c *Core[H]) activeTicketCount() int {
	c.ticketsMu.Lock()
	defer c.ticketsMu.Unlock()
	return len(c.tickets)
}
