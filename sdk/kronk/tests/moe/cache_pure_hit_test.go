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
		PtrNBatch:           new(2048),
		PtrNUBatch:          new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(1),
		PtrIncrementalCache: new(true),
		PtrCacheMinTokens:   new(1), // exercise IMC even on tiny prompts.
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		skipsBefore := readIMCSnapshotSkippedTotal(t)

		d := model.D{
			"messages": []model.D{
				{"role": "system", "content": "You are a helpful assistant."},
				{"role": "user", "content": "Echo back the word: Red"},
			},
			"max_tokens": 256,
		}

		ch1, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}
		_, content1, err := testlib.DrainChat(ctx, ch1)
		if err != nil || content1 == "" {
			t.Fatalf("turn 1: err=%v content empty=%v", err, content1 == "")
		}

		ch2, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}
		_, content2, err := testlib.DrainChat(ctx, ch2)
		if err != nil || content2 == "" {
			t.Fatalf("turn 2 (pure hit): err=%v content empty=%v", err, content2 == "")
		}

		skipsAfter := readIMCSnapshotSkippedTotal(t)
		if skipsAfter <= skipsBefore {
			t.Errorf("imc_snapshot_skipped_total did not increment: before=%v after=%v", skipsBefore, skipsAfter)
		}

		t.Logf("turn 1: %q", content1)
		t.Logf("turn 2: %q (pure hit, skip fired)", content2)
	})
}
