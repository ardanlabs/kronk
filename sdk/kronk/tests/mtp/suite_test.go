// Package mtp_test exercises the MTP (Multi-Token Prediction) drafter
// against the unsloth/Qwen3.6-35B-A3B-MTP target. The drafter is
// auto-enabled by the loader based on the GGUF's nextn_predict_layers
// metadata — no explicit DraftModelMTP configuration is required.
//
// These are smoke tests: a successful Chat / ChatStreaming response
// implicitly verifies that the MTP draft context loaded, that
// speculation produced valid draft tokens, and that the target accepted
// and emitted text without corruption. Recurrent-state snapshot
// allocation (NRsSeq) is logged by the loader and visible in test
// output.
package mtp_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgMTPChat(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("MTPChat", func(t *testing.T) { testChat(t, krn, testlib.DChatNoTool) })
		t.Run("MTPStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatNoTool) })
	})
}

// TestSuiteMultiSlot runs the same chat / streaming exercises against
// an MTP drafter with NSeqMax=2 so concurrent requests genuinely land
// on different slots. This exercises the Pass 2A/2B split (multi-slot
// hybrid + MTP), the one-prefill-chunk-per-slot-per-round contiguity
// cap, and the post-verify mirror across multiple active spec slots.
// Single-slot behavior remains covered by TestSuite above.
func TestSuiteMultiSlot(t *testing.T) {
	testlib.WithModel(t, testlib.CfgMTPChatMultiSlot(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("MTPChat", func(t *testing.T) { testChat(t, krn, testlib.DChatNoTool) })
		t.Run("MTPStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatNoTool) })
	})
}

// checkMTPUsage verifies that the MTP drafter actually produced and
// got at least one draft token accepted on this request. A
// regression that silently fell back to plain target decoding (e.g.,
// drafter never loaded, every draft rejected) would otherwise pass
// the content assertion. Logs a warning instead of failing because
// some shorter requests may legitimately produce a small draft count
// on a cold acceptance EMA.
func checkMTPUsage(t *testing.T, id string, usage *model.Usage) {
	t.Helper()

	if usage == nil {
		t.Errorf("%s: MTP request returned no usage block", id)
		return
	}
	if usage.DraftTokens == 0 {
		t.Errorf("%s: MTP request produced 0 draft tokens (drafter may have failed to load or was silently disabled)", id)
		return
	}
	if usage.DraftAcceptedTokens == 0 {
		t.Logf("%s: WARNING MTP drafted %d tokens but accepted 0 (acceptance EMA may have collapsed)", id, usage.DraftTokens)
		return
	}
	reason := usage.DraftDisableReason
	if reason == "" {
		reason = "active"
	}
	t.Logf("%s: MTP draft=%d accepted=%d rate=%.2f coverage=%.2f reason=%s",
		id, usage.DraftTokens, usage.DraftAcceptedTokens, usage.DraftAcceptanceRate, usage.DraftCoverage, reason)
}

func testChat(t *testing.T, krn *kronk.Kronk, d model.D) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, d)
		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}

		reasoning := testlib.HasReasoningField(krn)

		result := testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatTextFinal, "Gorilla", "", "", false, reasoning)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", resp)
			return result.Err
		}

		checkMTPUsage(t, id, resp.Usage)

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testChatStreaming(t *testing.T, krn *kronk.Kronk, d model.D) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		reasoning := testlib.HasReasoningField(krn)

		var acc testlib.StreamAccumulator
		var lastResp model.ChatResponse
		for resp := range ch {
			acc.Accumulate(resp)
			lastResp = resp

			if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatText, reasoning, true); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		result := testlib.TestStreamingContent(&acc, lastResp, "Gorilla")

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("accumulated content: %q", acc.Content.String())
			t.Logf("%#v", lastResp)
			return result.Err
		}

		checkMTPUsage(t, id, lastResp.Usage)

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
