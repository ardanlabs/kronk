package model

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// Speculative verify loop — position arithmetic and sampler-accept bookkeeping.
//
// Both regressions pinned here live inside (*batchEngine).verifySpeculativeTokens
// and (*batchEngine).finalizeSpeculativeTokens, which cannot be called without a
// live llama.Context holding a decoded speculative batch: every branch begins
// with llama.GetLogitsIth / llama.SamplerSample against e.model.lctx. Following
// the convention established by rollback_draft_test.go, the pure arithmetic and
// the pure control flow are extracted into VERBATIM MIRRORS below, each citing
// the production line it mirrors, and the mirrors are pinned instead.
//
// The mirrors are backed by an AST guard (TestVerifyLoopSamplerAcceptIsNotDoubled
// subtest "production-shape") so they cannot silently drift away from the code
// they claim to mirror.
//
// Reference semantics used as ground truth throughout:
//
//   - Spec batch layout. Kronk (batchgen_engine.go:284-287) and llama.cpp
//     (tools/server/server-context.cpp:504-516) build the identical batch:
//     row baseBatch+0 = s.sampled at position basePast, row baseBatch+1+k =
//     draft[k] at position basePast+1+k. Both request logits on all 1+nDraft
//     rows.
//   - Logits index. llama_get_logits_ith takes the ORIGINAL BATCH TOKEN INDEX
//     and translates it through output_ids (src/llama-context.cpp:853-858), so
//     Kronk's baseBatch+i indices are the correct addressing mode. Verified —
//     no finding here beyond the already-known §6e hybrid-restore case.
//   - Draft width. common/speculative.cpp:1343-1346 caps n_max at the number of
//     nextn layers ONLY for chain_heads models (n_mtp_layers > 1). For a single
//     trained head (qwen35moe, nextn_predict_layers=1 in the shipped
//     mtp-Qwen3.6-35B-A3B GGUF) chain_heads is false, no cap is applied, and
//     speculative.cpp:1641 re-feeds the head its own previous hidden row at
//     dp.n_past+i+1 — exactly what generateDraftTokensMTP does. Upstream's own
//     default width is n_max=3 (common/common.h:325), above Kronk's
//     defMTPNDraft=2. Autoregressive multi-token drafting from one head is the
//     reference behaviour, so there is deliberately NO test asserting
//     nDraft <= nextn_predict_layers.

// verifyAcceptNPast is a VERBATIM MIRROR of the in-loop s.nPast update performed
// by (*batchEngine).verifySpeculativeTokens for each ACCEPTED draft token:
//
//	sdk/kronk/model/batchgen_speculative.go:454   (greedy / MTP branch)
//		s.nPast = basePast + llama.Pos(1+i)
//
//	sdk/kronk/model/batchgen_speculative.go:507   (sparse probabilistic branch)
//		s.nPast = basePast + llama.Pos(1+i)
//
//	sdk/kronk/model/batchgen_speculative.go:564   (full-vocab probabilistic branch)
//		s.nPast = basePast + llama.Pos(1+i)
//
// `i` is the loop index over draftTokens, so it is the index of the draft token
// that was just accepted and emitted.
//
// MAINTAINER: when batchgen_speculative.go:454/507/564 is fixed, update this
// mirror in the same commit so it keeps tracking the production formula.
func verifyAcceptNPast(basePast llama.Pos, i int) llama.Pos {
	return basePast + llama.Pos(1+i)
}

// finalizeAcceptedNPast is a VERBATIM MIRROR of the authoritative post-round
// s.nPast update performed by (*batchEngine).finalizeSpeculativeTokens:
//
//	sdk/kronk/model/batchgen_speculative.go:740
//		s.nPast = basePast + llama.Pos(1+accepted)
//
// It is also the formula the rollback range is derived from at
// batchgen_speculative.go:682 (rollbackFrom), so it is the value the target KV
// is actually trimmed to.
//
// MAINTAINER: keep in lock-step with batchgen_speculative.go:740.
func finalizeAcceptedNPast(basePast llama.Pos, accepted int) llama.Pos {
	return basePast + llama.Pos(1+accepted)
}

