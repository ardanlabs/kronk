package hybrid_test

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// readIMCSnapshotSkippedTotal reads the current value of the
// imc_snapshot_skipped_total counter across all model_id labels. Returns 0
// when the metric has not been incremented yet (the counter vec has not
// observed any labelled child).
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

// Test_CacheIMCHybridPureHitSnapshotSkip exercises the Part A pure-hit
// snapshot-skip optimization on a Hybrid (Attention + Recurrent) model.
//
// The critical hybrid-specific behavior under test: StateSeqSetData ->
// (skip StateSeqGetData) must preserve a valid recurrent (DeltaNet/SSM)
// state across pure-hit turns. If the recurrent state were not perfectly
// round-tripped by the previous request's snapshot, the second pure-hit
// turn would produce gibberish.
//
// Pattern:
//  1. Turn 1 builds an IMC prefix from [system, user].
//  2. Turn 2 repeats the same [system, user] verbatim — a pure hit on the
//     committed session. The cache boundary does not advance.
//  3. With IMCPureHitSnapshotSkip enabled, the post-restore snapshot in
//     startSlot is skipped. The session's externalized KV bytes (target
//     attention KV + recurrent state) remain whatever the turn-1 snapshot
//     wrote.
//
// Assertions:
//   - Both turns produce non-empty content (recurrent state survived restore).
//   - The skip counter incremented at least once after turn 2.
func Test_CacheIMCHybridPureHitSnapshotSkip(t *testing.T) {
	cfg := model.Config{
		ModelFiles:          testlib.MPHybridVision.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrNBatch:           new(2048),
		PtrNUBatch:          new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(1),
		PtrIncrementalCache: new(true),
	}

	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		systemPrompt := "You are a helpful assistant. Follow instructions precisely."

		// Baseline counter value: other tests in this binary may have
		// incremented it already.
		skipsBefore := readIMCSnapshotSkippedTotal(t)

		messages := []model.D{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Echo back the word: Red"},
		}

		d := model.D{
			"messages":   messages,
			"max_tokens": 256,
		}

		// Turn 1: builds the cache prefix from [system].
		ch1, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}
		_, content1, err := testlib.DrainChat(ctx, ch1)
		if err != nil {
			t.Fatalf("turn 1: %v", err)
		}
		if content1 == "" {
			t.Fatal("turn 1: expected non-empty content")
		}

		// Turn 2: identical message list — a pure hit (cacheable prefix
		// is messages[0:N-1] == [system] for both turns).
		ch2, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}
		_, content2, err := testlib.DrainChat(ctx, ch2)
		if err != nil {
			t.Fatalf("turn 2: %v", err)
		}
		if content2 == "" {
			t.Fatal("turn 2: expected non-empty content (recurrent state corrupted under skip?)")
		}

		skipsAfter := readIMCSnapshotSkippedTotal(t)
		if skipsAfter <= skipsBefore {
			t.Errorf("imc_snapshot_skipped_total did not increment: before=%v after=%v (pure-hit skip never fired)", skipsBefore, skipsAfter)
		}

		t.Logf("turn 1 content=%q", content1)
		t.Logf("turn 2 content=%q (pure hit, skip fired)", content2)
		t.Logf("imc_snapshot_skipped_total: before=%v after=%v", skipsBefore, skipsAfter)
	})
}
