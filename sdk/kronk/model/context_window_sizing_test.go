package model

import (
	"fmt"
	"testing"
)

// =============================================================================
// Context-window sizing / per-sequence division / prompt-limit regressions.
//
// Reference build: .extras/llama.cpp (llama.cpp b10211). Reference model:
// unsloth/Qwen3.6-35B-A3B-MTP-GGUF (arch qwen35moe, block_count 41,
// context_length 262144, nextn_predict_layers 1, full_attention_interval 4,
// ssm.* present -> hybrid attention+recurrent, NO attention.sliding_window key
// so hparams.n_swa == 0 for this target).
//
// RULED OUT while auditing this area (do not re-investigate):
//
//   - n_ctx vs n_ctx_seq. llama.cpp derives the per-sequence window as
//     n_ctx_seq = kv_unified ? n_ctx : GGML_PAD(n_ctx / n_seq_max, 256)
//     (.extras/llama.cpp/src/llama-context.cpp:286-298) and the server hands
//     that to every slot (tools/server/server-context.cpp:1294, :1349).
//     Kronk's ContextWindow is a PER-SEQUENCE quantity and modelCtxParams
//     scales it up before handing it to libllama:
//     sdk/kronk/model/config.go:862  ctxParams.NCtx = ContextWindow * nSeqMax
//     sdk/kronk/model/config.go:937-939  KVUnified = 1 when nSeqMax > 1
//     so n_ctx_seq == ContextWindow*nSeqMax (unified) or == pad(ContextWindow)
//     (nSeqMax == 1). Either way each slot's gate of ContextWindow
//     (batchgen_slot_start.go:839) is <= the capacity libllama allocated, and
//     sdk/kronk/gguf/kvcache.go:74-81 sizes the VRAM estimate with the same
//     ContextWindow*Slots convention. No halving bug.
//   - Silent prompt truncation. The only truncation in the repo is the opt-in
//     embeddings/rerank path (embed.go:138-150, rerank.go:160). The chat path
//     errors the request (batchgen_slot_start.go:839-843, :1012-1016,
//     :1112-1116), which is what llama.cpp does with ctx_shift disabled.
//   - Context shift. Kronk implements none, and llama.cpp defaults
//     common_params.ctx_shift = false (.extras/llama.cpp/common/common.h:569)
//     and force-disables it for memories that cannot shift
//     (tools/server/server-context.cpp:1268-1272), which covers this hybrid
//     target. Nothing to diverge from.

// =============================================================================

// ctxSizingLlamaCppDraftMax is a VERBATIM MIRROR of llama.cpp's
// server_slot::get_n_draft_max:
//
//	.extras/llama.cpp/tools/server/server-context.cpp:473
//		int n_draft_max = n_ctx - prompt.n_tokens() - 2;
//	.extras/llama.cpp/tools/server/server-context.cpp:475-477
//		if (n_remaining > 0) {
//		    n_draft_max = std::min(n_draft_max, n_remaining - 1);
//		}
//	.extras/llama.cpp/tools/server/server-context.cpp:2986-2988
//		const int n_draft_max = slot.get_n_draft_max();
//		if (n_draft_max > 0) { ... generate drafts ... }
//
// nPast is llama.cpp's slot.prompt.n_tokens(): the comment at
// server-context.cpp:471-473 notes it is NOT yet expanded with the token
// sampled this round, which is exactly Kronk's s.nPast at the point
// batchgen_engine.go:284 pushes the spec range. nRemaining is
// server-context.cpp:436-440 (n_predict - n_decoded); Kronk's equivalent is
// job.params.MaxTokens - (s.reasonTokens + s.completionTokens).
func ctxSizingLlamaCppDraftMax(nCtx int, nPast int, nRemaining int) int {
	n := nCtx - nPast - 2

	if nRemaining > 0 {
		n = min(n, nRemaining-1)
	}

	return max(n, 0)
}

