package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/maypok86/otter/v2"
)

// selectEvictionVictim implements the pure choice rule for picking a
// pool entry to evict. Extracted from evictOneIdle so it can be unit
// tested without a live cache or resource manager.
//
// idleColdestFirst contains evictable cache keys in coldest (LRU)
// order. usage is the resource manager's current accounting, used for
// sizing each reservation. Returns the chosen victim key plus a
// selection mode label ("smallest-fit" or "coldest-idle") for
// observability. Returns "", "" when there is no idle candidate.
func selectEvictionVictim(reason string, req resman.PlanRequest, idleColdestFirst []string, usage resman.Usage) (string, string) {
	if len(idleColdestFirst) == 0 {
		return "", ""
	}

	if reason == "budget" && (req.RAMBytes > 0 || req.VRAMBytes > 0) {
		// How much we still need to free to admit req.
		ramDeficit := max(req.RAMBytes-(usage.RAMBudget-usage.RAMUsed), 0)

		// Index reservations by key for O(1) lookup.
		sizes := make(map[string]resman.LoadPlan, len(usage.Reservations))
		for _, r := range usage.Reservations {
			sizes[r.Key] = r
		}

		// Smallest single-fit: among idle entries that the manager
		// actually tracks, pick the smallest whose RAM release covers
		// the deficit. This avoids freeing 44 GB to satisfy a 4 GB
		// shortfall when a 25 GB idle candidate would have done.
		var bestKey string
		var bestScore int64 = -1
		for _, key := range idleColdestFirst {
			s, ok := sizes[key]
			if !ok {
				continue
			}
			if ramDeficit > 0 && s.RAMBytes < ramDeficit {
				continue
			}
			// Dominant-axis size: RAM dominates on unified memory;
			// on split-budget hardware VRAM matters too.
			score := max(s.VRAMBytes, s.RAMBytes)
			if bestScore < 0 || score < bestScore {
				bestScore = score
				bestKey = key
			}
		}

		if bestKey != "" {
			return bestKey, "smallest-fit"
		}
	}

	// LRU fallback: coldest entry that is still idle. Also the path
	// for "cap"-driven evictions where there is no specific deficit.
	return idleColdestFirst[0], "coldest-idle"
}