// TestVerifyLoopNPastAgreesWithFinalizeNPast pins a position-arithmetic
// off-by-one inside the speculative verify loop.
//
// FINDING
// The in-loop s.nPast update in Phase A is one KV cell BEHIND the authoritative
// Phase B update for the same state. After accepting draft token `i` the slot
// has emitted i+1 draft tokens, so `accepted == i+1`, and the two formulas must
// name the same next-write position. They do not: Phase A writes
// basePast+1+i while Phase B writes basePast+1+(i+1) == basePast+2+i.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_speculative.go:454 (greedy / MTP verify branch)
//   - sdk/kronk/model/batchgen_speculative.go:507 (sparse verify branch)
//   - sdk/kronk/model/batchgen_speculative.go:564 (full-vocab verify branch)
//   - sdk/kronk/model/batchgen_speculative.go:740 (authoritative Phase B value)
//
// LLAMA.CPP REFERENCE
// tools/server/server-context.cpp:3936-3942. Upstream reconstructs the prompt
// after verification as
//
//	slot.prompt.tokens.keep_first(slot.prompt.n_tokens() - n_draft);   // :3936
//	slot.prompt.tokens.insert({ids.begin(), ids.end() - 1});           // :3937
//	slot.mem.seq_rm(slot.id, slot.prompt.tokens.pos_next(), -1);       // :3942
//
// With P == basePast before the round, handle_last_sampled_token
// (server-context.cpp:504-516) grew the prompt to P+1+n_draft; keep_first then
// leaves P+1 and inserting the n_accepted accepted tokens leaves P+1+n_accepted.
// So `pos_next() == basePast + 1 + accepted` is upstream's next write position —
// which is exactly Kronk's Phase B formula and exactly what the KV was trimmed
// to. The Phase A formula is one short.
//
// The batch layout confirms it independently: row baseBatch+k of the spec batch
// carries position basePast+k (batchgen_engine.go:284-287), so after draft[i] is
// accepted the KV holds positions basePast .. basePast+1+i INCLUSIVE and the
// next free cell is basePast+2+i.
//
// CONCRETE FAILURE SCENARIO
// Today the wrong value is always overwritten by Phase B a few statements later,
// so it is a latent landmine rather than a live corruption — but it is only
// latent because of an ordering that the Phase A doc comment itself says does
// not hold ("does NOT advance s.nPast — those are deferred to
// finalizeSpeculativeTokens"). Anything that observes s.nPast between Phase A
// and Phase B sees a sequence one cell shorter than the real KV:
//
//   - Phase A calls handleSpeculativeToken per accepted token
//     (batchgen_speculative.go:455), which reaches finishSlot on EOG /
//     MaxTokens / a stream error. Phase B is then skipped entirely
//     (specPendingFinalize stays false), so on those paths s.nPast is left
//     permanently one cell behind the KV contents.
//   - The batch-overflow diagnostic (batchgen_engine.go:440) and the
//     verify-done log (batchgen_speculative.go:752) both report s.nPast.
//   - Adding the missing context-length guard (findings §6b) at any point
//     between the two phases would compute the wrong remaining window.
//
// Because llama.cpp appends rather than overwrites by (seq, pos), an nPast that
// under-counts the KV is the precise precondition for phantom tokens: the next
// decode re-uses a position that already holds a cell.
//
// The fix is to delete the three in-loop assignments (Phase A is documented as
// non-mutating for nPast) or to write basePast+llama.Pos(2+i).
func TestVerifyLoopNPastAgreesWithFinalizeNPast(t *testing.T) {
	tests := []struct {
		name      string
		basePast  llama.Pos
		nDraft    int
		acceptIdx int // index of the draft token just accepted and emitted
	}{
		{
			name:      "first of two drafts accepted",
			basePast:  1000,
			nDraft:    2,
			acceptIdx: 0,
		},
		{
			name:      "both of two drafts accepted",
			basePast:  1000,
			nDraft:    2,
			acceptIdx: 1,
		},
		{
			name:      "single draft accepted",
			basePast:  4095,
			nDraft:    1,
			acceptIdx: 0,
		},
		{
			name:      "third of three drafts accepted",
			basePast:  0,
			nDraft:    3,
			acceptIdx: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.acceptIdx < 0 || tt.acceptIdx >= tt.nDraft {
				t.Fatalf("bad test case: acceptIdx %d outside [0,%d)", tt.acceptIdx, tt.nDraft)
			}

			// The verify loop increments `accepted` immediately before it
			// updates s.nPast (batchgen_speculative.go:452-454), so accepting
			// draft index i means accepted == i+1.
			accepted := tt.acceptIdx + 1

			// Ground truth from the spec batch layout: row baseBatch+k holds
			// position basePast+k, so accepting draft[acceptIdx] leaves the KV
			// occupied through basePast+1+acceptIdx inclusive.
			lastOccupied := tt.basePast + llama.Pos(1+tt.acceptIdx)
			wantNext := lastOccupied + 1

			gotPhaseB := finalizeAcceptedNPast(tt.basePast, accepted)
			if gotPhaseB != wantNext {
				t.Fatalf("mirror of batchgen_speculative.go:740 is out of date: "+
					"got %d, want %d (basePast %d, accepted %d)",
					gotPhaseB, wantNext, tt.basePast, accepted)
			}

			gotPhaseA := verifyAcceptNPast(tt.basePast, tt.acceptIdx)

			if gotPhaseA != gotPhaseB {
				t.Errorf("Phase A nPast = %d, Phase B nPast = %d (off by %d)\n"+
					"batchgen_speculative.go:454 (and :507, :564) compute\n"+
					"\ts.nPast = basePast + llama.Pos(1+i)\n"+
					"for the token at draft index i, while batchgen_speculative.go:740 "+
					"computes the authoritative value\n"+
					"\ts.nPast = basePast + llama.Pos(1+accepted)\n"+
					"with accepted == i+1. Both describe the same state, so they must agree.\n"+
					"Ground truth: the spec batch put s.sampled at basePast and draft[k] at "+
					"basePast+1+k (batchgen_engine.go:284-287), so accepting draft[%d] leaves "+
					"the KV occupied through position %d and the next write position is %d. "+
					"llama.cpp derives the same value as prompt.tokens.pos_next() after "+
					"keep_first/insert (tools/server/server-context.cpp:3936-3942).\n"+
					"Phase A's value under-counts the KV by one cell. It is masked only "+
					"because Phase B overwrites it — but Phase B is skipped whenever Phase A's "+
					"handleSpeculativeToken (batchgen_speculative.go:455) reaches finishSlot "+
					"(EOG / MaxTokens / stream error), and every reader between the two phases "+
					"sees the short value.\n"+
					"Fix: drop the three in-loop assignments (Phase A is documented at "+
					"batchgen_speculative.go:297-299 as NOT advancing s.nPast) or write "+
					"basePast + llama.Pos(2+i).",
					gotPhaseA, gotPhaseB, int(gotPhaseB-gotPhaseA), tt.acceptIdx, lastOccupied, wantNext)
			}
		})
	}
}

