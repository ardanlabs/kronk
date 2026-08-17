package model

import (
	"context"
	"fmt"
	"time"

	classicengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/classic"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// prefillDraft decodes generation prompt tokens into the draft model's KV cache.
// Called once after the target model's prefill completes. Uses incremental
// caching: finds the common prefix with the previous request's tokens and
// only decodes the new suffix, avoiding redundant re-prefill of the entire
// prompt on subsequent turns.
func (e *batchEngine) prefillDraft(ctx context.Context, s *slot) error {
	draft := e.model.draft.core()
	tokens := s.draftPromptTokens

	if len(tokens) == 0 {
		// Clear any stale draft KV from a previous request on this slot.
		if len(s.draftCachedTokens) > 0 {
			llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
			s.draftCachedTokens = s.draftCachedTokens[:0]
		}
		s.draftNPast = 0
		s.draftPrefillNeeded = false
		e.model.log(ctx, "speculative", "status", "draft-prefill-skip-empty", "slot", s.id)
		return nil
	}

	prefillStart := time.Now()

	// Find common prefix between this slot's cached tokens and new prompt.
	commonLen := 0
	cached := s.draftCachedTokens
	limit := min(len(cached), len(tokens))
	for commonLen < limit && cached[commonLen] == tokens[commonLen] {
		commonLen++
	}

	// Determine how many new tokens need decoding.
	newTokens := tokens[commonLen:]

	e.model.log(ctx, "speculative", "status", "draft-prefill-start",
		"slot", s.id, "total_tokens", len(tokens),
		"cached", len(cached), "common_prefix", commonLen,
		"new_tokens", len(newTokens))

	nBatch := int(e.model.ctxParams.NBatch)
	if nBatch <= 0 {
		nBatch = e.model.cfg.EffectiveNBatch()
	}

	// Trim divergent suffix from draft KV if we have a partial cache hit.
	// If no common prefix, clear everything and decode from scratch.
	switch {
	case commonLen == 0:
		llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
		e.model.log(ctx, "speculative", "status", "draft-cache-miss",
			"slot", s.id)
	case commonLen < len(cached):
		removed, err := llama.MemorySeqRm(draft.mem, s.seqID, llama.Pos(commonLen), -1)
		if err != nil {
			s.draftCachedTokens = s.draftCachedTokens[:0]
			s.draftNPast = 0
			return fmt.Errorf("removing divergent draft cache suffix: %w", err)
		}
		if !removed {
			if _, err := llama.MemorySeqRm(draft.mem, s.seqID, -1, -1); err != nil {
				s.draftCachedTokens = s.draftCachedTokens[:0]
				s.draftNPast = 0
				return fmt.Errorf("clearing divergent draft cache for seq %d: %w", s.seqID, err)
			}
			commonLen = 0
			newTokens = tokens
			e.model.log(ctx, "speculative", "status", "draft-cache-partial-reset",
				"slot", s.id, "trimmed", len(cached))
			break
		}
		e.model.log(ctx, "speculative", "status", "draft-cache-partial",
			"slot", s.id, "kept", commonLen, "trimmed", len(cached)-commonLen)
	default:
		e.model.log(ctx, "speculative", "status", "draft-cache-hit",
			"slot", s.id, "reused", commonLen)
	}

	// Decode new suffix tokens into draft model in chunks using the
	// pre-allocated prefill batch.
	if len(newTokens) > 0 {
		batch := draft.prefillBatch
		seqIDs := []llama.SeqId{s.seqID}

		for i := 0; i < len(newTokens); i += nBatch {
			batch.Clear()
			end := min(i+nBatch, len(newTokens))

			for j := i; j < end; j++ {
				pos := commonLen + j
				isLast := pos == len(tokens)-1
				if err := batch.Add(newTokens[j], llama.Pos(pos), seqIDs, isLast); err != nil {
					s.draftCachedTokens = s.draftCachedTokens[:0]
					return fmt.Errorf("add draft prefill token at pos %d: %w", pos, err)
				}
			}

			ret, err := llama.Decode(draft.lctx, batch)
			if err != nil || ret != 0 {
				// On failure, invalidate the slot's cache to avoid stale state.
				s.draftCachedTokens = s.draftCachedTokens[:0]
				return fmt.Errorf("draft prefill failed at pos %d: %w", commonLen+i, decodeError(ret, err))
			}
		}
	}

	s.draftNPast = llama.Pos(len(tokens))
	s.draftPromptTokens = nil
	s.draftPrefillNeeded = false

	// Store prompt tokens in the slot for the next request's prefix
	// comparison, reusing the existing buffer when capacity is sufficient.
	if cap(s.draftCachedTokens) >= len(tokens) {
		s.draftCachedTokens = s.draftCachedTokens[:len(tokens)]
	} else {
		s.draftCachedTokens = make([]llama.Token, len(tokens))
	}
	copy(s.draftCachedTokens, tokens)

	e.model.log(ctx, "speculative", "status", "draft-prefill-done",
		"slot", s.id, "draft_nPast", s.draftNPast,
		"decoded", len(newTokens), "reused", commonLen,
		"elapsed", fmtDur(time.Since(prefillStart)))

	return nil
}

// generateClassicDraft invokes the low-level llama draft operation. This
// delegates to llama.DraftGenerate which performs the entire
// decode→sample→capture loop in a single tight function, eliminating per-token
// Go overhead (condition checks, lazy init, buffer management) between FFI calls.
func (e *batchEngine) generateClassicDraft(s *slot, nDraft int) (classicengine.GenerationResult, error) {
	draft := e.model.draft.core()
	temperature := s.job.params.Temperature
	greedy := temperature == 0

	if nDraft == 0 {
		s.draftTokensBuf = s.draftTokensBuf[:0]
		return classicengine.GenerationResult{}, nil
	}

	// Select sampler. Greedy uses the shared draft sampler (argmax).
	// Non-greedy creates or reuses the per-slot draft sampler. Preserve the
	// existing per-round reset for random requests, but let a seeded request's
	// RNG stream advance across rounds instead of replaying its first draws.
	sampler := draft.sampler
	if !greedy {
		if s.draftSampler == 0 {
			s.draftSampler = buildDraftSampler(draft.vocab, draft.suppressTokens, s.job.params, s.samplingSeeds.draftDist)
		} else if s.job.params.Seed == nil {
			llama.SamplerReset(s.draftSampler)
		}
		sampler = s.draftSampler
	}

	// Register the sampler on the draft context for backend (GPU-side)
	// sampling. This enables llama_decode to produce sampled candidates
	// and probabilities as part of the compute graph, making them
	// available via GetSampledCandidatesIth / GetSampledProbsIth.
	// Only re-register when the sampler or seqID changes.
	if draft.registeredSampler != sampler || draft.registeredSeqID != s.seqID {
		if draft.registeredSampler != 0 {
			llama.SetSampler(draft.lctx, draft.registeredSeqID, 0)
		}
		if llama.SetSampler(draft.lctx, s.seqID, sampler) {
			draft.registeredSampler = sampler
			draft.registeredSeqID = s.seqID
		} else {
			draft.registeredSampler = 0
		}
	}

	// Ensure output buffers are large enough.
	if cap(s.draftTokensBuf) < nDraft {
		s.draftTokensBuf = make([]llama.Token, nDraft)
	}
	s.draftTokensBuf = s.draftTokensBuf[:nDraft]

	// Prepare sparse distribution output buffers for non-greedy mode.
	var outDists [][]llama.DraftCandidate
	if !greedy {
		if s.draftCandDistBuf == nil {
			s.draftCandDistBuf = make([][]llama.DraftCandidate, draft.nDraft)
			for i := range s.draftCandDistBuf {
				s.draftCandDistBuf[i] = make([]llama.DraftCandidate, 0, 128)
			}
		}
		outDists = s.draftCandDistBuf[:nDraft]
	}

	s.classic.DraftStartPosition = s.draftNPast

	// Perform the entire draft loop in a single call, minimizing per-token
	// Go overhead between FFI calls.
	drafted, finalPast, err := llama.DraftGenerate(
		draft.lctx,
		&draft.batch,
		e.model.vocab,
		sampler,
		s.sampled,
		s.draftNPast,
		s.seqIDs,
		nDraft,
		greedy,
		s.draftTokensBuf,
		outDists,
	)
	if err != nil {
		return classicengine.GenerationResult{}, fmt.Errorf("draft generate: %w", err)
	}

	s.draftNPast = finalPast
	s.draftTokensBuf = s.draftTokensBuf[:drafted]

	var distributions [][]llama.DraftCandidate
	if !greedy {
		distributions = outDists[:drafted]
	}

	e.model.log(s.job.ctx, "speculative", "status", "draft-generated",
		"slot", s.id, "drafted", len(s.draftTokensBuf), "adaptive_nDraft", nDraft,
		"max_nDraft", draft.nDraft, "acc_ema", fmt.Sprintf("%.2f", s.classic.AcceptanceEMA),
		"draft_nPast_before", s.classic.DraftStartPosition, "draft_nPast_after", s.draftNPast)

	return classicengine.GenerationResult{Candidates: s.draftTokensBuf, Distributions: distributions}, nil
}

func specAcceptedNPast(basePast llama.Pos, accepted int) llama.Pos {
	return basePast + llama.Pos(1+accepted)
}

// captureTargetSpecSnapshot saves the target context's per-sequence
// state for s.seqID into s.specSnapshot. This is the prerequisite for
// recovering from a partial-rejection spec round on a hybrid target —
// the per-seq recurrent state has no per-position trim, so the only
// way to roll back is to restore a pre-spec snapshot and re-decode the
// accepted prefix.
//
// Buffer is lazy-grow / never-shrink. Required size scales with the
// sequence's current KV occupancy and is queried via StateSeqGetSize
// each round (a cheap C call). Net per-spec-round overhead is the cost
// of two memcpys of the seq state (~10ms for a 27B Q8 model with a few
// hundred context tokens at first; grows with context length).
func (e *batchEngine) captureTargetSpecSnapshot(s *slot) error {
	size := llama.StateSeqGetSize(e.model.lctx, s.seqID)
	if size == 0 {
		return fmt.Errorf("state-seq-get-size returned 0 for seq %d", s.seqID)
	}

	switch {
	case uint64(cap(s.specSnapshot)) < size:
		s.specSnapshot = make([]byte, size)
	default:
		s.specSnapshot = s.specSnapshot[:size]
	}

	e.model.decodeMu.Lock()
	n := llama.StateSeqGetData(e.model.lctx, s.specSnapshot, s.seqID)
	e.model.decodeMu.Unlock()

	if n != size {
		s.specSnapshot = s.specSnapshot[:0]
		return fmt.Errorf("state-seq-get-data short read: got %d want %d for seq %d", n, size, s.seqID)
	}
	return nil
}

// restoreTargetSpecSnapshot rewinds the target context to the pre-spec
// state captured in s.specSnapshot, then re-decodes the accepted prefix
// (s.sampled + the first `accepted` draft tokens) at positions
// [basePast .. basePast+accepted] so the seq state is left consistent
// with s.nPast == basePast + 1 + accepted.
//
// Called only for hybrid targets on partial rejection — dense / pure-
// attention targets use MemorySeqRm and skip this path entirely.
//
// On success the target's KV+recurrent state is exactly as it was
// before the spec batch, plus the accepted prefix re-applied. The caller
// fails and clears the slot when restore or re-decode does not complete.
func (e *batchEngine) restoreTargetSpecSnapshot(s *slot, basePast llama.Pos, sampledAtBase llama.Token, draftTokens []llama.Token, accepted int) error {
	e.model.decodeMu.Lock()
	n := llama.StateSeqSetData(e.model.lctx, s.specSnapshot, s.seqID)
	e.model.decodeMu.Unlock()
	if n != uint64(len(s.specSnapshot)) {
		return fmt.Errorf("state-seq-set-data short restore: got %d want %d for seq %d", n, len(s.specSnapshot), s.seqID)
	}

	// Re-decode the accepted prefix into the now-rewound seq. The
	// re-batch is small (1 + accepted tokens, capped at nDraft+1)
	// so BatchInit/BatchFree per round is negligible. logits=true
	// only on the LAST position because classic verification
	// already sampled and emitted its accepted tokens from the
	// original spec batch's logits; we don't need them again here.
	//
	// sampledAtBase is the ORIGINAL s.sampled captured before the
	// verify loop ran — not the current s.sampled, which has been
	// overwritten by handleSampledToken as each accepted draft was
	// emitted. Using the current value would re-decode the wrong
	// token at position basePast and corrupt every subsequent round.
	count := 1 + accepted
	rebatch := llama.BatchInit(int32(count), 0, 1)
	defer llama.BatchFree(rebatch)

	if err := rebatch.Add(sampledAtBase, basePast, s.seqIDs, accepted == 0); err != nil {
		return fmt.Errorf("add speculative restore base token: %w", err)
	}
	for i := range accepted {
		isLast := i == accepted-1
		if err := rebatch.Add(draftTokens[i], basePast+llama.Pos(1+i), s.seqIDs, isLast); err != nil {
			return fmt.Errorf("add speculative restore draft token %d: %w", i, err)
		}
	}

	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, rebatch)
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
	}
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return fmt.Errorf("re-decode of accepted prefix failed: %w", decodeError(ret, err))
	}
	return nil
}

