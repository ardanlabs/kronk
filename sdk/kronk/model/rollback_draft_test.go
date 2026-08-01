package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// rollbackDraftBasePast is a VERBATIM MIRROR of the base-position arithmetic
// inside (*batchEngine).rollbackDraft:
//
//	sdk/kronk/model/batchgen_speculative.go:1002
//		draftBasePast := s.draftNPast - llama.Pos(nDraft)
//
//	sdk/kronk/model/batchgen_speculative.go:969   (own-draft-KV MTP branch)
//		draftBasePast := s.draftNPast - llama.Pos(nDraft)
//
// rollbackDraft itself calls llama.MemorySeqRm on a live draft context, so it
// cannot be invoked without a loaded model. This mirror isolates the one piece
// of the function that is pure arithmetic so the contract can be pinned in a
// unit test.
//
// MAINTAINER: when batchgen_speculative.go:969/1002 is fixed, update this
// mirror in the same commit so it keeps tracking the production formula.
func rollbackDraftBasePast(draftNPast llama.Pos, nDraft int) llama.Pos {
	return draftNPast - llama.Pos(nDraft)
}

// TestRollbackDraftBasePastRecoversDraftStartPast pins findings2 §6c: the
// rollbackDraft off-by-one after an EOG-truncated draft.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_speculative.go:1002 (classic drafter)
//   - sdk/kronk/model/batchgen_speculative.go:969  (own-draft-KV MTP)
//
// Both recompute the draft round's base KV position as
// `s.draftNPast - llama.Pos(nDraft)`, i.e. they assume the draft context
// advanced exactly one KV cell per RETURNED draft token.
//
// That assumption does not match llama.DraftGenerate's contract. yzma's
// pkg/llama/draft.go increments its internal nPast immediately after a
// successful llama_decode but BEFORE the EOG check, and on EOG it breaks out of
// the loop before `drafted++`:
//
//	decode -> nPast++ -> sample -> [p_min early stop: break] ->
//	  [EOG: break] -> outTokens[drafted] = token; drafted++
//
// So the number of decodes performed is `drafted` on a full round and on a
// decode failure, but `drafted+1` when the round was truncated by EOG (or by
// the p_min early stop). yzma's own draft_test.go encodes exactly this
// ambiguity by asserting finalPast-startPast is in [drafted, drafted+1]. The
// ambiguity is an upstream CONTRACT, not an upstream bug — the caller is
// responsible for remembering where the round started.
//
// Kronk does capture it: generateDraftTokens saves
// `draftStartPast := s.draftNPast` at batchgen_speculative.go:238, but then
// only uses it for a log line at :286. It is never persisted on the slot, so
// rollbackDraft has to reconstruct it — and reconstructs it wrong on every
// EOG-truncated round. `draftBasePast` comes out one cell too high, the
// MemorySeqRm trim leaves a stale cell behind, and s.draftNPast stays
// misaligned with the target for the rest of the request.
//
// nDraft at the rollbackDraft call site is len(s.specDraftTokens)
// (batchgen_speculative.go:331 via batchgen_engine.go:281), which is
// s.draftTokensBuf[:drafted] — i.e. nDraft == drafted exactly.
//
// The fix is to persist generateDraftTokens' draftStartPast on the slot and use
// it in rollbackDraft instead of subtracting nDraft.
func TestRollbackDraftBasePastRecoversDraftStartPast(t *testing.T) {
	// decodes is the number of successful llama_decode calls DraftGenerate
	// made, i.e. finalPast-startPast. drafted is what it returned, which is
	// what Kronk passes to rollbackDraft as nDraft.
	tests := []struct {
		name      string
		startPast llama.Pos
		nDraftReq int
		decodes   int
		drafted   int
	}{
		{
			name:      "full round, no truncation",
			startPast: 1000,
			nDraftReq: 4,
			decodes:   4,
			drafted:   4,
		},
		{
			name:      "EOG truncated after the third decode",
			startPast: 1000,
			nDraftReq: 4,
			decodes:   3,
			drafted:   2,
		},
		{
			name:      "EOG truncated after the second decode",
			startPast: 1000,
			nDraftReq: 4,
			decodes:   2,
			drafted:   1,
		},
		{
			name:      "decode error on the third iteration",
			startPast: 1000,
			nDraftReq: 4,
			decodes:   2,
			drafted:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Post-conditions of llama.DraftGenerate, as implemented in
			// yzma pkg/llama/draft.go.
			finalPast := tt.startPast + llama.Pos(tt.decodes)

			if delta := int(finalPast - tt.startPast); delta != tt.drafted && delta != tt.drafted+1 {
				t.Fatalf("bad test case: finalPast-startPast = %d, must be drafted (%d) or drafted+1 (%d)",
					delta, tt.drafted, tt.drafted+1)
			}

			if tt.drafted > tt.nDraftReq || tt.decodes > tt.nDraftReq {
				t.Fatalf("bad test case: DraftGenerate loops at most nDraft (%d) times, "+
					"got decodes=%d drafted=%d", tt.nDraftReq, tt.decodes, tt.drafted)
			}

			// generateDraftTokens (batchgen_speculative.go:256-257) stores
			// finalPast on the slot and truncates the token buffer to
			// drafted, so this is the state rollbackDraft observes.
			draftNPast := finalPast
			nDraft := tt.drafted

			got := rollbackDraftBasePast(draftNPast, nDraft)

			if got != tt.startPast {
				t.Errorf("rollbackDraft base position = %d, want %d (off by %d)\n"+
					"batchgen_speculative.go:1002 (and :969) computes\n"+
					"\tdraftBasePast := s.draftNPast - llama.Pos(nDraft)\n"+
					"which assumes one draft-KV cell per RETURNED token. DraftGenerate performed "+
					"%d decode(s) but returned %d token(s): on an EOG-truncated round it advances "+
					"nPast before the EOG check and breaks before drafted++, so it consumed one "+
					"more KV position than it reported.\n"+
					"The recomputed base is too high, MemorySeqRm trims the wrong range and leaves "+
					"a stale draft cell, and s.draftNPast stays misaligned with the target for the "+
					"rest of the request.\n"+
					"Fix: persist generateDraftTokens' draftStartPast (batchgen_speculative.go:238) "+
					"on the slot and use it here instead of subtracting nDraft.",
					got, tt.startPast, int(got-tt.startPast), tt.decodes, tt.drafted)
			}
		})
	}
}
