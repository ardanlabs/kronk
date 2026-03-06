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
		nBatch = e.model.cfg.NBatch
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
		"elapsed", time.Since(prefillStart).String())

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

	// Determine whether to use sparse candidate-based verification.
	useSparse := !greedy && draftDistsSparse != nil

	// Capture context before handleSampledToken may trigger finishSlot → reset,
	// which sets s.job to nil.
	ctx := s.job.ctx

	e.model.log(ctx, "speculative", "status", "verify-start",
		"slot", s.id, "nDraft", nDraft, "basePast", basePast, "baseBatch", baseBatch,
		"temperature", temperature, "useSparse", useSparse)

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

		// Greedy verification: accept if draft token matches target's argmax.
		// No softmax needed — just find the highest logit.
		if greedy {
			targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(i), nVocab)
			if err != nil {
				var fallbackToken llama.Token
				switch {
				case s.grammarSampler != nil:
					fallbackToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(i))
				default:
					fallbackToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
				}
				bonusToken = fallbackToken
				break
			}

			targetArgmax := argmax(targetLogits)
			if draftToken == targetArgmax {
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

			bonusToken = targetArgmax
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
			case s.grammarSampler != nil:
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
			case s.grammarSampler != nil:
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

	// Rollback rejected draft tokens from target KV cache.
	rollbackFrom := basePast + llama.Pos(1+accepted)
	rollbackTo := basePast + llama.Pos(1+nDraft)

	if rollbackFrom < rollbackTo {
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
		e.model.decodeMu.Unlock()
	}

	// Rollback draft KV to match.
	e.rollbackDraft(ctx, s, accepted, nDraft)

	// Set nPast after s.sampled + accepted drafts.
	s.nPast = basePast + llama.Pos(1+accepted)

	e.model.log(ctx, "speculative", "status", "verify-done",
		"slot", s.id, "accepted", accepted, "nDraft", nDraft,
		"target_nPast", s.nPast, "draft_nPast", s.draftNPast)

	// Process the bonus token through the streaming pipeline.
	e.handleSampledToken(s, bonusToken, baseBatch+int32(accepted), buf)

	if !s.active {
		return
	}

	s.iBatch = -1
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
