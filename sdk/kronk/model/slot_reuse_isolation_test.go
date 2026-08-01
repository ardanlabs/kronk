package model

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// Slot / sequence lifecycle and cross-request isolation.
//
// Every test in this file pins a field or an ordering that survives a slot
// release and re-acquire when llama.cpp's reference lifecycle says it must
// not. The reference is tools/server/server-context.cpp in .extras/llama.cpp
// (b10211):
//
//   - server_slot::reset()        server-context.cpp:333  — the checklist of
//                                per-request state that must be dropped.
//   - server_slot::release()      server-context.cpp:524  — computes the
//                                request's timings BEFORE calling reset().
//   - server_slot::prompt_clear() server-context.cpp:291  — the KV clear
//                                (mem.seq_rm(id,-1,-1) at :294) is ATOMIC with
//                                clearing the slot's token bookkeeping.
//   - update_slots()              server-context.cpp:3124 — re-zeroes
//                                slot.t_start_generation when a slot picks up
//                                a new prompt.
//
// Helper names here are suffixed to avoid collisions with the existing
// source-analysis and mirror tests in this package. kronkRepoRoot,
// parseKronkSource, findKronkFunc and srcPos are reused from
// mtp_ctxparams_source_test.go.

// =============================================================================

// TestSlotResetClearsPerRequestClockAndSpecSnapshot pins two fields that
// (*slot).reset() forgets, both of which llama.cpp's reference lifecycle
// explicitly clears.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_slot.go:300  — func (s *slot) reset(), the
//     complete list of fields dropped between requests. It never assigns
//     s.startTime or s.specSnapshot.
//   - sdk/kronk/model/batchgen_slot.go:293  — startTime declaration.
//   - sdk/kronk/model/batchgen_slot.go:263  — specSnapshot declaration.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/tools/server/server-context.cpp:3124
//     `slot.t_start_generation = 0;` — the generation clock is re-zeroed the
//     moment a slot picks up a NEW prompt, precisely so a slot that never
//     reaches generation cannot report the previous request's clock.
//   - .extras/llama.cpp/tools/server/server-context.cpp:349-351
//     inside reset(): `spec_draft.clear(); spec_i_batch.clear();
//     spec_ckpt.clear();` — the speculative checkpoint buffer (the direct
//     analogue of Kronk's specSnapshot, a saved per-sequence state used to
//     rewind a spec round) is dropped on every release.
//
// FAILURE SCENARIO — startTime
//
//	s.startTime is written exactly once per request, at
//	batchgen_tokens.go:97, and only inside the `if !s.prefillDone` branch —
//	i.e. only when the request produced its FIRST output token. reset() does
//	not clear it, so it survives into the next request on the same slot.
//
//	finishSlot then reads it unconditionally at batchgen_finish.go:149-150:
//
//	    if !s.startTime.IsZero() {
//	        elapsed = time.Since(s.startTime)
//	    }
//
//	Request A generates on slot 0 and sets startTime. Request B lands on
//	slot 0 and dies before its first token — context-window rejection
//	(batchgen_slot_start.go:841), an IMC stale-session error
//	(batchgen_slot_start.go:197), a client disconnect during prefill
//	(batchgen_engine.go:375), a decode failure (batchgen_engine.go:471) or
//	shutdown drain (batchgen_shutdown.go:27). B's error Usage and its
//	"slot-finished" / "request-lifecycle" log lines are then computed from
//	A's wall clock: `elapsed` spans both requests and TokensPerSecond
//	(batchgen_finish.go:207-208, :326-327) is divided by an unrelated
//	interval. The staleness is silent and unbounded — an idle slot makes it
//	arbitrarily large.
//
// FAILURE SCENARIO — specSnapshot
//
//	specSnapshot is a serialized copy of the TARGET context's per-sequence
//	state (llama.StateSeqGetData, batchgen_speculative.go:797) for the
//	slot's seqID, captured before a speculative batch on a hybrid target
//	(batchgen_engine.go:309). Its length is the "a snapshot exists" flag:
//	finalizeSpeculativeTokens gates the rollback path on
//	`len(s.specSnapshot) > 0` at batchgen_speculative.go:684 and, when set,
//	feeds those bytes straight back into the live context via
//	llama.StateSeqSetData (batchgen_speculative.go:823).
//
//	Because reset() leaves both the bytes and the length intact, a released
//	slot hands the next request a populated, non-empty snapshot of the
//	PREVIOUS conversation's sequence state. Today every capture site is
//	paired with a verify site inside the same round, so the stale buffer is
//	overwritten before it can be restored — but nothing in reset() or in the
//	restore path enforces that pairing, and the failure mode if it ever
//	breaks is a whole-sequence rollback to another conversation's KV. This
//	is the one field on the slot that literally holds another request's KV
//	bytes; llama.cpp clears its equivalent on every release
//	(server-context.cpp:351) rather than relying on the pairing.
func TestSlotResetClearsPerRequestClockAndSpecSnapshot(t *testing.T) {
	t.Run("startTime", func(t *testing.T) {
		// A slot that finished a request which reached generation.
		prev := time.Now().Add(-42 * time.Minute)
		s := slot{
			id:        0,
			seqID:     0,
			startTime: prev,
		}

		s.reset()

		if !s.startTime.IsZero() {
			t.Errorf("slot.reset() left startTime = %v (want zero time)\n"+
				"batchgen_slot.go:300 (reset) never clears the field declared at batchgen_slot.go:293.\n"+
				"finishSlot reads it unconditionally at batchgen_finish.go:149-150, so the next\n"+
				"request on this slot reports elapsed/TokensPerSecond measured from the PREVIOUS\n"+
				"request's first token. llama.cpp re-zeroes the same clock when a slot picks up a\n"+
				"new prompt: server-context.cpp:3124 `slot.t_start_generation = 0;`.",
				s.startTime)
		}
	})

	t.Run("specSnapshot", func(t *testing.T) {
		// A slot that finished a request on a hybrid target, leaving a
		// captured per-sequence state behind. The bytes stand in for a
		// real llama.StateSeqGetData payload.
		s := slot{
			id:           0,
			seqID:        0,
			specSnapshot: []byte{0xDE, 0xAD, 0xBE, 0xEF},
		}

		s.reset()

		if len(s.specSnapshot) != 0 {
			t.Errorf("slot.reset() left len(specSnapshot) = %d (want 0)\n"+
				"batchgen_slot.go:300 (reset) never truncates the field declared at\n"+
				"batchgen_slot.go:263. len(specSnapshot) is the \"a snapshot exists\" flag read at\n"+
				"batchgen_speculative.go:684, and the bytes are pushed back into the live context\n"+
				"by llama.StateSeqSetData at batchgen_speculative.go:823 — so a released slot hands\n"+
				"the next request a restorable snapshot of the previous conversation's sequence\n"+
				"state. llama.cpp clears the equivalent speculative checkpoint on every release:\n"+
				"server-context.cpp:351 `spec_ckpt.clear();` inside reset().",
				len(s.specSnapshot))
		}
	})
}

