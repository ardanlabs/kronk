package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/pool/loader"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

// Acquire returns the cached handle for req.Key, loading it if
// necessary. Concurrent calls for the same key are deduplicated via
// singleflight so only one Load runs at a time.
//
// The result is annotated with "hit", "miss", "dedup", "busy", or
// "error" metrics labels matching the prior pool behavior. Callers
// (Pool wrappers) typically log additional backend-specific fields
// after Acquire returns.
func (c *Core[H]) Acquire(ctx context.Context, req loader.LoadRequest) (H, error) {
	var zero H
	start := time.Now()

	if entry, exists := c.cache.GetEntry(req.Key); exists {
		c.log(ctx, "acquire",
			"status", "cache-hit",
			"key", req.Key,
			"ttl-reset", true,
			"expires-at", entry.ExpiresAt(),
			"active-streams", entry.Value.ActiveStreams(),
		)
		metrics.AddPoolAcquire("hit")
		metrics.ObservePoolAcquireDuration("hit", time.Since(start))
		return entry.Value, nil
	}

	c.log(ctx, "acquire",
		"status", "cache-miss",
		"key", req.Key,
	)

	// Use singleflight to prevent concurrent loads of the same key.
	sfStart := time.Now()
	result, err, shared := c.loadGroup.Do(req.Key, func() (any, error) {

		// Double-check pool after acquiring the singleflight lock.
		if h, exists := c.cache.GetIfPresent(req.Key); exists {
			return h, nil
		}

		planReq, err := c.loader.Plan(ctx, req)
		if err != nil {
			metrics.AddPoolLoadFailure("plan")
			return zero, fmt.Errorf("acquire: plan: %w", err)
		}

		ticket, plan, err := c.reserveWithEviction(ctx, req.Key, planReq)
		if err != nil {
			return zero, fmt.Errorf("acquire: %w", err)
		}

		reservedArgs := append([]any{
			"status", "reserved",
			"key", req.Key,
		}, describePlan(plan)...)
		c.log(ctx, "acquire", reservedArgs...)
		c.LogResmanUsage(ctx, "post-reserve", "key", req.Key)

		h, err := c.loader.Load(ctx, req)
		if err != nil {
			c.resman.Release(ticket)
			c.log(ctx, "acquire",
				"status", "load-failed-reservation-released",
				"key", req.Key,
				"ERROR", err,
			)
			c.LogResmanUsage(ctx, "post-failed-load", "key", req.Key)
			metrics.AddPoolLoadFailure("load")
			return zero, fmt.Errorf("acquire: %w", err)
		}

		c.storeTicket(req.Key, ticket)
		c.cache.Set(req.Key, h)
		c.itemsInPool.Add(1)

		if entry, ok := c.cache.GetEntryQuietly(req.Key); ok {
			c.log(ctx, "acquire",
				"status", "cache-set",
				"key", req.Key,
				"expires-at", entry.ExpiresAt(),
				"ttl", entry.ExpiresAfter().String(),
			)
		}

		return h, nil
	})

	if shared {
		metrics.ObservePoolSingleflightWait(time.Since(sfStart))
	}

	switch {
	case err == nil && shared:
		metrics.AddPoolAcquire("dedup")
	case err == nil:
		metrics.AddPoolAcquire("miss")
	case errors.Is(err, ErrServerBusy):
		metrics.AddPoolAcquire("busy")
	default:
		metrics.AddPoolAcquire("error")
	}
	metrics.ObservePoolAcquireDuration("miss", time.Since(start))

	c.PublishMetrics()

	if err != nil {
		return zero, err
	}

	return result.(H), nil
}