// specRoundSamplerAccepts is a VERBATIM MIRROR of the llama_sampler_accept
// bookkeeping performed by one MTP (greedy-forced) speculative round. It returns
// the tokens Kronk EMITS through the streaming pipeline and, separately, every
// token that reaches the slot's sampler chain via llama_sampler_accept.
//
// The mirrored production statements, in execution order:
//
//	sdk/kronk/model/batchgen_speculative.go:433
//		targetTok = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
//	  -> yzma SamplerSample (.extras/yzma/pkg/llama/sampling.go:574-583) is a thin
//	     FFI wrapper over llama_sampler_sample, which ENDS WITH
//	     llama_sampler_accept(smpl, token) (.extras/llama.cpp/src/llama-sampler.cpp:873).
//	     So sampling a position already accepts that token into the chain.
//
//	sdk/kronk/model/batchgen_speculative.go:451-455 (draft accepted)
//		if draftToken == targetTok { ... e.handleSpeculativeToken(s, draftToken, ...) }
//	  -> handleSpeculativeToken (batchgen_tokens.go:245-247) -> handleSampledToken
//	     -> llama.SamplerAccept(s.sampler, token) (batchgen_tokens.go:63).
//	     A SECOND accept of the same token.
//
//	sdk/kronk/model/batchgen_speculative.go:465-466 (draft rejected)
//		bonusToken = targetTok; break
//	  -> no further sampler call; targetTok was already accepted at :433.
//
//	sdk/kronk/model/batchgen_speculative.go:593-595 (all drafts accepted)
//		case greedy: ... bonusToken = argmax(targetLogits)
//	  -> argmax (batchgen_speculative.go:865) touches no sampler, so the bonus
//	     token is NOT implicitly accepted on this path.
//
//	sdk/kronk/model/batchgen_speculative.go:762
//		e.handleSpeculativeToken(s, bonusToken, baseBatch+int32(accepted), buf)
//	  -> one accept of the bonus token via batchgen_tokens.go:63.
//
// targetSampled[i] is the token the target model selects at spec-batch row
// baseBatch+i; it must have len(draft)+1 entries (the bonus row included).
//
// MAINTAINER: this mirror encodes the SHAPE of the production code, not just its
// arithmetic. The "production-shape" subtest of
// TestVerifyLoopSamplerAcceptIsNotDoubled re-derives that shape from the AST, so
// update both together.
func specRoundSamplerAccepts(draft []llama.Token, targetSampled []llama.Token) (emitted []llama.Token, accepts []llama.Token) {
	accepted := 0
	var bonusToken llama.Token

	for i := range draft {
		// batchgen_speculative.go:433 -> src/llama-sampler.cpp:873.
		targetTok := targetSampled[i]
		accepts = append(accepts, targetTok)

		if draft[i] == targetTok {
			accepted++

			// batchgen_speculative.go:455 -> batchgen_tokens.go:63.
			emitted = append(emitted, draft[i])
			accepts = append(accepts, draft[i])

			continue
		}

		// batchgen_speculative.go:465-466.
		bonusToken = targetTok

		break
	}

	if accepted == len(draft) {
		// batchgen_speculative.go:593-595: raw argmax, no sampler call.
		bonusToken = targetSampled[len(draft)]
	}

	// batchgen_speculative.go:762 -> batchgen_tokens.go:63.
	emitted = append(emitted, bonusToken)
	accepts = append(accepts, bonusToken)

	return emitted, accepts
}

