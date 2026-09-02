// TEMPORARY DIAGNOSTIC — delete once the ROCm draft-acceptance failure is
// understood. Nothing in this file runs unless KRONK_SPEC_DIAG is set.
//
// The failure it exists to explain: gpu.yml's ROCm leg fails
// sdk/kronk/tests/draft with draft=27..33 accepted=0 on every run, while
// the Vulkan leg on identical hardware and the same llama.cpp build
// (b10751) reaches rate=0.45..0.50. classic.Verify returns at the FIRST
// rejection, so accepted=0 means index 0 was rejected on every verify
// step, not that high rows misbehaved.
//
// The tests run at the default temperature (DefTemp = 1.0), so
// verification takes the probabilistic branch: pTarget is read from the
// dense-but-sparse array applySamplerFilters populates, which is zero for
// any token outside the target's top-K/top-P/min-P survivor set. A zero
// pTarget makes ratio 0 and rejection certain. These probes separate the
// three shapes that can produce that:
//
//	drafter broken    candidate detokenizes to nonsense, or the drafter's
//	                  own qDraft for its own pick is ~0
//	target row wrong  the target's argmax for the row detokenizes to
//	                  nonsense while generation output stays coherent, or
//	                  the logits carry NaN/Inf
//	real divergence   both sides sane, candidate simply outside the
//	                  survivor set

package model

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// specDiagEnv reports whether a diagnostic switch is on. Scoped to this
// file so it disappears with it.
func specDiagEnv(key string) bool {
	switch os.Getenv(key) {
	case "", "0", "false":
		return false
	}
	return true
}

// specDiagEnabled gates every probe below. Read once: this is consulted
// per verification row, which is far too hot for a repeated Getenv.
var specDiagEnabled = specDiagEnv("KRONK_SPEC_DIAG")

// specDiagUncapOutputs skips the NOutputsMax / NOutputsMaxPerSeq capping
// in newContextParams, restoring llama.cpp's own default of n_batch rows.
// The cap is the newest code in the verification path (upstream PR 23861)
// and the only thing that bounds the buffer GetLogitsIth reads from, so
// running the draft suite once with this set and once without tells us
// whether the HIP backend mishandles a capped n_outputs_max.
var specDiagUncapOutputs = specDiagEnv("KRONK_SPEC_UNCAP_OUTPUTS")

// specDiagRow carries one verification row's evidence. A struct rather
// than a parameter list because every field is needed to tell the three
// failure shapes apart, and callers should not have to keep nine
// positional arguments straight.
type specDiagRow struct {
	slotID    int
	index     int   // draft position; == len(candidates) for the bonus row
	row       int32 // absolute batch row handed to GetLogitsIth
	baseBatch int32
	candidate llama.Token
	hasCand   bool
	logits    []float32
	probs     []float32              // dense, sparse-populated by applySamplerFilters
	survivors []int                  // top-K indices; only those with probs != 0 survived
	draftDist []llama.DraftCandidate // drafter's own distribution for this position
}

// specDiag logs one verification row.
func (e *batchEngine) specDiag(ctx context.Context, r specDiagRow) {
	argmax, maxVal, nans, negInfs, posInfs := logitsShape(r.logits)

	args := []any{
		"slot", r.slotID,
		"index", r.index,
		"row", r.row,
		"base_batch", r.baseBatch,
		"logits_argmax", argmax,
		"logits_max", fmt.Sprintf("%.4f", maxVal),
		"logits_len", len(r.logits),
		"nan", nans,
		"pos_inf", posInfs,
		"neg_inf", negInfs,
		"suppressed", len(e.model.suppressTokens),
		"target_argmax_text", strconv.Quote(e.specDiagText(r.argmaxToken(argmax))),
	}

	// pTarget is the whole ballgame: zero here guarantees rejection
	// regardless of how sane everything else looks.
	if r.hasCand {
		var pTarget float32
		if int(r.candidate) < len(r.probs) {
			pTarget = r.probs[r.candidate]
		}

		args = append(args,
			"candidate", r.candidate,
			"candidate_text", strconv.Quote(e.specDiagText(r.candidate)),
			"p_target", fmt.Sprintf("%.6g", pTarget),
			"p_target_zero", pTarget == 0,
			"candidate_in_survivors", r.candidateSurvived(),
			"q_draft", fmt.Sprintf("%.6g", r.qDraft()),
		)
	}

	args = append(args,
		"survivors", r.survivorCount(),
		"target_top", e.specDiagTop(r.probs, r.survivors, 5),
		"draft_top", e.specDiagDraftTop(r.draftDist, 5),
	)

	e.model.log(ctx, "spec-diag", args...)
}

// specDiagVerifyResult logs the per-step outcome. The usage block only
// carries suite totals, which cannot show whether rejection happened at
// index 0 every time or somewhere later.
func (e *batchEngine) specDiagVerifyResult(ctx context.Context, slotID int, accepted int, drafted int, ema float64, complete bool) {
	e.model.log(ctx, "spec-diag-step",
		"slot", slotID,
		"accepted", accepted,
		"drafted", drafted,
		"acceptance_ema", fmt.Sprintf("%.4f", ema),
		"complete", complete,
	)
}

