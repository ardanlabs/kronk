// Package draft_test exercises the TRADITIONAL separate-GGUF draft model
// path for speculative decoding: a Qwen3-8B target paired with the
// vocab-matched Qwen3-0.6B draft (testlib.CfgClassicDraftChat). This is
// the classic drafter (*classicDrafter) — it owns its own llama_model and
// KV cache and uses the token-only draft/verify loop — as distinct from
// the MTP path covered by the mtp package.
//
// These are smoke tests: a successful Chat / ChatStreaming response with a
// non-zero accepted draft count implicitly verifies that the separate
// draft model loaded, generated valid candidate tokens, and that the
// target accepted them and emitted text without corruption.
package draft_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// sawAcceptedDraft is set true by checkDraftUsage when any request in the
// suite gets at least one draft token accepted. It is the suite-level proof
// that the classic separate-GGUF drafter actually loaded, drafted, and had
// drafts verified — a regression that silently dropped speculation would
// leave it false. It must be suite-level (not per-request) because the
// adaptive throttle legitimately disables drafting on individual requests;
// see checkDraftUsage.
var sawAcceptedDraft atomic.Bool

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgClassicDraftChat(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("DraftChat", func(t *testing.T) { testChat(t, krn, testlib.DChatNoTool) })
		t.Run("DraftStreamingChat", func(t *testing.T) { testChatStreaming(t, krn, testlib.DChatNoTool) })
	})

	// Registered after WithModel's unload cleanup, so by LIFO order this
	// runs first — after every (parallel) subtest has completed, before
	// the model unloads. At least one request must have produced an
	// accepted draft token for the classic drafter path to be considered
	// exercised.
	t.Cleanup(func() {
		if !sawAcceptedDraft.Load() {
			t.Error("classic drafter produced no accepted draft tokens across the suite; the separate-GGUF draft path may have regressed (drafter failed to load or every draft was rejected)")
		}
	})
}

// checkDraftUsage records whether the separate draft model drafted and got
// tokens accepted on this request. Unlike the MTP head (whose acceptance
// stays high), a small separate draft model can have acceptance near the
// adaptive-throttle floor: chooseNDraft returns 0 below an EMA of 0.30 and
// the EMA PERSISTS across requests on a slot, so it is correct and expected
// for later requests to draft nothing. A per-request DraftTokens==0 is
// therefore NOT a failure here — the suite-level sawAcceptedDraft check in
// TestSuite guards against the drafter never working at all.
func checkDraftUsage(t *testing.T, id string, usage *model.Usage) {
	t.Helper()

	if usage == nil {
		t.Errorf("%s: draft request returned no usage block", id)
		return
	}
	if usage.DraftTokens == 0 {
		t.Logf("%s: drafted 0 tokens (adaptive throttle disabled speculation for this request)", id)
		return
	}
	if usage.DraftAcceptedTokens > 0 {
		sawAcceptedDraft.Store(true)
	} else {
		t.Logf("%s: WARNING drafted %d tokens but accepted 0 (acceptance EMA may have collapsed)", id, usage.DraftTokens)
		return
	}
	reason := usage.DraftDisableReason
	if reason == "" {
		reason = "active"
	}
	t.Logf("%s: draft=%d accepted=%d rate=%.2f coverage=%.2f reason=%s",
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

		checkDraftUsage(t, id, resp.Usage)

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

		checkDraftUsage(t, id, lastResp.Usage)

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