// =============================================================================

// assembleDraftPromptMirror is a VERBATIM MIRROR of the draft-prompt assembly
// in (*batchEngine).startSlotText:
//
//	sdk/kronk/model/batchgen_slot_start.go:902-935
//
//	  draft := e.model.draft.core()
//	  ...
//	  case job.imcCacheHit && job.imcSession != nil:
//	      cachedLen = len(cached)
//	      needed = cachedLen + len(tokens)
//	      if cap(draft.promptBuf) >= needed {
//	          draft.promptBuf = draft.promptBuf[:needed]
//	      } else {
//	          draft.promptBuf = make([]llama.Token, needed)
//	      }
//	      copy(draft.promptBuf, cached)
//	      copy(draft.promptBuf[cachedLen:], tokens)
//	  default:
//	      needed = len(tokens)
//	      if cap(draft.promptBuf) >= needed {
//	          draft.promptBuf = draft.promptBuf[:needed]
//	      } else {
//	          draft.promptBuf = make([]llama.Token, needed)
//	      }
//	      copy(draft.promptBuf, tokens)
//	  }
//	  s.draftPromptTokens = draft.promptBuf
//
// startSlotText itself needs a loaded model (llama.Tokenize, otel spans, a
// live sampler), so this mirror isolates the buffer handoff. It takes the REAL
// draftCore type — the shared struct returned by drafter.core() — so the
// aliasing being pinned is the production one, not a stand-in.
//
// MAINTAINER: when batchgen_slot_start.go:902-935 is fixed to copy into a
// per-slot buffer, update this mirror in the same commit so it keeps tracking
// the production code.
func assembleDraftPromptMirror(draft *draftCore, cached, tokens []llama.Token) []llama.Token {
	var needed int
	var cachedLen int

	switch {
	case cached != nil:
		cachedLen = len(cached)
		needed = cachedLen + len(tokens)

		if cap(draft.promptBuf) >= needed {
			draft.promptBuf = draft.promptBuf[:needed]
		} else {
			draft.promptBuf = make([]llama.Token, needed)
		}
		copy(draft.promptBuf, cached)
		copy(draft.promptBuf[cachedLen:], tokens)

	default:
		needed = len(tokens)

		if cap(draft.promptBuf) >= needed {
			draft.promptBuf = draft.promptBuf[:needed]
		} else {
			draft.promptBuf = make([]llama.Token, needed)
		}
		copy(draft.promptBuf, tokens)
	}

	return draft.promptBuf
}