// TestVerifyLoopSamplerAcceptIsNotDoubled pins a sampler-state divergence in the
// speculative verify loop: every token it emits is accepted into the slot's
// sampler chain TWICE, and the MTP bonus token on a fully-accepted round is
// accepted only ONCE, so the penalty history is a non-uniform distortion of the
// text the model actually produced.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_speculative.go:433 (verify-position sample; also
//     :431 grammar variant, :442, :493, :536, :590)
//   - sdk/kronk/model/batchgen_tokens.go:63 (the explicit second accept)
//   - sdk/kronk/model/batchgen_speculative.go:595 (bonus token via argmax — the
//     one emitted token that is NOT double-accepted)
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/src/llama-sampler.cpp:873 — llama_sampler_sample ends
//     with `llama_sampler_accept(smpl, token);`. Sampling accepts.
//   - .extras/llama.cpp/common/sampling.cpp:646-674 — the reference verify loop
//     `common_sampler_sample_and_accept_n` therefore does NOT call
//     llama_sampler_sample. It calls common_sampler_sample (which uses
//     llama_sampler_apply directly, common/sampling.cpp:600-644) and then
//     exactly one `common_sampler_accept(gsmpl, id, true)` per token it returns
//     (common/sampling.cpp:655 and :670). Upstream emits every token in `ids`
//     (tools/server/server-context.cpp:3944-3961), so accepts == emitted, once
//     each. That 1:1 invariant is the contract this test asserts.
//
// MECHANISM
// llama_sampler_chain_accept forwards to every sampler in the chain
// (src/llama-sampler.cpp:634-644). Kronk's chain (params.go:763-828) includes
// llama_sampler_init_penalties whenever repeat_penalty != 1.0 ||
// frequency_penalty != 0 || presence_penalty != 0 (params.go:784-788) and
// llama_sampler_init_dry when dry_multiplier > 0 (params.go:790-793) — the only
// two stateful samplers in the chain. llama_sampler_penalties_accept
// (src/llama-sampler.cpp:2657-2676) increments token_count[token] and pushes
// onto a ring buffer bounded by penalty_last_n. Double-accepting therefore
// halves the effective penalty window (repeat_last_n 64 covers ~32 real tokens)
// and doubles every repeat/frequency/presence count.
//
// CONCRETE FAILURE SCENARIO
// A request that sets any penalty parameter (frequency_penalty and
// presence_penalty are standard OpenAI-compatible fields, so most clients send
// them) runs with a penalty history that no longer matches the emitted text. On
// MTP the distortion is not even uniform: with defMTPNDraft=2 and the ~0.71
// acceptance measured on mtp-Qwen3.6-35B-A3B, a fully-accepted round contributes
// 2 accepts for each of its 2 draft tokens but only 1 for the bonus token
// (batchgen_speculative.go:595 uses argmax and never touches the sampler), while
// a rejected round contributes 2 for the rejected position's token. The ring
// buffer thus advances at a rate that depends on the acceptance rate, so the
// window over recent text expands and contracts as acceptance drifts — long
// generations drift away from the penalty state the same text would have
// produced without speculation.
//
// Fix: make the token source and the accept a single step. Either drop
// llama.SamplerAccept at batchgen_tokens.go:63 (accepting that
// llama.SamplerSample already did it, and adding an explicit accept only on the
// paths that bypass it, e.g. argmax), or stop using llama.SamplerSample and
// mirror common_sampler_sample with llama.SamplerApply + one explicit
// llama.SamplerAccept — which is also what §1's grammar fix needs.
func TestVerifyLoopSamplerAcceptIsNotDoubled(t *testing.T) {
	t.Run("accept-count", func(t *testing.T) {
		tests := []struct {
			name          string
			draft         []llama.Token
			targetSampled []llama.Token
		}{
			{
				name:          "both drafts accepted, bonus via argmax",
				draft:         []llama.Token{101, 102},
				targetSampled: []llama.Token{101, 102, 103},
			},
			{
				name:          "first draft accepted, second rejected",
				draft:         []llama.Token{101, 102},
				targetSampled: []llama.Token{101, 999, 103},
			},
			{
				name:          "first draft rejected",
				draft:         []llama.Token{101, 102},
				targetSampled: []llama.Token{999, 102, 103},
			},
			{
				name:          "single draft accepted",
				draft:         []llama.Token{101},
				targetSampled: []llama.Token{101, 102},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if len(tt.targetSampled) != len(tt.draft)+1 {
					t.Fatalf("bad test case: targetSampled must hold len(draft)+1 = %d rows, got %d",
						len(tt.draft)+1, len(tt.targetSampled))
				}

				emitted, accepts := specRoundSamplerAccepts(tt.draft, tt.targetSampled)

				if len(emitted) == 0 {
					t.Fatal("mirror produced no emitted tokens; every verify round emits at least the bonus token")
				}

				// llama.cpp's contract: common_sampler_sample_and_accept_n
				// accepts exactly the tokens it returns, once each, in order
				// (common/sampling.cpp:653-672), and the server emits all of
				// them (tools/server/server-context.cpp:3944-3961).
				if len(accepts) != len(emitted) {
					t.Errorf("sampler accepts = %v (%d), emitted tokens = %v (%d)\n"+
						"llama.cpp accepts exactly one token per emitted token "+
						"(common/sampling.cpp:646-674, accept at :655 and :670).\n"+
						"Kronk accepts %d times for %d emitted tokens because "+
						"batchgen_speculative.go:433 uses llama.SamplerSample, whose C "+
						"implementation already calls llama_sampler_accept "+
						"(.extras/llama.cpp/src/llama-sampler.cpp:873), and then "+
						"batchgen_tokens.go:63 accepts the same token again.\n"+
						"llama_sampler_chain_accept forwards to every sampler "+
						"(src/llama-sampler.cpp:634-644), so the penalties ring buffer "+
						"(src/llama-sampler.cpp:2657-2676) and DRY see each token twice: "+
						"repeat_last_n=64 covers ~32 real tokens and every "+
						"repeat/frequency/presence count is doubled.\n"+
						"Note the distortion is NOT uniform — the bonus token on a "+
						"fully-accepted round is produced by argmax "+
						"(batchgen_speculative.go:595), which touches no sampler, so it is "+
						"accepted once while every other token is accepted twice. The "+
						"penalty window therefore advances at a rate that depends on the "+
						"acceptance rate.",
						accepts, len(accepts), emitted, len(emitted), len(accepts), len(emitted))

					return
				}

				for i := range emitted {
					if accepts[i] != emitted[i] {
						t.Errorf("accept[%d] = %d, emitted[%d] = %d: the accepted sequence must be "+
							"the emitted sequence (common/sampling.cpp:653-672)",
							i, accepts[i], i, emitted[i])
					}
				}
			})
		}
	})

	// production-shape re-derives from the AST the two facts the mirror above
	// encodes, so the mirror cannot silently stop describing the code. It fails
	// while BOTH halves of the double accept are present and passes once either
	// half is removed.
	t.Run("production-shape", func(t *testing.T) {
		root := kronkRepoRoot(t)
		dir := filepath.Join(root, "sdk", "kronk", "model")

		fset := token.NewFileSet()

		specPath := filepath.Join(dir, "batchgen_speculative.go")
		specFile := parseKronkSource(t, fset, specPath)
		verify := findKronkFunc(t, specFile, specPath, "verifySpeculativeTokens")

		tokensPath := filepath.Join(dir, "batchgen_tokens.go")
		tokensFile := parseKronkSource(t, fset, tokensPath)
		handle := findKronkFunc(t, tokensFile, tokensPath, "handleSampledToken")

		var sampleSites []string
		ast.Inspect(verify, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok && isLlamaCall(call, "SamplerSample") {
				sampleSites = append(sampleSites, srcPos(fset, root, call.Pos()))
			}
			return true
		})

		var acceptSites []string
		ast.Inspect(handle, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok && isLlamaCall(call, "SamplerAccept") {
				acceptSites = append(acceptSites, srcPos(fset, root, call.Pos()))
			}
			return true
		})

		if len(sampleSites) > 0 && len(acceptSites) > 0 {
			t.Errorf("double sampler accept still present.\n"+
				"verifySpeculativeTokens obtains its target tokens with llama.SamplerSample at %v, "+
				"and llama_sampler_sample accepts the token it returns "+
				"(.extras/llama.cpp/src/llama-sampler.cpp:873).\n"+
				"handleSampledToken then accepts the same token again with llama.SamplerAccept at %v.\n"+
				"llama.cpp's verify loop accepts exactly once per emitted token: it samples via "+
				"common_sampler_sample (llama_sampler_apply, no accept) and calls "+
				"common_sampler_accept once (common/sampling.cpp:646-674).\n"+
				"Fix one side: drop the explicit accept, or replace llama.SamplerSample with "+
				"llama.SamplerApply-based sampling.",
				sampleSites, acceptSites)
		}
	})
}
