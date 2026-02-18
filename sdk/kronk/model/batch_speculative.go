package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// prefillDraft decodes all prompt tokens into the draft model's KV cache.
// Called once after the target model's prefill completes. The draft model
// must process the same prompt as the target to produce meaningful draft
// candidates during generation.
func (e *batchEngine) prefillDraft(ctx context.Context, s *slot) error {
	draft := e.model.draft
	tokens := s.draftPromptTokens

	if len(tokens) == 0 {
		return nil
	}

	e.model.log(ctx, "speculative", "status", "draft-prefill-start",
		"slot", s.id, "tokens", len(tokens))

	nBatch := int(e.model.ctxParams.NBatch)
	if nBatch <= 0 {
		nBatch = e.model.cfg.NBatch
	}

	// Clear draft KV for this sequence.
	llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)

	// Decode prompt tokens into draft model in chunks.
	batchSize := int32(min(nBatch, len(tokens)))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{s.seqID}

	for i := 0; i < len(tokens); i += nBatch {
		batch.Clear()
		end := min(i+nBatch, len(tokens))

		for j := i; j < end; j++ {
			isLast := j == len(tokens)-1
			batch.Add(tokens[j], llama.Pos(j), seqIDs, isLast)
		}

		ret, err := llama.Decode(draft.lctx, batch)
		if err != nil || ret != 0 {
			return fmt.Errorf("draft prefill failed at pos %d: %w", i, decodeError(ret, err))
		}
	}

	s.draftNPast = llama.Pos(len(tokens))
	s.draftPromptTokens = nil
	s.draftPrefillNeeded = false

	e.model.log(ctx, "speculative", "status", "draft-prefill-done",
		"slot", s.id, "draft_nPast", s.draftNPast)

	return nil
}

// generateDraftTokens auto-regressively generates candidate tokens using the
// draft model. Each token requires a separate decode call on the draft context,
// but these are fast because the draft model is small.
func (e *batchEngine) generateDraftTokens(s *slot) []llama.Token {
	draft := e.model.draft
	draftTokens := make([]llama.Token, 0, draft.nDraft)

	lastToken := s.sampled

	for range draft.nDraft {
		// Feed last token to draft model.
		draft.batch.Clear()
		draft.batch.Add(lastToken, s.draftNPast, s.seqIDs, true)

		ret, err := llama.Decode(draft.lctx, draft.batch)
		if err != nil || ret != 0 {
			break
		}

		s.draftNPast++

		// Greedy sample from draft model.
		token := llama.SamplerSample(draft.sampler, draft.lctx, -1)

		// Check for end of generation.
		if llama.VocabIsEOG(e.model.vocab, token) {
			break
		}

		draftTokens = append(draftTokens, token)
		lastToken = token
	}

	return draftTokens
}

// verifySpeculativeTokens verifies draft tokens against the target model's
// predictions. After the shared batch decode, it samples from the target's
// logits at each speculative position and compares with the draft tokens.
//
// Accepted tokens are processed through handleSampledToken for streaming.
// Rejected tokens are rolled back from both target and draft KV caches.
// The target's prediction at the rejection point becomes the bonus token.
func (e *batchEngine) verifySpeculativeTokens(s *slot, buf []byte) {
	draftTokens := s.specDraftTokens
	nDraft := len(draftTokens)
	baseBatch := s.specBaseBatch
	basePast := s.specBasePast

	// Clear speculative state.
	s.specDraftTokens = nil

	accepted := 0
	var bonusToken llama.Token

	for i := range nDraft + 1 {
		// Sample what the target model predicts at this position.
		var targetToken llama.Token
		switch {
		case s.grammarSampler != nil:
			targetToken = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, baseBatch+int32(i))
		default:
			targetToken = llama.SamplerSample(s.sampler, e.model.lctx, baseBatch+int32(i))
		}

		if i == nDraft {
			// All draft tokens accepted. Target's prediction is the bonus.
			bonusToken = targetToken
			break
		}

		if targetToken != draftTokens[i] {
			// Mismatch: target's prediction is the bonus token.
			bonusToken = targetToken
			break
		}

		// Draft token accepted. Process through streaming pipeline.
		accepted++

		// Update nPast to include this accepted token.
		// Position in KV: basePast (s.sampled) + i + 1 (this draft token).
		s.nPast = basePast + llama.Pos(1+i)

		e.handleSampledToken(s, draftTokens[i], baseBatch+int32(i), buf)

		if !s.active {
			// Slot finished (EOG or maxTokens). Clean up remaining
			// speculative tokens from target KV cache.
			rollbackFrom := basePast + llama.Pos(1+accepted)
			rollbackTo := basePast + llama.Pos(1+nDraft)
			if rollbackFrom < rollbackTo {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
				e.model.decodeMu.Unlock()
			}

			// Clean up draft KV.
			e.rollbackDraft(s, accepted, nDraft)

			return
		}
	}

	// Rollback rejected draft tokens from target KV cache.
	// Keep: basePast through basePast + accepted (s.sampled + accepted drafts).
	// Remove: basePast + 1 + accepted through basePast + 1 + nDraft.
	rollbackFrom := basePast + llama.Pos(1+accepted)
	rollbackTo := basePast + llama.Pos(1+nDraft)

	if rollbackFrom < rollbackTo {
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, rollbackFrom, rollbackTo)
		e.model.decodeMu.Unlock()
	}

	// Rollback draft KV to match.
	e.rollbackDraft(s, accepted, nDraft)

	// Set nPast after s.sampled + accepted drafts.
	s.nPast = basePast + llama.Pos(1+accepted)

	// Process the bonus token through the streaming pipeline.
	// The bonus was sampled from logits at baseBatch + accepted, which due
	// to causal attention only depends on tokens at positions ≤ basePast + accepted.
	// So the logits are valid even though rejected drafts exist at later positions.
	e.handleSampledToken(s, bonusToken, baseBatch+int32(accepted), buf)

	if !s.active {
		return
	}

	// The bonus token is now s.sampled (set by handleSampledToken).
	// It will be added to the batch on the next iteration, where its
	// KV entry is created by the target model decode.
	s.iBatch = -1
}

// rollbackDraft removes rejected draft tokens from the draft model's KV cache
// and updates the slot's draft position to stay in sync with the target.
func (e *batchEngine) rollbackDraft(s *slot, accepted, nDraft int) {
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
	// After drafting: s.draftNPast = draftBasePast + nDraft
	//
	// We want to keep: s.sampled + accepted drafts (positions draftBasePast..draftBasePast+accepted).
	// Remove: positions draftBasePast+accepted+1 through draftBasePast+nDraft-1.
	draftBasePast := s.draftNPast - llama.Pos(nDraft)
	draftKeep := draftBasePast + llama.Pos(accepted+1)
	draftEnd := s.draftNPast

	if draftKeep < draftEnd {
		llama.MemorySeqRm(draft.mem, s.seqID, draftKeep, draftEnd)
	}

	// Update draft nPast to the next write position after accepted tokens.
	s.draftNPast = draftKeep
}
