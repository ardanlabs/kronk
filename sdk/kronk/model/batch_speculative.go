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
		s.draftPrefillNeeded = false
		e.model.log(ctx, "speculative", "status", "draft-prefill-skip-empty", "slot", s.id)
		return nil
	}

	prefillStart := time.Now()

	// Find common prefix between cached tokens and new prompt.
	commonLen := 0
	cached := draft.cachedTokens
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

	// Decode new suffix tokens into draft model in chunks.
	if len(newTokens) > 0 {
		batchSize := int32(min(nBatch, len(newTokens)))
		if batchSize <= 0 {
			batchSize = 1
		}
		batch := llama.BatchInit(batchSize, 0, 1)
		defer llama.BatchFree(batch)

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
				// On failure, invalidate the cache to avoid stale state.
				draft.cachedTokens = nil
				return fmt.Errorf("draft prefill failed at pos %d: %w", commonLen+i, decodeError(ret, err))
			}
		}
	}

	s.draftNPast = llama.Pos(len(tokens))
	s.draftPromptTokens = nil
	s.draftPrefillNeeded = false

	// Store a copy of the prompt tokens for the next request's prefix comparison.
	draft.cachedTokens = make([]llama.Token, len(tokens))
	copy(draft.cachedTokens, tokens)

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
// the draft model's probability distribution at each step. The distributions
// are stored in s.specDraftProbs for use during verification.
func (e *batchEngine) generateDraftTokens(s *slot) []llama.Token {
	draft := e.model.draft
	nVocab := int(llama.VocabNTokens(e.model.vocab))
	draftTokens := make([]llama.Token, 0, draft.nDraft)

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

		// Capture draft probability distribution into pre-allocated buffer.
		logits, err := llama.GetLogitsIth(draft.lctx, -1, nVocab)
		if err != nil {
			break
		}

		softmaxInto(logits, draft.draftProbs[drafted])

		// Greedy sample from draft model.
		token := llama.SamplerSample(draft.sampler, draft.lctx, -1)

		// Check for end of generation.
		if llama.VocabIsEOG(e.model.vocab, token) {
			break
		}

		draftTokens = append(draftTokens, token)
		drafted++
		lastToken = token
	}

	s.specDraftProbs = draft.draftProbs[:drafted]

	e.model.log(s.job.ctx, "speculative", "status", "draft-generated",
		"slot", s.id, "drafted", len(draftTokens), "target_nDraft", draft.nDraft,
		"draft_nPast_before", draftStartPast, "draft_nPast_after", s.draftNPast)

	return draftTokens
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
	nDraft := len(draftTokens)
	baseBatch := s.specBaseBatch
	basePast := s.specBasePast
	nVocab := int(llama.VocabNTokens(e.model.vocab))

	// Capture context before handleSampledToken may trigger finishSlot → reset,
	// which sets s.job to nil.
	ctx := s.job.ctx

	e.model.log(ctx, "speculative", "status", "verify-start",
		"slot", s.id, "nDraft", nDraft, "basePast", basePast, "baseBatch", baseBatch)

	// Clear speculative state.
	s.specDraftTokens = nil
	s.specDraftProbs = nil

	accepted := 0
	var bonusToken llama.Token

	for i := range nDraft {
		// Get target probability distribution at this position.
		targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(i), nVocab)
		if err != nil {
			// Fallback: reject all remaining draft tokens. Sample from target
			// using the sampler at the first position as the bonus token.
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
		softmaxInto(targetLogits, draft.targetProbs)

		draftToken := draftTokens[i]
		pTarget := draft.targetProbs[draftToken]
		qDraft := draftProbs[i][draftToken]

		// Accept with probability min(1, p_target / q_draft).
		if qDraft > 0 {
			ratio := float64(pTarget) / float64(qDraft)
			if ratio >= 1.0 || rand.Float64() < ratio {
				// Draft token accepted.
				accepted++

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
		draft := e.model.draft
		targetLogits, err := llama.GetLogitsIth(e.model.lctx, baseBatch+int32(nDraft), nVocab)
		if err != nil {
			// Fallback to sampler.
			switch {
			case s.grammarSampler != nil:
				bonusToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(nDraft))
			default:
				bonusToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(nDraft))
			}
		} else {
			softmaxInto(targetLogits, draft.targetProbs)
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

// sampleAdjustedInto samples from the adjusted distribution max(0, p_target - q_draft),
// normalized, writing into the pre-allocated adjusted buffer. This is the rejection
// branch of speculative sampling, ensuring the output distribution exactly matches
// the target model.
func sampleAdjustedInto(targetProbs, draftProbs, adjusted []float32) llama.Token {
	var sum float64

	for i := range targetProbs {
		diff := float64(targetProbs[i]) - float64(draftProbs[i])
		if diff > 0 {
			adjusted[i] = float32(diff)
			sum += diff
		} else {
			adjusted[i] = 0
		}
	}

	// If the adjusted distribution is empty (shouldn't happen in practice),
	// fall back to sampling from the target distribution directly.
	if sum <= 0 {
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
