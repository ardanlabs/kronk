package qwen3_test

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

// Test_CacheIMCQwen3PureHitSnapshotSkip exercises the Part A optimization
// on a Dense attention model. Two requests with an identical cacheable
// prefix produce a pure hit on the second; the post-restore snapshot is
// skipped. Verifies the model still produces a coherent reply (the
// optimization did not corrupt the restored KV state).
func Test_CacheIMCQwen3PureHitSnapshotSkip(t *testing.T) {
	cfg := model.Config{
		ModelFiles:          testlib.MPThinkToolChat.ModelFiles,
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

// Test_CacheIMCQwen3ExtensionDoesNotSkip is a regression guard for the
// extension path. A normal growing-history turn must NOT take the skip
// branch — the snapshot is required to externalize the newly-extended
// prefix for the next request.
func Test_CacheIMCQwen3ExtensionDoesNotSkip(t *testing.T) {
	cfg := model.Config{
		ModelFiles:          testlib.MPThinkToolChat.ModelFiles,
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

		systemPrompt := "You are a helpful assistant."

		// Turn 1: builds the cache prefix from [system].
		d1 := model.D{
			"messages": []model.D{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": "Echo back the word: Red"},
			},
			"max_tokens": 64,
		}
		ch1, err := krn.ChatStreaming(ctx, d1)
		if err != nil {
			t.Fatalf("turn 1: chat streaming: %v", err)
		}
		_, content1, err := testlib.DrainChat(ctx, ch1)
		if err != nil || content1 == "" {
			t.Fatalf("turn 1: err=%v content empty=%v", err, content1 == "")
		}

		skipsBeforeTurn2 := readIMCSnapshotSkippedTotal(t)

		// Turn 2 grows the cacheable prefix to [system, user, assistant].
		// This must extend, not pure-hit.
		d2 := model.D{
			"messages": []model.D{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": "Echo back the word: Red"},
				{"role": "assistant", "content": content1},
				{"role": "user", "content": "Echo back the word: Blue"},
			},
			"max_tokens": 64,
		}
		ch2, err := krn.ChatStreaming(ctx, d2)
		if err != nil {
			t.Fatalf("turn 2: chat streaming: %v", err)
		}
		_, content2, err := testlib.DrainChat(ctx, ch2)
		if err != nil || content2 == "" {
			t.Fatalf("turn 2: err=%v content empty=%v", err, content2 == "")
		}

		skipsAfterTurn2 := readIMCSnapshotSkippedTotal(t)
		if skipsAfterTurn2 != skipsBeforeTurn2 {
			t.Errorf("imc_snapshot_skipped_total incremented on an extension turn: before=%v after=%v (extensions must always snapshot)", skipsBeforeTurn2, skipsAfterTurn2)
		}
	})
}