// TestChooseNDraftClampsToRemainingContext pins a missing guard: Kronk has no
// equivalent of llama.cpp's get_n_draft_max, so the speculative/MTP draft width
// is chosen from the acceptance EMA alone and is never clamped by the slot's
// remaining context or its remaining token budget.
//
// FINDING
//
//	chooseNDraft (sdk/kronk/model/batchgen_speculative.go:147) branches only on
//	s.specAccEMA and returns up to maxDraft. It cannot see s.nPast, the
//	configured ContextWindow, or job.params.MaxTokens, and neither can its three
//	call sites:
//	  sdk/kronk/model/batchgen_speculative.go:181 (separate-GGUF drafter)
//	  sdk/kronk/model/batchgen_mtp.go:258         (own-KV embedded MTP)
//	  sdk/kronk/model/batchgen_mtp.go:671         (shared-KV MTP assistant)
//	The returned width is then written straight into the target batch as
//	positions s.nPast .. s.nPast+nDraft:
//	  sdk/kronk/model/batchgen_engine.go:284-287
//	so a speculative round needs nDraft MORE KV cells than the plain
//	single-token round it replaces.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-context.cpp:473
//	    int n_draft_max = n_ctx - prompt.n_tokens() - 2;
//	.extras/llama.cpp/tools/server/server-context.cpp:476
//	    n_draft_max = std::min(n_draft_max, n_remaining - 1);
//	.extras/llama.cpp/tools/server/server-context.cpp:2988
//	    if (n_draft_max > 0) { ... }
//	Upstream reserves two cells of the slot's window (n_ctx here is
//	slot.n_ctx == llama_n_ctx_seq, server-context.cpp:1294/1349) and refuses to
//	draft at all when the reserve is gone.
//
// CONCRETE FAILURE SCENARIO
//
//	ContextWindow 8192, nseq-max 2 (KVUnified, pool = 16384 cells,
//	config.go:862/937), MTP nDraft 2 (draft_mtp.go:22). Slot A's conversation
//	reaches s.nPast == 8190 with acceptance high, so chooseNDraft returns 2 and
//	batchgen_engine.go:284-287 asks libllama for positions 8190..8192 — one cell
//	past what the slot is allowed and, with both slots near their gate, past the
//	shared unified pool. llama_decode returns 1 and batchgen_engine.go:464-475
//	fails EVERY active slot, so the co-resident conversation that was nowhere
//	near its own limit dies too. llama.cpp would have returned n_draft_max <= 0
//	and quietly decoded a single token instead. The same missing clamp makes
//	Kronk draft a full round when only one token of MaxTokens budget is left:
//	handleSpeculativeToken -> handleSampledToken finishes the slot mid-verify
//	(batchgen_tokens.go:207-210), so Phase B (finalizeSpeculativeTokens) never
//	runs and the round's draft+verify decode work is pure waste.
//
// The fix is to thread the slot's remaining context and remaining budget into
// chooseNDraft and clamp exactly as get_n_draft_max does.
func TestChooseNDraftClampsToRemainingContext(t *testing.T) {
	const contextWindow = 8192

	// maxDraft as the MTP path supplies it: mtpNDraft(cfg) with no override,
	// i.e. defMTPNDraft (sdk/kronk/model/draft_mtp.go:22).
	maxDraft := defMTPNDraft

	tests := []struct {
		name string
		// nPast is the slot's KV position before this round's sampled token,
		// i.e. what batchgen_engine.go:284 passes to e.batch.Add.
		nPast int
		// nRemaining is MaxTokens - (reasonTokens + completionTokens).
		nRemaining int
	}{
		{
			name:       "mid conversation, plenty of room",
			nPast:      4000,
			nRemaining: 4000,
		},
		{
			name:       "exactly at llama.cpp's two-cell reserve",
			nPast:      contextWindow - 2,
			nRemaining: 1000,
		},
		{
			name:       "one cell left in the window",
			nPast:      contextWindow - 1,
			nRemaining: 1000,
		},
		{
			name:       "last token of the max-tokens budget",
			nPast:      100,
			nRemaining: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A slot whose acceptance is healthy, so the adaptive throttle
			// does not mask the missing clamp. specAccEMA starts at 1.0 for
			// every slot (newBatchEngine, batchgen_engine.go:55).
			s := &slot{specAccEMA: 1.0}

			got := chooseNDraft(s, maxDraft)

			want := ctxSizingLlamaCppDraftMax(contextWindow, tt.nPast, tt.nRemaining)

			if got > want {
				// Cells the spec range writes past the slot's last legal
				// position (contextWindow-1). Negative means the round fits in
				// the window and it is the max-tokens budget that is violated.
				overrun := tt.nPast + got - (contextWindow - 1)

				effect := "batchgen_engine.go:284-287 pushes positions " +
					fmt.Sprintf("%d..%d", tt.nPast, tt.nPast+got) +
					" into the target batch — " + fmt.Sprintf("%d", overrun) +
					" cell(s) past the slot's last legal position " +
					fmt.Sprintf("%d", contextWindow-1) +
					". llama_decode returns 1 and batchgen_engine.go:464-475 fails every active " +
					"slot, including co-resident conversations that were nowhere near their own limit."
				if overrun <= 0 {
					effect = "the round fits the window but overshoots the remaining max-tokens " +
						"budget: handleSpeculativeToken -> handleSampledToken finishes the slot " +
						"mid-verify (batchgen_tokens.go:207-210), so Phase B " +
						"(finalizeSpeculativeTokens) never runs and the whole draft+verify decode " +
						"is wasted. llama.cpp's min(n_draft_max, n_remaining-1) prevents it."
				}

				t.Errorf("chooseNDraft = %d draft token(s), but only %d fit\n"+
					"  slot state:    n_past = %d, context window = %d, remaining budget = %d\n"+
					"  llama.cpp:     n_draft_max = n_ctx - prompt.n_tokens() - 2 = %d - %d - 2, "+
					"then min(.., n_remaining-1) -> %d\n"+
					"                 (.extras/llama.cpp/tools/server/server-context.cpp:473,476; "+
					"gated at :2988)\n"+
					"  kronk:         chooseNDraft (batchgen_speculative.go:147) branches only on "+
					"s.specAccEMA and never sees n_past, ContextWindow or MaxTokens.\n"+
					"  effect:        %s\n"+
					"  fix:           thread the slot's remaining context and remaining token budget "+
					"into chooseNDraft and clamp as get_n_draft_max does.",
					got, want,
					tt.nPast, contextWindow, tt.nRemaining,
					contextWindow, tt.nPast, want,
					effect)
			}
		})
	}
}

