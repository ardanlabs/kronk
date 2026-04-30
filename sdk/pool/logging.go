package pool

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

// logResmanInit emits a one-time summary of the resource manager's
// configuration and detected hardware. Useful for confirming the pool is
// reasoning about the right machine at startup.
func (p *Pool) logResmanInit(ctx context.Context) {
	u := p.resman.Usage()

	args := []any{
		"status", "resman-init",
		"budget-percent", u.BudgetPercent,
		"headroom-bytes", u.HeadroomBytes,
		"gpu-count", len(u.Devices),
		"ram-budget", u.RAMBudget,
		"max-models-in-cache", p.maxModelsInCache,
	}

	for _, d := range u.Devices {
		args = append(args,
			fmt.Sprintf("gpu[%d]", d.Index),
			fmt.Sprintf("name=%s type=%s total=%s budget=%s",
				d.Name, d.Type, humanBytes(d.TotalBytes), humanBytes(d.BudgetBytes)),
		)
	}

	p.log(ctx, "pool", args...)
}

// logResmanUsage emits the manager's current per-device and RAM accounting.
// Called at key transitions (after reserve, after release, on eviction)
// so we can correlate logs with what the budget looked like at that moment.
func (p *Pool) logResmanUsage(ctx context.Context, op string, extra ...any) {
	u := p.resman.Usage()

	args := make([]any, 0, 6+len(u.Devices)*2+len(extra))
	args = append(args,
		"status", "resman-usage",
		"op", op,
		"reservations", len(u.Reservations),
		"ram-used", u.RAMUsed,
		"ram-budget", u.RAMBudget,
	)

	for _, d := range u.Devices {
		args = append(args,
			fmt.Sprintf("gpu[%d]", d.Index),
			fmt.Sprintf("name=%s used=%s/%s (%s free)",
				d.Name,
				humanBytes(d.UsedBytes),
				humanBytes(d.BudgetBytes),
				humanBytes(d.BudgetBytes-d.UsedBytes)),
		)
	}

	args = append(args, extra...)
	p.log(ctx, "pool", args...)
}

// describePlan formats a resman.LoadPlan into compact key/value pairs
// suitable for inclusion in a log call.
func describePlan(plan resman.LoadPlan) []any {
	args := []any{
		"plan-vram", plan.VRAMBytes,
		"plan-ram", plan.RAMBytes,
	}
	for i, alloc := range plan.Per {
		args = append(args,
			fmt.Sprintf("alloc[%d]", i),
			fmt.Sprintf("device=%s bytes=%s", alloc.Name, humanBytes(alloc.Bytes)),
		)
	}
	return args
}

// humanBytes formats a byte count using binary (IEC) units. The output is
// short and stable for log scraping (e.g. "12.0GiB", "256MiB", "0").
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}

	return fmt.Sprintf("%.1f%s", float64(n)/float64(div), suffixes[exp])
}
