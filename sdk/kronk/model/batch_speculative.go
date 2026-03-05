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

// generateDraftTokens auto-regressively generates candidate tokens using the
// draft model. Each token requires a separate decode call on the draft context,
// but these are fast because the draft model is small.
//
// For proper speculative sampling (Leviathan et al., 2023), this also captures
// the draft model's probability distribution at each step. Non-greedy mode uses
// sparse candidate-based probability capture instead of full-vocab softmax.
// The sparse distributions are stored in s.specDraftDistsSparse for verification.
func (e *batchEngine) generateDraftTokens(s *slot) []llama.Token {
	draft := e.model.draft
	nVocab := int(llama.VocabNTokens(e.model.vocab))
	draftTokens := draft.draftBuf[:0]
	temperature := s.job.params.Temperature
	greedy := temperature == 0

	// Create or reuse the per-slot draft sampler for non-greedy mode.
	// Reset on each call so rejected token history from the previous
	// speculative round doesn't accumulate and skew proposals.
	if !greedy {
		if s.draftSampler == 0 {
			s.draftSampler = buildDraftSampler(s.job.params)
		} else {
			llama.SamplerReset(s.draftSampler)
		}
	}

	lastToken := s.sampled
	draftStartPast := s.draftNPast
	drafted := 0

	for range draft.nDraft {
		// Feed last token to draft model.
		draft.batch.Clear()
		draft.batch.Add(lastToken, s.draftNPast, s.seqIDs, true)

		ret, err := llama.Decode(draft.lctx, draft.batch)
		if err != nil || ret != 0 {
			break
		}

		s.draftNPast++

		// Sample from draft model. Non-greedy uses the per-slot sampler and
		// captures sparse candidate probabilities. Greedy uses the shared
		// draft sampler (argmax) and skips probability capture entirely.
		var token llama.Token
		if !greedy {
			token = llama.SamplerSample(s.draftSampler, draft.lctx, -1)

			// Lazy init sparse dist buffer.
			if s.draftDistBuf == nil {
				s.draftDistBuf = make([][]candidateEntry, draft.nDraft)
				for i := range s.draftDistBuf {
					s.draftDistBuf[i] = make([]candidateEntry, 0, 128)
				}
			}

			// Always clear the buffer for this position to prevent stale data
			// from a prior speculative step being reused if cnt==0.
			s.draftDistBuf[drafted] = s.draftDistBuf[drafted][:0]

			cnt, _ := llama.GetSampledCandidatesCountIth(draft.lctx, -1)
			if cnt > 0 {
				cands, _ := llama.GetSampledCandidatesIth(draft.lctx, -1, nVocab)
				probs, _ := llama.GetSampledProbsIth(draft.lctx, -1, nVocab)

				// Copy C-backed views into slot-owned buffer immediately
				// before the next sampler call can overwrite them.
				buf := s.draftDistBuf[drafted]
				for j := range cnt {
					buf = append(buf, candidateEntry{tok: cands[j], prob: probs[j]})
				}
				s.draftDistBuf[drafted] = buf
			}

			llama.SamplerAccept(s.draftSampler, token)
		} else {
			token = llama.SamplerSample(draft.sampler, draft.lctx, -1)
		}

		// Check for end of generation.
		if llama.VocabIsEOG(e.model.vocab, token) {
			break
		}

		draftTokens = append(draftTokens, token)
		drafted++
		lastToken = token
	}

	// Copy draft tokens into slot-owned buffer to avoid shared buffer
	// corruption when multiple slots draft in the same processBatch.
	if cap(s.draftTokensBuf) >= len(draftTokens) {
		s.draftTokensBuf = s.draftTokensBuf[:len(draftTokens)]
	} else {
		s.draftTokensBuf = make([]llama.Token, len(draftTokens))
	}
	copy(s.draftTokensBuf, draftTokens)

	if greedy {
		s.specDraftProbs = nil
		s.specDraftDistsSparse = nil
	} else {
		s.specDraftProbs = nil
		s.specDraftDistsSparse = s.draftDistBuf[:drafted]
	}

	s.specDraftedTotal += drafted

	e.model.log(s.job.ctx, "speculative", "status", "draft-generated",
		"slot", s.id, "drafted", len(s.draftTokensBuf), "target_nDraft", draft.nDraft,
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

	// Clone the target sampler for sparse verification so we can track
	// accepted tokens without mutating the slot's sampler state.
	var verifySampler llama.Sampler
	if useSparse {
		verifySampler = llama.SamplerClone(s.sampler)
		defer func() {
			if verifySampler != 0 {
				llama.SamplerFree(verifySampler)
			}
		}()
	}

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

			llama.SamplerSample(verifySampler, e.model.lctx, baseBatch+int32(i))

			cnt, _ := llama.GetSampledCandidatesCountIth(e.model.lctx, baseBatch+int32(i))
			if cnt == 0 {
				// Fallback: can't get target candidates, reject remaining.
				bonusToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
				break
			}

			cands, _ := llama.GetSampledCandidatesIth(e.model.lctx, baseBatch+int32(i), nVocab)
			probs, _ := llama.GetSampledProbsIth(e.model.lctx, baseBatch+int32(i), nVocab)

			// Copy C-backed views into scratch buffer immediately.
			if cap(s.targetDistBuf) < int(cnt) {
				s.targetDistBuf = make([]candidateEntry, 0, cnt)
			}
			targetEntries := s.targetDistBuf[:0]
			for j := range cnt {
				targetEntries = append(targetEntries, candidateEntry{tok: cands[j], prob: probs[j]})
			}
			s.targetDistBuf = targetEntries

			pTarget := lookupProb(targetEntries, draftToken)
			if pTarget <= 0 {
				// Draft token not in target top-K — fall through to
				// full-vocab to avoid forced rejection bias.
				useSparse = false
				goto fullVocab
			}

			// Accept with probability min(1, p_target / q_draft).
			ratio := float64(pTarget) / float64(qDraft)
			if ratio >= 1.0 || rand.Float64() < ratio {
				accepted++
				s.specAcceptedTotal++
				llama.SamplerAccept(verifySampler, draftToken)
				s.nPast = basePast + llama.Pos(1+i)
				e.handleSampledToken(s, draftToken, baseBatch+int32(i), buf)
				if !s.active {
					e.model.log(ctx, "speculative", "status", "verify-done-eog",
						"slot", s.id, "accepted", accepted, "nDraft", nDraft)
					return
				}
				continue
			}

			// Rejected: sample from adjusted distribution using sparse candidates.
			if cap(s.adjustedDistBuf) < len(targetEntries) {
				s.adjustedDistBuf = make([]candidateEntry, 0, len(targetEntries))
			}
			bonusToken = sampleAdjustedSparse(targetEntries, draftDistsSparse[i], s.adjustedDistBuf)
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
		softmaxTempInto(targetLogits, draft.targetProbs, temperature)

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
		switch {
		case useSparse:
			bonusToken = llama.SamplerSample(verifySampler, e.model.lctx, baseBatch+int32(nDraft))

		default:
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
				softmaxTempInto(targetLogits, draft.targetProbs, temperature)
				bonusToken = sampleFromProbs(draft.targetProbs)
			}
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

	for i, p := range probs {
		cumulative += p
		if r < cumulative {
			return llama.Token(i)
		}
	}

	// Fallback: return last token (rounding errors).
	return llama.Token(len(probs) - 1)
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