// =============================================================================

// ctxSizingPromptRejected is a VERBATIM MIRROR of Kronk's one and only
// prompt-length gate, which appears three times with identical arithmetic:
//
//	sdk/kronk/model/batchgen_slot_start.go:839  (startSlotText)
//		if s.nPrompt > e.model.cfg.ContextWindow() {
//	sdk/kronk/model/batchgen_slot_start.go:1012 (startSlotTextMRoPE)
//	sdk/kronk/model/batchgen_slot_start.go:1112 (startSlotMedia)
//
// The gate lives inside startSlot* which needs a live llama.Context and a
// loaded GGUF, so this mirror isolates the comparison.
//
// MAINTAINER: when those three lines are fixed, update this mirror in the same
// commit so it keeps tracking the production predicate.
func ctxSizingPromptRejected(nPrompt int, contextWindow int) bool {
	return nPrompt > contextWindow
}

// TestPromptAtExactContextWindowIsRejected pins an off-by-one in the prompt
// admission gate: a prompt of EXACTLY ContextWindow tokens is admitted, leaving
// zero cells for the very first generated token.
//
// FINDING
//
//	sdk/kronk/model/batchgen_slot_start.go:839 (and :1012, :1112) rejects only
//	when s.nPrompt > ContextWindow. A prompt of exactly ContextWindow tokens
//	occupies KV positions 0..ContextWindow-1, i.e. the whole per-sequence
//	window, and then the first generation round writes position ContextWindow
//	(batchgen_engine.go:333-334 / :284).
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-context.cpp:3188-3194
//		if (slot.task->n_tokens() >= slot.n_ctx) {
//		    send_error(slot, "request (%d tokens) exceeds the available context
//		                      size (%d tokens), try increasing it",
//		               ERROR_TYPE_EXCEED_CONTEXT_SIZE);
//		    slot.release();
//		    return;
//		}
//	Upstream uses >=, not >, precisely because a slot needs at least one free
//	cell to decode anything. slot.n_ctx is the per-sequence window
//	(llama_n_ctx_seq, server-context.cpp:1294/1349) — the same quantity Kronk
//	compares against.
//
// CONCRETE FAILURE SCENARIO
//
//	ContextWindow 8192 and a rendered conversation that tokenizes to exactly
//	8192 tokens. Kronk admits the slot, prefills all 8192 cells, then the first
//	decode of the generation phase asks for position 8192. libllama's find_slot
//	has no free cell for the sequence, llama_decode returns 1, and
//	batchgen_engine.go:464-475 fails EVERY active slot with the generic
//	"the context window is full" message (batchgen_errors.go:81) — after the
//	caller has already paid for a full 8192-token prefill, and taking any
//	co-resident conversation down with it. llama.cpp rejects the request up
//	front with an actionable EXCEED_CONTEXT_SIZE error and never touches the
//	other slots.
//
// The fix is to use >= at all three sites (or reserve at least one cell), which
// also makes the reported limit self-consistent with MaxTokens >= 1.
func TestPromptAtExactContextWindowIsRejected(t *testing.T) {
	const contextWindow = 8192

	tests := []struct {
		name    string
		nPrompt int
		want    bool
	}{
		{
			name:    "one token below the window is admissible",
			nPrompt: contextWindow - 1,
			want:    false,
		},
		{
			name:    "exactly the window leaves no cell to generate into",
			nPrompt: contextWindow,
			want:    true,
		},
		{
			name:    "one token above the window",
			nPrompt: contextWindow + 1,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctxSizingPromptRejected(tt.nPrompt, contextWindow)

			if got != tt.want {
				t.Errorf("prompt of %d token(s) against a %d-token window: rejected = %t, want %t\n"+
					"  kronk:      batchgen_slot_start.go:839 (also :1012, :1112)\n"+
					"                  if s.nPrompt > e.model.cfg.ContextWindow()\n"+
					"  llama.cpp:  .extras/llama.cpp/tools/server/server-context.cpp:3188\n"+
					"                  if (slot.task->n_tokens() >= slot.n_ctx)\n"+
					"              -> send_error(... ERROR_TYPE_EXCEED_CONTEXT_SIZE), slot.release()\n"+
					"  effect:     a prompt of exactly ContextWindow tokens fills KV positions\n"+
					"              0..ContextWindow-1, so the first generation decode\n"+
					"              (batchgen_engine.go:333-334) asks for position ContextWindow,\n"+
					"              llama_decode returns 1, and batchgen_engine.go:464-475 fails\n"+
					"              EVERY active slot after a full-window prefill has already been\n"+
					"              paid for.\n"+
					"  fix:        use >= at all three sites.",
					tt.nPrompt, contextWindow, got, tt.want)
			}
		})
	}
}