// specDiagSampler records whether backend (GPU-side) sampling was armed
// on the draft context. This is the fork the row probes leave open:
// empty distributions mean llama_get_sampled_candidates_count_ith
// returned 0, and that has two very different causes.
//
//	ok=false  llama_set_sampler refused. Kronk can see this and does not
//	          look: batchgen_speculative.go zeroes registeredSampler and
//	          carries on, so speculation silently degrades to nothing.
//	ok=true   the backend accepted the sampler but produced no candidates
//	          during llama_decode, which is an upstream HIP graph-sampling
//	          gap rather than anything Kronk did.
func (e *batchEngine) specDiagSampler(ctx context.Context, slotID int, seqID llama.SeqId, sampler llama.Sampler, ok bool, greedy bool) {
	e.model.log(ctx, "spec-diag-sampler",
		"slot", slotID,
		"seq_id", seqID,
		"sampler", uintptr(sampler),
		"set_sampler_ok", ok,
		"greedy", greedy,
	)
}

// specDiagLogitsError records the GetLogitsIth failure that the
// verification path otherwise swallows: on error it returns
// SamplerAccepted true, so a broken read shows up as an acceptance rather
// than as an error anywhere in the logs.
func (e *batchEngine) specDiagLogitsError(ctx context.Context, slotID int, index int, row int32, err error) {
	e.model.log(ctx, "spec-diag",
		"slot", slotID,
		"index", index,
		"row", row,
		"event", "logits-read-failed",
		"err", err.Error(),
	)
}

// =============================================================================

func (r specDiagRow) argmaxToken(argmax int) llama.Token {
	if argmax < 0 {
		return 0
	}
	return llama.Token(argmax)
}

func (r specDiagRow) qDraft() float32 {
	if !r.hasCand {
		return 0
	}
	for _, c := range r.draftDist {
		if c.Tok == r.candidate {
			return c.Prob
		}
	}
	return 0
}

// candidateSurvived reports whether the drafted token carries a non-zero
// target probability, which is exactly the condition Verify needs to have
// any chance of accepting it.
func (r specDiagRow) candidateSurvived() bool {
	if !r.hasCand || int(r.candidate) >= len(r.probs) {
		return false
	}
	return r.probs[r.candidate] != 0
}

func (r specDiagRow) survivorCount() int {
	n := 0
	for _, idx := range r.survivors {
		if idx >= 0 && idx < len(r.probs) && r.probs[idx] != 0 {
			n++
		}
	}
	return n
}

// logitsShape reports the row's argmax and magnitude plus its non-finite
// entries. Uninitialized device memory shows up here as NaN/+Inf or as an
// absurd magnitude long before it shows up as a rejection.
//
// negInf is counted separately because it is EXPECTED: this runs after
// applySamplerFilters, which calls maskSuppressTokenLogits and writes
// -Inf over every suppressed token. Compare negInf against the suppress
// count in the log line — equal means only masking, more means trouble.
// nans and posInfs have no legitimate source.
func logitsShape(logits []float32) (argmax int, maxVal float32, nans int, negInfs int, posInfs int) {
	argmax = -1
	maxVal = float32(math.Inf(-1))

	for i, l := range logits {
		switch {
		case math.IsNaN(float64(l)):
			nans++
			continue
		case math.IsInf(float64(l), -1):
			negInfs++
			continue
		case math.IsInf(float64(l), 1):
			posInfs++
			continue
		}
		if l > maxVal {
			maxVal = l
			argmax = i
		}
	}

	return argmax, maxVal, nans, negInfs, posInfs
}

// specDiagText detokenizes one token for eyeballing. Truncated and quoted
// by the caller so a stray control byte cannot wreck the log line.
func (e *batchEngine) specDiagText(token llama.Token) string {
	buf := make([]byte, 64)
	l := llama.TokenToPiece(e.model.vocab, token, buf, 0, true)
	if l <= 0 {
		return ""
	}
	return string(buf[:min(int(l), len(buf))])
}

// specDiagTop renders the target's highest-probability survivors. If these
// read as a plausible continuation, the target's row is fine and the
// drafter is the suspect; if they read as noise, the row is.
func (e *batchEngine) specDiagTop(probs []float32, survivors []int, n int) string {
	var b strings.Builder

	count := 0
	for _, idx := range survivors {
		if count == n {
			break
		}
		if idx < 0 || idx >= len(probs) || probs[idx] == 0 {
			continue
		}
		if count > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d:%s:%.4f", idx, strconv.Quote(e.specDiagText(llama.Token(idx))), probs[idx])
		count++
	}

	if count == 0 {
		return "<none>"
	}
	return b.String()
}

// specDiagDraftTop renders the drafter's own distribution for the same
// position, so the two models' opinions sit side by side in one line.
func (e *batchEngine) specDiagDraftTop(dist []llama.DraftCandidate, n int) string {
	if len(dist) == 0 {
		return "<none>"
	}

	var b strings.Builder
	for i, c := range dist {
		if i == n {
			break
		}
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d:%s:%.4f", c.Tok, strconv.Quote(e.specDiagText(c.Tok)), c.Prob)
	}

	return b.String()
}