// reserveWithEviction reserves the request's memory footprint with the
// resource manager, evicting idle entries to free either the budget or
// the items-in-pool cap when necessary.
//
// On success it returns the ticket and the resolved plan. On failure
// it returns ErrServerBusy if no idle victims remain, or a wrapped
// error from the resource manager / context.
func (c *Core[H]) reserveWithEviction(ctx context.Context, newKey string, req resman.PlanRequest) (resman.Ticket, resman.LoadPlan, error) {
	const maxAttempts = 64

	c.log(ctx, "reserve",
		"status", "begin",
		"key", newKey,
		"vram", HumanBytes(req.VRAMBytes),
		"ram", HumanBytes(req.RAMBytes),
		"devices", req.Devices,
		"items-in-pool", c.itemsInPool.Load(),
		"max-models-in-pool", c.maxItems,
	)

	// Reject infeasible reservations BEFORE evicting anything. Without
	// this check, a request whose footprint exceeds the total
	// configured budget (e.g. an over-spec'd context window) would
	// walk the eviction loop kicking out every loaded model in turn,
	// then fail with ErrServerBusy — leaving the user with no models
	// loaded and the pool empty. The reservation can only ever be
	// satisfied if its footprint fits inside the relevant budget when
	// nothing else is reserved.
	if err := c.checkRequestFitsBudget(newKey, req); err != nil {
		c.log(ctx, "reserve",
			"status", "infeasible",
			"key", newKey,
			"ERROR", err,
		)
		metrics.AddPoolLoadFailure("plan")
		metrics.AddResmanRejection("no_capacity")
		return resman.Ticket{}, resman.LoadPlan{}, fmt.Errorf("reserve: %w", err)
	}

	for attempt := range maxAttempts {

		// Enforce the items-in-pool cap before attempting to reserve.
		// Even when budget allows, the cap bounds how many distinct
		// entries we keep in memory.
		if int(c.itemsInPool.Load()) >= c.maxItems {
			c.log(ctx, "reserve",
				"status", "cap-hit",
				"key", newKey,
				"attempt", attempt,
				"items-in-pool", c.itemsInPool.Load(),
				"max-models-in-pool", c.maxItems,
			)
			if err := c.evictOneIdle(ctx, newKey, "cap", req); err != nil {
				c.log(ctx, "reserve",
					"status", "cap-evict-failed",
					"key", newKey,
					"ERROR", err,
				)
				metrics.AddPoolLoadFailure("evict")
				return resman.Ticket{}, resman.LoadPlan{}, err
			}
			continue
		}

		ticket, plan, err := c.resman.Reserve(req)
		if err == nil {
			c.log(ctx, "reserve",
				"status", "success",
				"key", newKey,
				"attempt", attempt,
			)
			return ticket, plan, nil
		}

		// Track every Reserve failure as a resman rejection.
		// ErrNoCapacity rejections are common during eviction loops;
		// other classes indicate misconfiguration.
		metrics.AddResmanRejection(classifyResmanError(err))

		// Only ErrNoCapacity is recoverable via eviction.
		if !errors.Is(err, resman.ErrNoCapacity) {
			c.log(ctx, "reserve",
				"status", "fatal",
				"key", newKey,
				"attempt", attempt,
				"ERROR", err,
			)
			metrics.AddPoolLoadFailure("reserve")
			return resman.Ticket{}, resman.LoadPlan{}, fmt.Errorf("reserve: %w", err)
		}

		c.log(ctx, "reserve",
			"status", "no-capacity",
			"key", newKey,
			"attempt", attempt,
			"ERROR", err,
		)
		c.LogResmanUsage(ctx, "no-capacity", "key", newKey)

		if err := c.evictOneIdle(ctx, newKey, "budget", req); err != nil {
			c.log(ctx, "reserve",
				"status", "budget-evict-failed",
				"key", newKey,
				"ERROR", err,
			)
			metrics.AddPoolLoadFailure("evict")
			return resman.Ticket{}, resman.LoadPlan{}, err
		}
	}

	c.log(ctx, "reserve",
		"status", "gave-up",
		"key", newKey,
		"max-attempts", maxAttempts,
	)
	metrics.AddPoolLoadFailure("reserve")
	return resman.Ticket{}, resman.LoadPlan{}, fmt.Errorf("reserve: gave up after %d eviction attempts", maxAttempts)
}

// checkRequestFitsBudget returns a non-nil error when the request can
// never be satisfied given the manager's current configuration — i.e.
// it asks for more bytes than the relevant budget would hold even if
// every other reservation were released. The core must refuse to evict
// in that case; otherwise it would gut the cache for nothing.
func (c *Core[H]) checkRequestFitsBudget(newKey string, req resman.PlanRequest) error {
	usage := c.resman.Usage()

	if req.RAMBytes > 0 && usage.RAMBudget > 0 && req.RAMBytes > usage.RAMBudget {
		return fmt.Errorf("request[%s] needs ram=%s but max ram budget is %s",
			newKey, HumanBytes(req.RAMBytes), HumanBytes(usage.RAMBudget))
	}

	if req.VRAMBytes > 0 {
		// When specific devices are requested, each must individually
		// fit (the manager allocates per-device, not pooled). When
		// unpinned, the request must fit on at least one device.
		if len(req.Devices) > 0 {
			for _, name := range req.Devices {
				var budget int64
				for _, d := range usage.Devices {
					if d.Name == name {
						budget = d.BudgetBytes
						break
					}
				}
				if budget > 0 && req.VRAMBytes > budget && len(req.Devices) == 1 {
					return fmt.Errorf("request[%s] needs vram=%s but device[%s] budget is %s",
						newKey, HumanBytes(req.VRAMBytes), name, HumanBytes(budget))
				}
			}
		} else {
			var maxBudget int64
			for _, d := range usage.Devices {
				if d.BudgetBytes > maxBudget {
					maxBudget = d.BudgetBytes
				}
			}
			if maxBudget > 0 && req.VRAMBytes > maxBudget {
				return fmt.Errorf("request[%s] needs vram=%s but largest device budget is %s",
					newKey, HumanBytes(req.VRAMBytes), HumanBytes(maxBudget))
			}
		}
	}

	return nil
}

// classifyResmanError maps a resman error to a metrics rejection
// reason label.
func classifyResmanError(err error) string {
	switch {
	case errors.Is(err, resman.ErrNoCapacity):
		return "no_capacity"
	case errors.Is(err, resman.ErrUnknownDevice):
		return "unknown_device"
	case errors.Is(err, resman.ErrInvalidPlan):
		return "invalid_plan"
	case errors.Is(err, resman.ErrDuplicateKey):
		return "duplicate_key"
	case errors.Is(err, resman.ErrNoGPUs):
		return "no_gpus"
	default:
		return "other"
	}
}