// evictOneIdle selects an idle pool entry to evict and waits for the
// eviction callback to release the reservation. Returns ErrServerBusy
// when no idle victim is available.
//
// Selection policy:
//   - When reason is "budget" and req has a non-zero footprint, prefer
//     the SMALLEST idle reservation whose RAMBytes (and VRAMBytes if
//     relevant) individually frees enough memory to admit the request.
//     This avoids the pathological "evict a 44 GB AGENT model to make
//     room for a 4 GB deficit" case — keeping expensive-to-reload
//     models warm whenever a smaller idle candidate would have
//     sufficed.
//   - When no single victim fits the deficit (or for cap-driven
//     evictions where there is no specific deficit), fall back to the
//     coldest idle entry — the historical LRU behaviour.
//
// In both cases the choice respects: never evict newKey itself, and
// never evict an entry with active streams.
func (c *Pool[H]) evictOneIdle(ctx context.Context, newKey, reason string, req resman.PlanRequest) error {
	const pollInterval = 25 * time.Millisecond
	const maxWait = 60 * time.Second

	// Walk the cache in coldest-first order to preserve LRU semantics
	// for the fallback path, recording only entries that are evictable
	// (non-self, no active streams).
	var idleColdestFirst []string
	for entry := range c.cache.Coldest() {
		if entry.Key == newKey {
			continue
		}
		if entry.Value.ActiveStreams() != 0 {
			continue
		}
		idleColdestFirst = append(idleColdestFirst, entry.Key)
	}

	usage := c.resman.Usage()
	victim, victimSelectionMode := selectEvictionVictim(reason, req, idleColdestFirst, usage)

	if victim == "" {
		return ErrServerBusy
	}

	c.log(ctx, "acquire",
		"status", "evict-before-load",
		"reason", reason,
		"selection", victimSelectionMode,
		"victim", victim,
		"items-in-pool", c.itemsInPool.Load(),
		"max-models-in-pool", c.maxItems,
	)

	metrics.AddPoolEvictBeforeLoad()
	metrics.AddPoolEviction(reason, victimSelectionMode)

	evictStart := time.Now()
	c.cache.Invalidate(victim)

	deadline := time.Now().Add(maxWait)
	for {
		if !c.hasTicket(victim) && int(c.itemsInPool.Load()) < c.maxItems+1 {
			// The eviction callback has run (ticket released) and the
			// counter has been decremented or is at its previous level.
			// We use cap+1 to allow the counter check to succeed even
			// if another acquire raced; the loop in reserveWithEviction
			// will recheck on the next iteration.
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("evict-one-idle: timeout waiting for victim[%s] to unload", victim)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	metrics.ObservePoolEvictWait(time.Since(evictStart))

	c.log(ctx, "acquire",
		"status", "evict-before-load-complete",
		"victim", victim,
		"items-in-pool", c.itemsInPool.Load(),
	)

	return nil
}

// eviction is the otter cache callback fired when a key is removed
// from the cache (TTL expiry, capacity overflow, explicit
// invalidation, or replacement). It unloads the handle, releases the
// reservation, and decrements the counter.
func (c *Pool[H]) eviction(event otter.DeletionEvent[string, H]) {
	const unloadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), unloadTimeout)
	defer cancel()

	c.log(ctx, "pool eviction", "key", event.Key, "cause", event.Cause.String(), "cause-code", int(event.Cause), "was-evicted", event.WasEvicted(), "active-streams", event.Value.ActiveStreams())

	// If there are active streams and this was an automatic eviction
	// (not a replacement from our own Set call below), re-insert the
	// model to prevent eviction. WasEvicted() returns false for
	// CauseReplacement and CauseInvalidation.
	if event.Value.ActiveStreams() > 0 && event.WasEvicted() {
		c.log(ctx, "pool eviction prevented", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		c.cache.Set(event.Key, event.Value)
		return
	}

	// If this is a replacement event (from our Set above) and there
	// are still active streams, just return without unloading - the
	// handle is still in the pool. For invalidation (shutdown), we
	// still need to unload since the pool is being cleared.
	if event.Value.ActiveStreams() > 0 && event.Cause != otter.CauseInvalidation {
		c.log(ctx, "pool eviction skipped (replacement with active streams)", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		return
	}

	c.log(ctx, "pool eviction", "key", event.Key, "status", "unload-started", "active-streams", event.Value.ActiveStreams())

	unloadStart := time.Now()

	if err := event.Value.Unload(ctx); err != nil {
		c.log(ctx, "pool eviction", "key", event.Key, "ERROR", err)
	}

	unloadDur := time.Since(unloadStart)
	metrics.ObservePoolUnloadDuration(event.Key, unloadDur)

	// Track the eviction reason as observed by the otter cache. The
	// "evict-before-load" path also fires this callback (via
	// Invalidate), so this counter is the union of TTL, replacement,
	// invalidation, and capacity-driven evictions.
	metrics.AddPoolEviction(otterCauseLabel(event.Cause), "")

	c.log(ctx, "pool eviction", "key", event.Key, "status", "unload-finished")

	metrics.ClearVRAM(event.Key)
	metrics.ClearPoolActiveStreams(event.Key)

	// Decrement BEFORE takeTicket. evictOneIdle's wait predicate reads
	// hasTicket() first and then itemsInPool, and uses ticket-absence
	// as a signal that the counter is up to date. Sequencing the
	// decrement before takeTicket means the mutex release inside
	// takeTicket publishes the new counter value to any observer that
	// subsequently sees hasTicket()==false — keeping the two pieces of
	// state consistent from the waiter's point of view.
	c.itemsInPool.Add(-1)

	if ticket, ok := c.takeTicket(event.Key); ok {
		c.resman.Release(ticket)
		c.log(ctx, "pool eviction",
			"status", "reservation-released",
			"key", event.Key,
		)
		c.LogResmanUsage(ctx, "post-release", "key", event.Key)
	}

	c.PublishMetrics()
}

// otterCauseLabel maps the otter eviction cause to a metrics label.
func otterCauseLabel(cause otter.DeletionCause) string {
	switch cause {
	case otter.CauseExpiration:
		return "ttl"
	case otter.CauseOverflow:
		return "cap"
	case otter.CauseReplacement:
		return "replacement"
	case otter.CauseInvalidation:
		return "invalidation"
	default:
		return "unknown"
	}
}
