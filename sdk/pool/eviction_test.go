package pool

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

const GiB int64 = 1 << 30

// TestSelectEvictionVictim_SmallestFitForBudget reproduces the AGENT
// over-eviction case the user reported: with three idle models in cache
// and a small request that just exceeds the remaining budget, the policy
// must pick the smallest victim that frees enough — not the largest
// (which used to happen because the cache walks coldest-first and the
// AGENT was the LRU entry).
func TestSelectEvictionVictim_SmallestFitForBudget(t *testing.T) {
	idle := []string{
		"AGENT-LARGE", // coldest first
		"OMNI-MEDIUM",
		"BASE-SMALL",
	}

	usage := resman.Usage{
		RAMBudget: 110 * GiB,
		RAMUsed:   100 * GiB, // 10 GB free
		Reservations: []resman.LoadPlan{
			{Key: "AGENT-LARGE", RAMBytes: 44 * GiB},
			{Key: "OMNI-MEDIUM", RAMBytes: 33 * GiB},
			{Key: "BASE-SMALL", RAMBytes: 23 * GiB},
		},
	}

	req := resman.PlanRequest{Key: "Qwen3-8B", RAMBytes: 14 * GiB}

	victim, mode := selectEvictionVictim("budget", req, idle, usage)

	if victim != "BASE-SMALL" {
		t.Fatalf("victim got %q, want BASE-SMALL (smallest single-fit)", victim)
	}
	if mode != "smallest-fit" {
		t.Fatalf("mode got %q, want smallest-fit", mode)
	}
}

// TestSelectEvictionVictim_FallbackToColdestWhenNoSingleFit verifies the
// LRU fallback fires when no single idle reservation can free enough RAM
// on its own. Each of three small entries individually fails to cover the
// deficit, so we must drop back to the coldest-idle and let the outer
// loop call us again.
func TestSelectEvictionVictim_FallbackToColdestWhenNoSingleFit(t *testing.T) {
	idle := []string{"COLD", "WARM", "HOT"}

	usage := resman.Usage{
		RAMBudget: 100 * GiB,
		RAMUsed:   95 * GiB, // 5 GB free
		Reservations: []resman.LoadPlan{
			{Key: "COLD", RAMBytes: 3 * GiB},
			{Key: "WARM", RAMBytes: 3 * GiB},
			{Key: "HOT", RAMBytes: 3 * GiB},
		},
	}

	req := resman.PlanRequest{RAMBytes: 20 * GiB} // deficit = 15 GB > any single victim

	victim, mode := selectEvictionVictim("budget", req, idle, usage)

	if victim != "COLD" {
		t.Fatalf("victim got %q, want COLD (LRU fallback)", victim)
	}
	if mode != "coldest-idle" {
		t.Fatalf("mode got %q, want coldest-idle", mode)
	}
}

// TestSelectEvictionVictim_CapReasonAlwaysLRU confirms cap-driven
// evictions ignore the smallest-fit logic — cap evictions don't have a
// deficit, only a count limit, so coldest-idle is the correct policy.
func TestSelectEvictionVictim_CapReasonAlwaysLRU(t *testing.T) {
	idle := []string{"COLD-BIG", "WARM-SMALL"}

	usage := resman.Usage{
		Reservations: []resman.LoadPlan{
			{Key: "COLD-BIG", RAMBytes: 50 * GiB},
			{Key: "WARM-SMALL", RAMBytes: 5 * GiB},
		},
	}

	victim, mode := selectEvictionVictim("cap", resman.PlanRequest{RAMBytes: 1 * GiB}, idle, usage)

	if victim != "COLD-BIG" {
		t.Fatalf("victim got %q, want COLD-BIG (LRU regardless of size)", victim)
	}
	if mode != "coldest-idle" {
		t.Fatalf("mode got %q, want coldest-idle", mode)
	}
}

// TestSelectEvictionVictim_NoIdleCandidates returns empty when nothing
// is evictable. Caller translates this to ErrServerBusy.
func TestSelectEvictionVictim_NoIdleCandidates(t *testing.T) {
	victim, mode := selectEvictionVictim("budget", resman.PlanRequest{RAMBytes: 1 * GiB}, nil, resman.Usage{RAMBudget: 100 * GiB})

	if victim != "" || mode != "" {
		t.Fatalf("got victim=%q mode=%q, want empty", victim, mode)
	}
}

// TestSelectEvictionVictim_SkipsUntrackedKeys checks that an idle cache
// entry with no matching reservation in resman is treated as size-zero
// (and therefore can't single-fit a positive deficit). The smallest-fit
// pass must skip it; the LRU fallback may still pick it, but only if no
// tracked candidate fits.
func TestSelectEvictionVictim_SkipsUntrackedKeys(t *testing.T) {
	idle := []string{"GHOST-LRU", "TRACKED"}

	usage := resman.Usage{
		RAMBudget: 100 * GiB,
		RAMUsed:   95 * GiB, // 5 GB free; deficit = 5 GB
		Reservations: []resman.LoadPlan{
			{Key: "TRACKED", RAMBytes: 8 * GiB},
		},
	}

	req := resman.PlanRequest{RAMBytes: 10 * GiB}

	victim, mode := selectEvictionVictim("budget", req, idle, usage)

	if victim != "TRACKED" {
		t.Fatalf("victim got %q, want TRACKED (only sized candidate that fits deficit)", victim)
	}
	if mode != "smallest-fit" {
		t.Fatalf("mode got %q, want smallest-fit", mode)
	}
}
