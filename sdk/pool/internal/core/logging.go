package core

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

// LogResmanInit emits a one-time summary of the resource manager's
// configuration and detected hardware. Useful for confirming the pool
// is reasoning about the right machine at startup.
func (c *Core[H]) LogResmanInit(ctx context.Context) {
	u := c.resman.Usage()

	args := []any{
		"status", "resman-init",
		"budget-percent", u.BudgetPercent,
		"headroom", HumanBytes(u.HeadroomBytes),
		"gpu-count", len(u.Devices),
		"ram-budget", HumanBytes(u.RAMBudget),
		"max-models-in-pool", c.maxItems,
	}

	for _, d := range u.Devices {
		args = append(args,
			fmt.Sprintf("gpu[%d]", d.Index),
			fmt.Sprintf("name=%s type=%s total=%s budget=%s",
				d.Name, d.Type, HumanBytes(d.TotalBytes), HumanBytes(d.BudgetBytes)),
		)
	}

	c.log(ctx, "pool", args...)
}

// LogResmanUsage emits the manager's current per-device and RAM
// accounting. Called at key transitions (after reserve, after release,
// on eviction) so logs correlate with the budget at that moment.
func (c *Core[H]) LogResmanUsage(ctx context.Context, op string, extra ...any) {
	u := c.resman.Usage()

	args := make([]any, 0, 6+len(u.Devices)*2+len(extra))
	args = append(args,
		"status", "resman-usage",
		"op", op,
		"reservations", len(u.Reservations),
		"ram-used", HumanBytes(u.RAMUsed),
		"ram-budget", HumanBytes(u.RAMBudget),
	)

	for _, d := range u.Devices {
		args = append(args,
			fmt.Sprintf("gpu[%d]", d.Index),
			fmt.Sprintf("name=%s used=%s/%s (%s free)",
				d.Name,
				HumanBytes(d.UsedBytes),
				HumanBytes(d.BudgetBytes),
				HumanBytes(d.BudgetBytes-d.UsedBytes)),
		)
	}

	args = append(args, extra...)
	c.log(ctx, "pool", args...)
}

// describePlan formats a resman.LoadPlan into compact key/value pairs
// suitable for inclusion in a log call.
func describePlan(plan resman.LoadPlan) []any {
	args := []any{
		"plan-vram", HumanBytes(plan.VRAMBytes),
		"plan-ram", HumanBytes(plan.RAMBytes),
	}
	for i, alloc := range plan.Per {
		args = append(args,
			fmt.Sprintf("alloc[%d]", i),
			fmt.Sprintf("device=%s bytes=%s", alloc.Name, HumanBytes(alloc.Bytes)),
		)
	}
	return args
}

// HumanBytes formats a byte count using decimal (SI) units. The output
// is short and stable for log scraping (e.g. "12.9GB", "256MB", "0B").
func HumanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}

	return fmt.Sprintf("%.1f%s", float64(n)/float64(div), suffixes[exp])
}

// PublishMetrics refreshes the pool/resman gauges with the current
// snapshot. Cheap (one Usage() call + a few Set() operations) and
// called after every Reserve/Release event so dashboards see fresh
// data without a separate scraper goroutine.
func (c *Core[H]) PublishMetrics() {
	u := c.resman.Usage()

	pu := metrics.ResmanUsage{
		BudgetPercent: u.BudgetPercent,
		HeadroomBytes: u.HeadroomBytes,
		UnifiedMemory: u.UnifiedMemory,
		RAMTotal:      u.RAMTotal,
		RAMBudget:     u.RAMBudget,
		RAMUsed:       u.RAMUsed,
	}

	pu.Devices = make([]metrics.ResmanDeviceUsage, 0, len(u.Devices))
	for _, d := range u.Devices {
		pu.Devices = append(pu.Devices, metrics.ResmanDeviceUsage{
			Name:        d.Name,
			Type:        d.Type,
			TotalBytes:  d.TotalBytes,
			BudgetBytes: d.BudgetBytes,
			UsedBytes:   d.UsedBytes,
		})
	}

	pu.Reservations = make([]metrics.ResmanReservation, 0, len(u.Reservations))
	for _, r := range u.Reservations {
		per := make([]metrics.ResmanPerDevice, 0, len(r.Per))
		for _, a := range r.Per {
			per = append(per, metrics.ResmanPerDevice{Name: a.Name, Bytes: a.Bytes})
		}
		pu.Reservations = append(pu.Reservations, metrics.ResmanReservation{
			Key:       r.Key,
			RAMBytes:  r.RAMBytes,
			VRAMBytes: r.VRAMBytes,
			Per:       per,
		})
	}

	metrics.PublishResmanUsage(pu)

	items := int(c.itemsInPool.Load())
	metrics.SetPoolItemsInPool(items)

	// Inflight = tickets held but not yet visible in the cache.
	inflight := c.activeTicketCount() - items
	if inflight < 0 {
		inflight = 0
	}
	metrics.SetPoolInflightLoads(inflight)
}