// TestDraftPromptTokensAreSlotOwned pins a cross-request aliasing bug in the
// slot lifecycle: two concurrently-starting slots are handed the SAME backing
// array for their draft prompts.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_slot_start.go:935
//     `s.draftPromptTokens = draft.promptBuf`
//   - sdk/kronk/model/batchgen_slot_start.go:916-932 — the in-place refill of
//     draft.promptBuf that precedes it.
//   - sdk/kronk/model/model.go:137 — `promptBuf []llama.Token // Reusable
//     buffer for assembling draft prompt tokens`, a single field on draftCore.
//     drafter.core() returns the SAME draftCore to every slot
//     (sdk/kronk/model/model.go:106-128), so promptBuf is engine-global, not
//     per-slot.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/tools/server/server-context.cpp:333 (reset) and
//     server-context.cpp:3116 (`const auto & input_tokens = slot.task->tokens;`)
//     — every slot's prompt tokens live in that slot's own task
//     (server_slot::task, reset at server-context.cpp:364 `task.reset()`).
//     There is no server-wide scratch buffer that slots point into, so a
//     second slot launching cannot alter a first slot's prompt.
//
// FAILURE SCENARIO
//
//	Separate-GGUF speculative decoding (classicDrafter; MTP skips this path,
//	batchgen_slot_start.go:865-869). fillSlots assigns every pending job it
//	can in ONE call — batchgen_schedule.go:43-63 loops over pendingJobs and
//	gives each the first idle slot — so two startSlotText calls can run
//	back to back inside a single processBatch iteration.
//
//	  1. Job A (long prompt) starts on slot 0. draft.promptBuf is grown to
//	     A's length, filled with A's tokens, and slot0.draftPromptTokens is
//	     pointed at it. draftPrefillNeeded = true
//	     (batchgen_slot_start.go:941).
//	  2. Job B starts on slot 1. cap(draft.promptBuf) is already >= B's
//	     length, so line 917 reslices the SAME array and line 932 copies B's
//	     tokens over A's. slot0.draftPromptTokens now reads B's tokens in
//	     its leading positions and A's stale tail after them.
//	  3. The draft prefill is deferred to the NEXT processBatch iteration
//	     and gated on s.prefillDone (batchgen_engine.go:218-228), so a slot
//	     with a multi-chunk prompt loses the race by construction.
//	     prefillDraft (batchgen_speculative.go:17) then decodes that mixed
//	     token sequence into slot 0's DRAFT KV sequence, and persists it as
//	     s.draftCachedTokens (batchgen_speculative.go:103-107) — a field
//	     reset() deliberately keeps across requests
//	     (batchgen_slot.go:343).
//
//	Slot 0's drafter is now conditioned on another conversation's prompt.
//	Every subsequent round drafts from it, acceptance collapses, and
//	s.specAccEMA — which also persists across requests on the slot
//	(batchgen_slot.go:163) — latches speculation off for the slot's whole
//	lifetime except for the periodic probe (chooseNDraft,
//	batchgen_speculative.go:147-152). The next request's prefillDraft
//	compares its prompt against the poisoned draftCachedTokens, so the
//	wrong-conversation prefix keeps being "reused" until a divergence is
//	found.
//
//	The fix is a per-slot buffer, exactly as was already done for
//	draftTokensBuf / draftDistBuf / draftCandDistBuf
//	(batchgen_slot.go:167-171: "Per-slot owned buffers for speculative
//	decoding. Avoids shared buffer corruption when multiple slots generate
//	draft tokens in the same processBatch iteration."). draftPromptTokens
//	was missed.
func TestDraftPromptTokensAreSlotOwned(t *testing.T) {
	t.Run("mirror-two-slots-share-one-array", func(t *testing.T) {
		// One shared draftCore, as returned by drafter.core() for every slot.
		draft := &draftCore{}

		tokensA := []llama.Token{10, 11, 12, 13, 14, 15, 16, 17}
		tokensB := []llama.Token{90, 91, 92, 93}

		wantA := append([]llama.Token(nil), tokensA...)

		// Job A starts on slot 0.
		slotA := slot{id: 0, seqID: 0}
		slotA.draftPromptTokens = assembleDraftPromptMirror(draft, nil, tokensA)

		// Job B starts on slot 1 in the same fillSlots pass. B's prompt is
		// shorter, so cap(draft.promptBuf) already suffices and the array is
		// refilled in place.
		slotB := slot{id: 1, seqID: 1}
		slotB.draftPromptTokens = assembleDraftPromptMirror(draft, nil, tokensB)

		for i := range wantA {
			if i >= len(slotA.draftPromptTokens) || slotA.draftPromptTokens[i] != wantA[i] {
				t.Fatalf("slot 0 draftPromptTokens = %v, want %v\n"+
					"batchgen_slot_start.go:935 aliases the engine-global draft.promptBuf\n"+
					"(model.go:137) onto every slot, and batchgen_slot_start.go:916-932 refills it\n"+
					"in place for the next slot that starts. prefillDraft\n"+
					"(batchgen_speculative.go:17) decodes this slice into slot 0's draft KV and\n"+
					"persists it as s.draftCachedTokens, so slot 0's drafter runs on another\n"+
					"conversation's prompt. llama.cpp keeps each slot's prompt in its own\n"+
					"server_slot::task (server-context.cpp:3116, reset at server-context.cpp:364).",
					slotA.draftPromptTokens, wantA)
			}
		}

		if len(slotB.draftPromptTokens) != len(tokensB) {
			t.Fatalf("mirror is out of sync with production: slot 1 got %d tokens, want %d",
				len(slotB.draftPromptTokens), len(tokensB))
		}
	})

	t.Run("source-no-shared-buffer-aliased-onto-slot", func(t *testing.T) {
		root := kronkRepoRoot(t)
		path := filepath.Join(root, "sdk", "kronk", "model", "batchgen_slot_start.go")

		fset := token.NewFileSet()
		f := parseKronkSource(t, fset, path)
		fn := findKronkFunc(t, f, path, "startSlotText")

		var hits []string
		ast.Inspect(fn, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range assign.Lhs {
				if !isSelectorPathIsolation(lhs, "s", "draftPromptTokens") {
					continue
				}
				if i >= len(assign.Rhs) {
					continue
				}
				if isSelectorPathIsolation(assign.Rhs[i], "draft", "promptBuf") {
					hits = append(hits, srcPos(fset, root, assign.Pos()))
				}
			}
			return true
		})

		if len(hits) > 0 {
			t.Errorf("startSlotText aliases the shared draftCore.promptBuf onto the slot at %v\n"+
				"draftCore is engine-global (drafter.core() returns the same struct to every\n"+
				"slot, model.go:106-128) and promptBuf is refilled in place by the next slot to\n"+
				"start (batchgen_slot_start.go:916-932). The slot must own its draft prompt\n"+
				"buffer, as draftTokensBuf / draftDistBuf / draftCandDistBuf already do\n"+
				"(batchgen_slot.go:167-171). llama.cpp keeps prompt tokens in the per-slot\n"+
				"server_slot::task (server-context.cpp:3116).", hits)
		}
	})
}