// rollbackDraft removes rejected draft tokens from the draft model's KV cache
// and updates the slot's draft position to stay in sync with the target.
func (e *batchEngine) rollbackDraft(ctx context.Context, s *slot, accepted, nDraft int) error {
	dr := e.model.draft
	if dr == nil {
		return nil
	}
	draft := dr.core()

	// During generateDraftTokens, the draft model decoded tokens at positions:
	//   draftBasePast+0: s.sampled
	//   draftBasePast+1: draft[0]
	//   ...
	//   draftBasePast+nDraft-1: draft[nDraft-2]
	//
	// Note: draft[nDraft-1] was sampled but NOT decoded (not in KV cache).
	// The actual KV end is draftBasePast + nDraft, but position nDraft-1
	// holds draft[nDraft-2], not draft[nDraft-1].
	//
	// After drafting: s.draftNPast = draftBasePast + nDraft
	//
	// We want to keep: s.sampled + accepted drafts decoded into KV.
	// The draft decoded s.sampled + draft[0..nDraft-2], so the KV contains
	// nDraft entries at positions draftBasePast through draftBasePast+nDraft-1.
	//
	// For accepted < nDraft:
	//   Keep positions draftBasePast..draftBasePast+accepted (accepted+1 entries).
	//   Remove positions draftBasePast+accepted+1 through draftBasePast+nDraft-1.
	//
	// For accepted == nDraft (all accepted):
	//   Keep all decoded positions. But draft[nDraft-1] was sampled, not decoded,
	//   so the KV only extends to draftBasePast+nDraft-1. Set draftNPast to the
	//   actual KV end (draftBasePast + nDraft), not beyond it.
	draftBasePast := s.classic.DraftStartPosition
	draftKVEnd := s.draftNPast
	draftKeep := classicengine.DraftKeepPosition(draftBasePast, draftKVEnd, accepted)

	if draftKeep < draftKVEnd {
		removed, err := llama.MemorySeqRm(draft.mem, s.seqID, draftKeep, draftKVEnd)
		if err != nil {
			return fmt.Errorf("removing rejected classic draft positions: %w", err)
		}
		if !removed {
			return fmt.Errorf("removing rejected classic draft positions for seq %d", s.seqID)
		}
	}

	// Update draft nPast to the next write position after kept tokens.
	s.draftNPast = draftKeep

	e.model.log(ctx, "speculative", "status", "draft-rollback",
		"slot", s.id, "accepted", accepted, "nDraft", nDraft,
		"draft_base", draftBasePast, "draft_keep", draftKeep,
		"draft_kv_end", draftKVEnd, "draft_nPast", s.draftNPast)

	return nil
}
