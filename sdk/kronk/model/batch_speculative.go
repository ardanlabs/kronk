package model

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// prefillDraft decodes prompt tokens into the draft model's KV cache.
// Called once after the target model's prefill completes. Uses incremental
// caching: finds the common prefix with the previous request's tokens and
// only decodes the new suffix, avoiding redundant re-prefill of the entire
// prompt on subsequent turns.
func (e *batchEngine) prefillDraft(ctx context.Context, s *slot) error {
	draft := e.model.draft
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
		nBatch = e.model.cfg.NBatch()
	}

	// Trim divergent suffix from draft KV if we have a partial cache hit.
	// If no common prefix, clear everything and decode from scratch.
	switch {
	case commonLen == 0:
		llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
		e.model.log(ctx, "speculative", "status", "draft-cache-miss",
			"slot", s.id)
	case commonLen < len(cached):
		llama.MemorySeqRm(draft.mem, s.seqID, llama.Pos(commonLen), -1)
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
				batch.Add(newTokens[j], llama.Pos(pos), seqIDs, isLast)
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

// chooseNDraft returns the number of draft tokens to generate based on the
// slot's acceptance rate EMA. When acceptance is low, drafting fewer tokens
// (or none) avoids wasting GPU cycles on likely-rejected candidates.
func chooseNDraft(s *slot, maxDraft int) int {
	switch {
	case s.specAccEMA < 0.3:
		return min(1, maxDraft)
	case s.specAccEMA < 0.5:
		return min(1, maxDraft)
	case s.specAccEMA < 0.7:
		return min(2, maxDraft)
	case s.specAccEMA < 0.85:
		return min(3, maxDraft)
	default:
		return maxDraft
	}
}

// generateDraftTokens auto-regressively generates candidate tokens using the
// draft model. This delegates to llama.DraftGenerate which performs the entire
// decode→sample→capture loop in a single tight function, eliminating per-token
// Go overhead (condition checks, lazy init, buffer management) between FFI calls.
//
// For proper speculative sampling (Leviathan et al., 2023), non-greedy mode
// captures the draft model's sparse probability distribution at each step.
// The sparse distributions are stored in s.specDraftDistsSparse for verification.
func (e *batchEngine) generateDraftTokens(s *slot) []llama.Token {
	draft := e.model.draft
	temperature := s.job.params.Temperature
	greedy := temperature == 0

	nDraft := chooseNDraft(s, draft.nDraft)
	if nDraft == 0 {
		s.draftTokensBuf = s.draftTokensBuf[:0]
		s.specDraftProbs = nil
		s.specDraftDistsSparse = nil
		return s.draftTokensBuf
	}

	// Select sampler. Greedy uses the shared draft sampler (argmax).
	// Non-greedy creates or reuses the per-slot draft sampler and resets it
	// so rejected token history from the previous speculative round doesn't
	// accumulate and skew proposals.
	sampler := draft.sampler
	if !greedy {
		if s.draftSampler == 0 {
			s.draftSampler = buildDraftSampler(s.job.params)
		} else {
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

	draftStartPast := s.draftNPast

	// Perform the entire draft loop in a single call, minimizing per-token
	// Go overhead between FFI calls.
	drafted, finalPast := llama.DraftGenerate(
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

	s.draftNPast = finalPast
	s.draftTokensBuf = s.draftTokensBuf[:drafted]

	// Convert sparse distributions from llama.DraftCandidate to candidateEntry.
	if greedy {
		s.specDraftProbs = nil
		s.specDraftDistsSparse = nil
	} else {
		s.specDraftProbs = nil
		if s.draftDistBuf == nil || cap(s.draftDistBuf) < drafted {
			s.draftDistBuf = make([][]candidateEntry, draft.nDraft)
			for i := range s.draftDistBuf {
				s.draftDistBuf[i] = make([]candidateEntry, 0, 128)
			}
		}
		for i := range drafted {
			src := outDists[i]
			s.draftDistBuf[i] = s.draftDistBuf[i][:0]
			for _, c := range src {
				s.draftDistBuf[i] = append(s.draftDistBuf[i], candidateEntry{tok: c.Tok, prob: c.Prob})
			}
		}
		s.specDraftDistsSparse = s.draftDistBuf[:drafted]
	}

	s.specDraftedTotal += drafted

	e.model.log(s.job.ctx, "speculative", "status", "draft-generated",
		"slot", s.id, "drafted", len(s.draftTokensBuf), "adaptive_nDraft", nDraft,
		"max_nDraft", draft.nDraft, "acc_ema", fmt.Sprintf("%.2f", s.specAccEMA),
		"draft_nPast_before", draftStartPast, "draft_nPast_after", s.draftNPast)

	return s.draftTokensBuf
}

// verifySpeculativeTokens implements the speculative sampling algorithm
// (Leviathan et al., 2023). After the shared batch decode, it retrieves the
// target model's probability distribution at each speculative position and
// compares with the draft model's distribution.
//
// For each draft token x_i with draft probability q(x_i) and target
// probability p(x_i):
//   - Accept with probability min(1, p(x_i) / q(x_i))
//   - On rejection: sample from the adjusted distribution max(0, p - q),
//     normalized. This guarantees the output distribution exactly matches
//     the target model, regardless of draft quality.
//
// Accepted tokens are processed through handleSampledToken for streaming.
// Rejected tokens are rolled back from both target and draft KV caches.
// If all draft tokens are accepted, a bonus token is sampled from the target.
func (e *batchEngine) verifySpeculativeTokens(s *slot, buf []byte) {
	draftTokens := s.specDraftTokens
	draftProbs := s.specDraftProbs
	draftDistsSparse := s.specDraftDistsSparse
	nDraft := len(draftTokens)
	baseBatch := s.specBaseBatch
	basePast := s.specBasePast
	nVocab := int(llama.VocabNTokens(e.model.vocab))
	temperature := s.job.params.Temperature
	greedy := temperature == 0

	// MTP: the MTP draft head currently runs only greedy sampling
	// (generateDraftTokensMTP uses SamplerInitGreedy) and does NOT
	// capture sparse or dense draft distributions. If we entered the
	// probabilistic verify path with no draft distribution, every
	// position would fall through to sampleFromProbs(target) and
	// reject the draft token unconditionally, giving 0% acceptance.
	//
	// Force greedy verification on the MTP path so we instead match
	// the draft's argmax proposal against the target's sampled token
	// at the same position. The greedy branch below is taught to use
	// the slot's full sampler (when mtpGreedy is set) so the user's
	// temperature / top-k / top-p still shape the emitted sequence —
	// this loses the rigorous speculative-sampling distribution
	// guarantee but is the standard approximation when the draft
	// distribution is unavailable.
	mtpGreedy := e.model.draft != nil && e.model.draft.mtp
	if mtpGreedy {
		greedy = true
	}

	// Determine whether to use sparse candidate-based verification.
	useSparse := !greedy && draftDistsSparse != nil

	// Capture context before handleSampledToken may trigger finishSlot → reset,
	// which sets s.job to nil.
	ctx := s.job.ctx

	// Snapshot s.sampled BEFORE the verify loop. handleSampledToken
	// inside the loop mutates s.sampled to each accepted draft token,
	// but the hybrid re-decode path (restoreTargetSpecSnapshot) needs
	// the ORIGINAL token that was at position basePast in the spec
	// batch — otherwise it re-decodes the wrong token there and the
	// subsequent rounds sample from a context that doesn't match what
	// the streaming pipeline actually emitted.
	originalSampled := s.sampled

	// Clear speculative state.
	s.specDraftTokens = nil
	s.specDraftProbs = nil
	s.specDraftDistsSparse = nil

	accepted := 0
	var bonusToken llama.Token

	// Update acceptance rate EMA when verification completes (including
	// early returns on EOG). Deferred so all exit paths are covered.
	defer func() {
		if nDraft > 0 {
			rate := float64(accepted) / float64(nDraft)
			s.specAccEMA = 0.9*s.specAccEMA + 0.1*rate
		}
	}()

	for i := range nDraft {
		draftToken := draftTokens[i]

		// Greedy verification: accept if draft token matches the target's
		// chosen token at this position. For temperature==0 we use
		// argmax (no softmax needed). For the MTP path (which forced
		// greedy because the draft distribution is unavailable) we
		// instead invoke the slot's full sampler so the user's
		// temperature / top-k / top-p still shape the emitted sequence.
		if greedy {
			var targetTok llama.Token
			switch {
			case mtpGreedy:
				switch {
				case s.grammarSampler != nil && s.reasonFlag == 0:
					targetTok = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(i))
				default:
					targetTok = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
				}
			default:
				targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(i), nVocab)
				if err != nil {
					switch {
					case s.grammarSampler != nil && s.reasonFlag == 0:
						targetTok = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(i))
					default:
						targetTok = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
					}
					bonusToken = targetTok
					break
				}
				targetTok = argmax(targetLogits)
			}

			if draftToken == targetTok {
				accepted++
				s.specAcceptedTotal++
				s.nPast = basePast + llama.Pos(1+i)
				e.handleSampledToken(s, draftToken, baseBatch+int32(i), buf)

				if !s.active {
					e.model.log(ctx, "speculative", "status", "verify-done-eog",
						"slot", s.id, "accepted", accepted, "nDraft", nDraft)
					return
				}
				continue
			}

			bonusToken = targetTok
			break
		}

		// Sparse candidate-based probabilistic verification.
		if useSparse {
			// Check if this position has a valid sparse draft distribution.
			// Fall through to full-vocab path if missing or empty.
			if i >= len(draftDistsSparse) || len(draftDistsSparse[i]) == 0 {
				useSparse = false
				goto fullVocab
			}

			qDraft := lookupProb(draftDistsSparse[i], draftToken)
			if qDraft <= 0 {
				// Draft token not in sparse candidates — can't compute
				// acceptance ratio. Fall through to full-vocab for this
				// and all remaining positions.
				useSparse = false
				goto fullVocab
			}

			// Get target probability for the draft token. Read target
			// logits and apply temperature-scaled softmax to obtain the
			// full target distribution. This works regardless of whether
			// the target context has backend samplers.
			targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(i), nVocab)
			if err != nil {
				bonusToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
				break
			}

			draftRef := e.model.draft
			draftRef.sortIndices = applySamplerFilters(targetLogits, draftRef.targetProbs, temperature, s.job.params.TopP, s.job.params.MinP, s.job.params.TopK, draftRef.sortIndices, &draftRef.filterBuf)

			pTarget := draftRef.targetProbs[draftToken]

			// Accept with probability min(1, p_target / q_draft).
			ratio := float64(pTarget) / float64(qDraft)
			if ratio >= 1.0 || rand.Float64() < ratio {
				accepted++
				s.specAcceptedTotal++
				s.nPast = basePast + llama.Pos(1+i)
				e.handleSampledToken(s, draftToken, baseBatch+int32(i), buf)
				if !s.active {
					e.model.log(ctx, "speculative", "status", "verify-done-eog",
						"slot", s.id, "accepted", accepted, "nDraft", nDraft)
					return
				}
				continue
			}

			// Rejected: sample from adjusted distribution using draft
			// sparse candidates against the full target distribution.
			if cap(s.adjustedDistBuf) < len(draftDistsSparse[i]) {
				s.adjustedDistBuf = make([]candidateEntry, 0, len(draftDistsSparse[i]))
			}
			bonusToken = sampleAdjustedSparseFromFull(draftRef.targetProbs, draftDistsSparse[i], s.adjustedDistBuf)
			break
		}

	fullVocab:

		// Full-vocab fallback for non-greedy when sparse distributions are unavailable.
		targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(i), nVocab)
		if err != nil {
			var fallbackToken llama.Token
			switch {
			case s.grammarSampler != nil && s.reasonFlag == 0:
				fallbackToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(i))
			default:
				fallbackToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
			}
			bonusToken = fallbackToken
			break
		}

		draft := e.model.draft
		draft.sortIndices = applySamplerFilters(targetLogits, draft.targetProbs, temperature, s.job.params.TopP, s.job.params.MinP, s.job.params.TopK, draft.sortIndices, &draft.filterBuf)

		pTarget := draft.targetProbs[draftToken]

		// When full draft probabilities are unavailable (sparse mode fell
		// through to full-vocab), we can't compute the adjusted rejection
		// distribution max(0, p-q). Sample from target and stop speculating
		// to preserve the target distribution guarantee.
		if draftProbs == nil {
			bonusToken = sampleFromProbs(draft.targetProbs)
			break
		}

		qDraft := draftProbs[i][draftToken]

		// Accept with probability min(1, p_target / q_draft).
		if qDraft > 0 {
			ratio := float64(pTarget) / float64(qDraft)
			if ratio >= 1.0 || rand.Float64() < ratio {
				accepted++
				s.specAcceptedTotal++
				s.nPast = basePast + llama.Pos(1+i)
				e.handleSampledToken(s, draftToken, baseBatch+int32(i), buf)

				if !s.active {
					e.model.log(ctx, "speculative", "status", "verify-done-eog",
						"slot", s.id, "accepted", accepted, "nDraft", nDraft)
					return
				}
				continue
			}
		}

		// Rejected: sample from adjusted distribution max(0, p_target - q_draft).
		bonusToken = sampleAdjustedInto(draft.targetProbs, draftProbs[i], draft.adjusted)
		break
	}

	// If all draft tokens were accepted, sample bonus from target at position nDraft.
	if accepted == nDraft {
		targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(nDraft), nVocab)
		switch {
		case err != nil:
			switch {
			case s.grammarSampler != nil && s.reasonFlag == 0:
				bonusToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(nDraft))
			default:
				bonusToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(nDraft))
			}

		case greedy:
			bonusToken = argmax(targetLogits)

		default:
			draft := e.model.draft
			draft.sortIndices = applySamplerFilters(targetLogits, draft.targetProbs, temperature, s.job.params.TopP, s.job.params.MinP, s.job.params.TopK, draft.sortIndices, &draft.filterBuf)
			bonusToken = sampleFromProbs(draft.targetProbs)
		}
	}

	// Roll back rejected draft positions from the target KV cache.
	//
	// Hybrid models (transformer + recurrent layers) need a state
	// restore here, NOT a MemorySeqRm: the recurrent layer's per-seq
	// state has been advanced through all 1+nDraft decoded positions
	// and there is no per-position trim. Restore from the pre-spec
	// snapshot taken at batch-add time, then re-decode the
	// (sampled + accepted drafts) prefix so the seq is left at exactly
	// basePast + 1 + accepted positions of correct state.
	//
	// For dense / pure-attention targets the simple MemorySeqRm path
	// is used — much cheaper than a snapshot/restore + re-decode and
	// fully correct because there is no recurrent state to rewind.
	rollbackFrom := basePast + llama.Pos(1+accepted)
	rollbackTo := basePast + llama.Pos(1+nDraft)
	hybridRestore := e.model.modelInfo.Type == ModelTypeHybrid && len(s.specSnapshot) > 0 && rollbackFrom < rollbackTo

	switch {
	case hybridRestore:
		if err := e.restoreTargetSpecSnapshot(s, basePast, originalSampled, draftTokens, accepted); err != nil {
			e.model.log(ctx, "speculative", "status", "restore-error",
				"slot", s.id, "accepted", accepted, "err", err)
			// Fall through to the MemorySeqRm path even though it's
			// broken on hybrid — best-effort, and the next request
			// will start from a fresh sequence anyway because we'll
			// likely fail the slot below.
			e.model.decodeMu.Lock()
			llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
			e.model.decodeMu.Unlock()
		}

	case rollbackFrom < rollbackTo:
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
		e.model.decodeMu.Unlock()
	}

	// Rollback draft KV to match.
	e.rollbackDraft(ctx, s, accepted, nDraft)

	// MTP: mirror the accepted prefix [basePast..basePast+accepted]
	// (1+accepted positions) from the target's just-decoded pre-norm
	// buffer into the draft KV. This OVERWRITES the AR-loop entries
	// the MTP draft wrote during generateDraftTokensMTP with the
	// target-derived hidden states, and updates s.pendingH to
	// h(basePast+accepted) for the next draft round.
	if e.model.draft != nil && e.model.draft.mtp && s.mtpHasBatch && !s.mtpDisabledForRequest {
		if err := e.mirrorTargetBatchToMTPDraft(s, 1+accepted); err != nil {
			e.model.log(ctx, "speculative", "status", "mtp-mirror-error",
				"slot", s.id, "accepted", accepted, "err", err)
			// Fall through: clear mtpHasBatch and let the next iteration
			// start fresh. The slot's draft KV may be in a partially
			// updated state; the next prefill or gen mirror will resync.
			s.mtpHasBatch = false
		}
	}

	// Set nPast after s.sampled + accepted drafts.
	s.nPast = basePast + llama.Pos(1+accepted)

	// Throttle per-round verify-done logging. The final slot-finished
	// line carries the per-request rollup, so steady-state INFO output
	// only needs a periodic summary to show acceptance drift and
	// nPast progression. First round is always logged so the start of
	// each request is anchored; afterwards every 32 rounds.
	s.specRounds++
	if s.specRounds == 1 || s.specRounds%32 == 0 {
		// Compute the EMA value as it will be after the deferred update
		// so the logged trend reflects the round we just completed.
		emaAfter := s.specAccEMA
		if nDraft > 0 {
			emaAfter = 0.9*s.specAccEMA + 0.1*(float64(accepted)/float64(nDraft))
		}
		e.model.log(ctx, "speculative", "status", "verify-done",
			"slot", s.id, "round", s.specRounds, "accepted", accepted, "nDraft", nDraft,
			"target_nPast", s.nPast, "draft_nPast", s.draftNPast,
			"acc_ema", fmt.Sprintf("%.2f", emaAfter))
	}

	// Process the bonus token through the streaming pipeline.
	e.handleSampledToken(s, bonusToken, baseBatch+int32(accepted), buf)

	if !s.active {
		return
	}

	s.iBatch = -1
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
// before the spec batch, plus the accepted prefix re-applied. Failure
// to restore or re-decode leaves the slot in an inconsistent state and
// the caller logs / continues; the slot will be cleared on the next
// finishSlot.
func (e *batchEngine) restoreTargetSpecSnapshot(s *slot, basePast llama.Pos, sampledAtBase llama.Token, draftTokens []llama.Token, accepted int) error {
	e.model.decodeMu.Lock()
	n := llama.StateSeqSetData(e.model.lctx, s.specSnapshot, s.seqID)
	e.model.decodeMu.Unlock()
	if n == 0 {
		return fmt.Errorf("state-seq-set-data returned 0 for seq %d", s.seqID)
	}

	// Re-decode the accepted prefix into the now-rewound seq. The
	// re-batch is small (1 + accepted tokens, capped at nDraft+1)
	// so BatchInit/BatchFree per round is negligible. logits=true
	// only on the LAST position because verifySpeculativeTokens
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

	rebatch.Add(sampledAtBase, basePast, s.seqIDs, accepted == 0)
	for i := range accepted {
		isLast := i == accepted-1
		rebatch.Add(draftTokens[i], basePast+llama.Pos(1+i), s.seqIDs, isLast)
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

// argmax returns the token with the highest logit value.
func argmax(logits []float32) llama.Token {
	if len(logits) == 0 {
		return 0
	}

	maxIdx := 0
	maxVal := logits[0]
	for i := 1; i < len(logits); i++ {
		if logits[i] > maxVal {
			maxVal = logits[i]
			maxIdx = i
		}
	}
	return llama.Token(maxIdx)
}

// sampleAdjustedInto samples from the adjusted distribution max(0, p_target - q_draft),
// normalized, writing into the pre-allocated adjusted buffer. This is the rejection
// branch of speculative sampling, ensuring the output distribution exactly matches
// the target model.
func sampleAdjustedInto(targetProbs, draftProbs, adjusted []float32) llama.Token {
	var sum float64

	for i := range targetProbs {
		diff := float64(targetProbs[i]) - float64(draftProbs[i])
		switch {
		case diff > 0:
			adjusted[i] = float32(diff)
			sum += diff
		default:
			adjusted[i] = 0
		}
	}

	// If the adjusted distribution is empty or invalid (NaN),
	// fall back to sampling from the target distribution directly.
	if !(sum > 0) {
		return sampleFromProbs(targetProbs)
	}

	// Normalize and sample.
	invSum := float32(1.0 / sum)
	for i := range adjusted {
		adjusted[i] *= invSum
	}

	return sampleFromProbs(adjusted)
}

// sampleFromProbs samples a token from a probability distribution using
// inverse transform sampling.
func sampleFromProbs(probs []float32) llama.Token {
	if len(probs) == 0 {
		return 0
	}

	r := rand.Float32()
	var cumulative float32

	last := 0
	for i, p := range probs {
		if p > 0 {
			last = i
		}
		cumulative += p
		if r < cumulative {
			return llama.Token(i)
		}
	}

	// Fallback: return last non-zero token (rounding errors).
	return llama.Token(last)
}

// rollbackDraft removes rejected draft tokens from the draft model's KV cache
// and updates the slot's draft position to stay in sync with the target.
func (e *batchEngine) rollbackDraft(ctx context.Context, s *slot, accepted, nDraft int) {
	draft := e.model.draft
	if draft == nil {
		return
	}

	// MTP: clear the ENTIRE drafted range from draft KV. The post-verify
	// mirror that runs next in verifySpeculativeTokens re-decodes
	// positions [base..base+accepted] from the target's pre-norm buffer,
	// and llama.cpp's transformer KV does NOT overwrite by (seq, pos)
	// when llama_decode is called on a position that already has an
	// entry — it just appends another KV slot, leaving duplicate
	// entries that corrupt subsequent attention.
	//
	// So for MTP we must remove ALL AR-loop entries first, then let the
	// mirror write the correct target-derived entries into clean slots.
	if draft.mtp {
		draftBasePast := s.draftNPast - llama.Pos(nDraft)
		if draftBasePast < s.draftNPast {
			llama.MemorySeqRm(draft.mem, s.seqID, draftBasePast, s.draftNPast)
		}
		s.draftNPast = draftBasePast
		return
	}

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
	draftBasePast := s.draftNPast - llama.Pos(nDraft)
	draftKeep := draftBasePast + llama.Pos(accepted+1)

	// Cap draftKeep at the actual KV end to prevent advancing past decoded content.
	draftKVEnd := s.draftNPast
	if draftKeep > draftKVEnd {
		draftKeep = draftKVEnd
	}

	if draftKeep < draftKVEnd {
		llama.MemorySeqRm(draft.mem, s.seqID, draftKeep, draftKVEnd)
	}

	// Update draft nPast to the next write position after kept tokens.
	s.draftNPast = draftKeep

	e.model.log(ctx, "speculative", "status", "draft-rollback",
		"slot", s.id, "accepted", accepted, "nDraft", nDraft,
		"draft_base", draftBasePast, "draft_keep", draftKeep,
		"draft_kv_end", draftKVEnd, "draft_nPast", s.draftNPast)
}
