package moe_test

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// readIMCSnapshotSkippedTotal aggregates the imc_snapshot_skipped_total
// counter across all model_id labels.
func readIMCSnapshotSkippedTotal(t *testing.T) float64 {
	t.Helper()

	families, err := metrics.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var total float64
	for _, fam := range families {
		if fam.GetName() != "imc_snapshot_skipped_total" {
			continue
		}
		for _, mtr := range fam.GetMetric() {
			if c := mtr.GetCounter(); c != nil {
				total += c.GetValue()
			}
		}
	}
	return total
}

// Test_CacheIMCMoEPureHitSnapshotSkip exercises the Part A optimization on
// an MoE model. The pure-hit snapshot-skip path is identical between Dense
// and MoE targets — both have only attention KV state in the externalized
// session — but this test guards against any MoE-specific assumption
// creeping into the predicate.
func Test_CacheIMCMoEPureHitSnapshotSkip(t *testing.T) {
	cfg := model.Config{
		ModelFiles:          testlib.MPMoEVision.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(1),
		PtrIncrementalCache: new(true),
		PtrCacheMinTokens:   new(1), // exercise IMC even on tiny prompts.
		Speculation:         model.SpeculationDisabled,
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "MoE", "Attention-only", "gemma4")
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		skipsBefore := readIMCSnapshotSkippedTotal(t)

		d := testlib.StateRequest("ALPHA", "DELTA", 101)

		ch1, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}
		resp1, content1, err := testlib.DrainStateChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}
		if err := testlib.CheckStateReply(resp1, content1, "ALPHA"); err != nil {
			t.Fatalf("turn 1: %v", err)
		}
		if cached := resp1.Usage.PromptTokensDetails.CachedTokens; cached != 0 {
			t.Errorf("cold request: got %d cached tokens, want 0", cached)
		}

		ch2, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}
		resp2, content2, err := testlib.DrainStateChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}
		if err := testlib.CheckStateReply(resp2, content2, "ALPHA"); err != nil {
			t.Fatalf("turn 2: %v", err)
		}
		if resp2.Usage.PromptTokensDetails.CachedTokens == 0 {
			t.Error("pure hit did not report cached prompt tokens")
		}
		if content2 != content1 {
			t.Errorf("pure hit: got %q, want cold response %q", content2, content1)
		}

		skipsAfter := readIMCSnapshotSkippedTotal(t)
		if skipsAfter <= skipsBefore {
			t.Errorf("imc_snapshot_skipped_total did not increment: before=%v after=%v", skipsBefore, skipsAfter)
		}

		t.Logf("turn 1: %q", content1)
		t.Logf("turn 2: %q (pure hit, skip fired)", content2)
	})
}