// isSelectorPathIsolation reports whether expr is exactly `recv.field`.
func isSelectorPathIsolation(expr ast.Expr, recv, field string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != field {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)

	return ok && ident.Name == recv
}

// =============================================================================

// TestPrefillStagingLoopDoesNotReleaseSlots pins the ordering hazard that lets
// a released slot's tokens be decoded back into the sequence finishSlot just
// cleared.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_engine.go:366-396 — the text-prefill staging
//     loop. Rows are pushed into the SHARED e.batch by addPrefillChunk
//     (batchgen_prefill_text.go:60-64: `e.batch.Add(tok, s.nPast, s.seqIDs,
//     isLast)`), and the loop makes MULTIPLE passes over e.slots.
//   - sdk/kronk/model/batchgen_engine.go:375 — `e.finishSlot(s,
//     s.job.ctx.Err())` inside that loop.
//   - sdk/kronk/model/batchgen_engine.go:381 — `e.finishSlot(s,
//     e.slotCancelError(s))` inside that loop.
//   - sdk/kronk/model/batchgen_finish.go:179 — finishSlot's
//     `llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)`.
//   - sdk/kronk/model/batchgen_engine.go:458 — the single
//     `llama.Decode(e.model.lctx, e.batch)` for the whole iteration.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/tools/server/server-context.cpp:291-296
//     `prompt_clear()` — `mem.seq_rm(id, -1, -1)` is executed together with
//     `prompt.clear()`. The KV wipe and the slot's record of what is in that
//     KV are dropped atomically; nothing may be in flight against the
//     sequence across the wipe.
//   - .extras/llama.cpp/tools/server/server-context.cpp:1449-1450 —
//     update_slots() runs as its own queue callback. Cancellations are
//     applied in process_single_task BEFORE update_slots builds the batch, so
//     a slot is never released between `batch.add(...)` (e.g.
//     server-context.cpp:1339-1342) and the decode that consumes the batch.
//   - .extras/llama.cpp/tools/server/server-context.cpp:3418-3423 — and even
//     if stray cells existed, every prompt launch re-trims
//     `slot.mem.seq_rm(slot.id, p0, -1)` past the validated common prefix.
//     Kronk has no such re-trim: startSlot ASSUMES the sequence is empty.
//
// FAILURE SCENARIO
//
//	The staging loop at batchgen_engine.go:366 iterates until no slot adds
//	tokens. A slot with a prompt longer than NUBatch is visited on pass 1
//	(rows staged into e.batch, s.nPast advanced) and again on pass 2. If the
//	client disconnects between the two passes, pass 2 takes the
//	batchgen_engine.go:374 branch (or addPrefillChunk returns false on its
//	own ctx.Done() check, batchgen_prefill_text.go:24) and finishSlot runs.
//
//	finishSlot clears the sequence (batchgen_finish.go:179) and resets the
//	slot — but it cannot un-stage the rows already sitting in e.batch, and
//	e.batch is only cleared at the TOP of the next iteration
//	(batchgen_engine.go:201). The decode at batchgen_engine.go:458 therefore
//	writes the dead request's prompt tokens back into the sequence that was
//	just wiped.
//
//	fillSlots runs at batchgen_engine.go:418 — after the staging loop and
//	still BEFORE that decode — and hands the now-idle slot the next pending
//	job (batchgen_schedule.go:51-58). startSlot clears the sequence again
//	(batchgen_slot_start.go:494) or restores an IMC snapshot over it
//	(batchgen_slot_start.go:221; llama_state_seq_set_data itself seq_rm's the
//	destination, .extras/llama.cpp/src/llama-kv-cache.cpp:2407) and stages
//	the NEW prompt from position 0. Both sets of rows then go through the one
//	llama.Decode with the same seq id, so the new conversation's sequence
//	ends up holding the cancelled request's tokens at positions >= its own
//	nPast. Those cells are never overwritten — llama.cpp's KV appends by
//	slot rather than by (seq, pos), as batchgen_speculative.go:947-952 already
//	documents — so as generation walks past them the causal mask admits them
//	and the model attends to the other conversation's text. That is the
//	reported "the model's context changed mid-conversation" symptom, and
//	nothing downstream can detect it because Kronk keeps no per-sequence
//	token list to validate the KV against.
//
//	Either fix satisfies this test: hoist the cancellation check so a slot
//	is released before it stages anything for the iteration (llama.cpp's
//	arrangement), or make finishSlot/reset neutralize the slot's already
//	staged rows.
func TestPrefillStagingLoopDoesNotReleaseSlots(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "batchgen_engine.go")

	fset := token.NewFileSet()
	f := parseKronkSource(t, fset, path)
	fn := findKronkFunc(t, f, path, "processBatch")

	// Locate the staging loop: the for-statement whose body calls
	// e.addPrefillChunk. Located structurally, not by line number.
	var loop *ast.ForStmt
	ast.Inspect(fn, func(n ast.Node) bool {
		forStmt, ok := n.(*ast.ForStmt)
		if !ok || loop != nil {
			return true
		}
		if callsEngineMethodIsolation(forStmt, "addPrefillChunk") {
			loop = forStmt
		}
		return true
	})

	if loop == nil {
		t.Fatal("no for-statement calling e.addPrefillChunk found in processBatch: " +
			"the prefill staging loop this test pins no longer exists; re-point it")
	}

	// Sanity check that the loop really does stage into the shared batch, so
	// a refactor that moves the staging elsewhere fails loudly instead of
	// silently passing.
	if !callsEngineMethodIsolation(loop, "addPrefillChunk") {
		t.Fatal("staging loop no longer calls addPrefillChunk")
	}

	var releases []string
	ast.Inspect(loop, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isSelectorPathIsolation(call.Fun, "e", "finishSlot") {
			releases = append(releases, srcPos(fset, root, call.Pos()))
		}
		return true
	})

	if len(releases) > 0 {
		t.Errorf("processBatch releases slots from inside the prefill staging loop at %v\n"+
			"The loop stages rows into the shared e.batch via addPrefillChunk\n"+
			"(batchgen_prefill_text.go:60-64) and makes multiple passes over e.slots. A\n"+
			"finishSlot on pass N+1 wipes the slot's KV sequence (batchgen_finish.go:179) but\n"+
			"cannot un-stage the rows added on pass N; e.batch is only cleared at the top of\n"+
			"the NEXT iteration (batchgen_engine.go:201). The single decode at\n"+
			"batchgen_engine.go:458 therefore writes the dead request's tokens back into the\n"+
			"sequence that was just cleared — and fillSlots (batchgen_engine.go:418) may\n"+
			"already have started the next conversation on that same slot and seq id before\n"+
			"the decode runs.\n"+
			"llama.cpp never releases a slot between staging and decode: cancellations are\n"+
			"applied before update_slots builds the batch (server-context.cpp:1449-1450), and\n"+
			"prompt_clear() drops the KV and the slot's token bookkeeping atomically\n"+
			"(server-context.cpp:291-296).", releases)
	}
}

// callsEngineMethodIsolation reports whether n contains a call to e.<name>.
func callsEngineMethodIsolation(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || found {
			return true
		}
		if isSelectorPathIsolation(call.Fun, "e", name) {
			found = true
		}
		return true
	})

	return found
}