// =============================================================================

// ctxSizingPickedWindow is a VERBATIM MIRROR of adjustContextWindow:
//
//	sdk/kronk/model/config.go:804-807
//		// User explicitly set the context window — honor it as-is.
//		if cfg.ContextWindow() > 0 {
//			return cfg
//		}
//	sdk/kronk/model/config.go:811-818  modelCW from GGUF "context_length"
//	sdk/kronk/model/config.go:825
//		pickedCW := min(modelCW, defContextWindow)
//
// adjustContextWindow takes a live llama.Model (it calls searchModelMeta), so
// this mirror isolates the decision. defContextWindow is the real production
// constant (sdk/kronk/model/config.go:50).
//
// MAINTAINER: when config.go:804-825 gains the n_ctx_train cap, update this
// mirror in the same commit.
func ctxSizingPickedWindow(userCW int, modelCW int) int {
	if userCW > 0 {
		return userCW
	}

	return min(modelCW, defContextWindow)
}

// TestPickedContextWindowNeverExceedsTrainedContext pins a missing cap: an
// explicitly configured ContextWindow is honoured verbatim and is never bounded
// by the model's trained context length.
//
// FINDING
//
//	sdk/kronk/model/config.go:804-807 returns immediately for any
//	cfg.ContextWindow() > 0 without reading the GGUF's "<arch>.context_length"
//	at all — that lookup (config.go:811-818) and the min() at config.go:825 are
//	on the auto-pick path only. validateConfig (config.go:525) never looks at
//	ContextWindow either. The value then becomes BOTH the per-request prompt
//	limit (batchgen_slot_start.go:839) and the default MaxTokens
//	(params.go:705-706), and is scaled into ctxParams.NCtx at config.go:862.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-context.cpp:1294-1297
//		int n_ctx_slot = llama_n_ctx_seq(ctx_tgt);
//		if (n_ctx_slot > n_ctx_train) {
//		    SRV_WRN("the slot context (%d) exceeds the training context of the
//		             model (%d) - capping\n", n_ctx_slot, n_ctx_train);
//		    n_ctx_slot = n_ctx_train;
//		}
//	server-context.cpp:1349 then assigns that capped value to every slot. The
//	C API alone only warns (.extras/llama.cpp/src/llama-context.cpp:320-322),
//	so the cap is a deliberate server-level policy — and Kronk is a server.
//
// CONCRETE FAILURE SCENARIO
//
//	The reference MTP target advertises qwen35moe.context_length = 262144, so
//	the auto-pick path is safe (it lands on defContextWindow = 8192). The
//	explicit path is not: WithContextWindow(N) with N above the model's trained
//	context makes Kronk admit prompts and grant a MaxTokens budget out to N
//	positions the RoPE was never trained on. Generation past n_ctx_train
//	degrades without any error, which for a long multi-turn conversation looks
//	exactly like the model forgetting or contradicting what it asserted
//	earlier. llama.cpp's server caps the slot instead and logs the cap.
//
// The fix is to clamp an explicit ContextWindow at the model's trained context
// in adjustContextWindow (which already has the llama.Model handle and already
// reads "context_length") and log the cap the way upstream does.
func TestPickedContextWindowNeverExceedsTrainedContext(t *testing.T) {
	tests := []struct {
		name string
		// userCW is cfg.ContextWindow() as configured (0 = auto-pick).
		userCW int
		// modelCW is the GGUF "<arch>.context_length" value, i.e. n_ctx_train.
		modelCW int
	}{
		{
			name:    "auto-pick on the reference qwen35moe MTP target",
			userCW:  0,
			modelCW: 262144,
		},
		{
			name:    "explicit window equal to the trained context",
			userCW:  262144,
			modelCW: 262144,
		},
		{
			name:    "explicit window above the trained context",
			userCW:  524288,
			modelCW: 262144,
		},
		{
			name:    "explicit 32K window on a 4K-trained model",
			userCW:  32768,
			modelCW: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctxSizingPickedWindow(tt.userCW, tt.modelCW)

			if got > tt.modelCW {
				t.Errorf("effective context window = %d, exceeds the model's trained context %d\n"+
					"  kronk:      config.go:804-807 returns cfg unchanged for any explicit\n"+
					"              ContextWindow; the GGUF \"context_length\" lookup at\n"+
					"              config.go:811-818 and the min() at config.go:825 are on the\n"+
					"              auto-pick path only, and validateConfig (config.go:525) never\n"+
					"              inspects ContextWindow.\n"+
					"  llama.cpp:  .extras/llama.cpp/tools/server/server-context.cpp:1294-1297\n"+
					"                  int n_ctx_slot = llama_n_ctx_seq(ctx_tgt);\n"+
					"                  if (n_ctx_slot > n_ctx_train) { ...capping...;\n"+
					"                      n_ctx_slot = n_ctx_train; }\n"+
					"              and :1349 assigns the capped value to every slot.\n"+
					"  effect:     this value is the prompt limit (batchgen_slot_start.go:839)\n"+
					"              AND the default MaxTokens (params.go:705-706), so Kronk\n"+
					"              silently generates out to positions the RoPE was never\n"+
					"              trained on — degradation with no error, which in a long\n"+
					"              multi-turn conversation reads as the model contradicting\n"+
					"              what it said earlier.\n"+
					"  fix:        clamp an explicit ContextWindow at modelCW in\n"+
					"              adjustContextWindow and log the cap.",
					got, tt.modelCW)
			}
		})
	}
}
