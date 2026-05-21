package model

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// batchTokensAt aliases the token-id range [start..start+count) of a
// llama.Batch as a Go slice. The returned slice shares memory with the
// underlying C-owned buffer — do not retain past the next batch
// mutation. Returns nil when bounds are out of range or the batch has
// no token buffer (embd-only batch).
func batchTokensAt(b llama.Batch, start, count int) []llama.Token {
	if b.Token == nil || count <= 0 {
		return nil
	}
	all := unsafe.Slice(b.Token, int(b.NTokens))
	if start < 0 || start+count > len(all) {
		return nil
	}
	return all[start : start+count]
}

// MTP (Multi-Token Prediction) speculative decoding implementation.
//
// Reference: common/speculative.cpp common_speculative_impl_draft_mtp in
// upstream llama.cpp. The two distinguishing requirements vs a normal
// (separate-GGUF) draft are:
//
//  1. The MTP draft head reads PRE-NORM hidden states alongside token
//     ids. Every target llama_decode emits a dense pre-norm buffer
//     (because we called SetEmbeddingsPreNorm(target, true, false) at
//     load) which must be mirrored into the draft context via a decode
//     with batch.token + batch.embd populated. The embd rows are SHIFTED
//     RIGHT BY ONE so slot 0 of each mirror batch carries the hidden
//     state from the previous decode's last position (per-slot
//     "pending_h") and slot k>0 carries h_tgt[k-1] from the current
//     decode. After the mirror, pending_h is updated to the last row of
//     the current decode.
//
//  2. The AR draft loop decodes the draft context with both the
//     just-sampled token id AND the hidden state read back from the
//     PREVIOUS draft decode (via GetEmbeddingsPreNormIth). This couples
//     each draft step to the MTP head's continuation prediction.
//
// All MTP paths live in this file. Non-MTP speculative decoding stays
// in batch_speculative.go and continues to use llama.DraftGenerate.

// mirrorTargetBatchToMTPDraft replays the just-decoded target batch
// range for slot s into the draft context, with batch.embd populated
// from the target's pre-norm hidden-state buffer (shift-right-by-1
// alignment per common_speculative_impl_draft_mtp).
//
// PRECONDITIONS
//   - llama.Decode(target, batch) has just succeeded.
//   - llama.Synchronize(target) has been called (so the pre-norm buffer
//     is populated and stable to read).
//   - s.targetBatchStart / s.targetBatchCount / s.targetBatchBasePos
//     describe the slot's contiguous range in the just-decoded target
//     batch (set during batch assembly).
//   - The caller passes `effectiveCount` which is the number of
//     positions whose KV survived rollback. For prefill and gen this
//     equals s.targetBatchCount; for spec verify it equals 1 + accepted.
//
// On success the draft KV holds positions
// [s.targetBatchBasePos .. s.targetBatchBasePos + effectiveCount), the
// slot's draftNPast is advanced to that end, and s.pendingH is updated
// to the hidden row of the last surviving position. On failure the
// draft KV may be partially advanced; the caller should fail the slot.
func (e *batchEngine) mirrorTargetBatchToMTPDraft(s *slot, effectiveCount int) error {
	draft := e.model.draft
	if draft == nil || !draft.mtp {
		s.mtpHasBatch = false
		return nil
	}
	if effectiveCount <= 0 || !s.mtpHasBatch {
		s.mtpHasBatch = false
		return nil
	}

	nEmbd := draft.nEmbd
	mirror := draft.mirrorBatchMTP

	// Choose pre-norm source. Phase A of speculative verify captures
	// the slot's pre-norm rows into s.verifyH BEFORE any Phase B side-
	// effect (notably restoreTargetSpecSnapshot on hybrid targets)
	// re-decodes on the target context and overwrites the per-context
	// pre-norm buffer. When verifyH is populated we read from it; the
	// rows are already sliced to [s.targetBatchStart .. start+1+nDraft).
	//
	// For non-spec paths (prefill/gen mirror in Pass 1), verifyH is
	// empty and we read the live target pre-norm buffer indexed by
	// raw target-batch position (SetEmbeddingsPreNorm was called with
	// masked=false at load).
	//
	// preNormRow returns the pre-norm row for slot-relative index k
	// (0 ≤ k < effectiveCount).
	start := int(s.targetBatchStart)
	var preNormRow func(k int) []float32

	switch {
	case len(s.verifyH) >= effectiveCount*nEmbd:
		src := s.verifyH
		preNormRow = func(k int) []float32 {
			return src[k*nEmbd : (k+1)*nEmbd]
		}

	default:
		totalRows := int(e.batch.NTokens)
		embd := GetEmbeddingsPreNorm(e.model.lctx, totalRows, nEmbd)
		if embd == nil {
			s.mtpHasBatch = false
			return fmt.Errorf("mtp-mirror: target pre-norm buffer is nil (SetEmbeddingsPreNorm may not have been enabled)")
		}
		if start < 0 || start+effectiveCount > totalRows {
			s.mtpHasBatch = false
			return fmt.Errorf("mtp-mirror: slot batch range [%d..%d) out of target batch (size %d)",
				start, start+effectiveCount, totalRows)
		}
		preNormRow = func(k int) []float32 {
			absRow := start + k
			return embd[absRow*nEmbd : (absRow+1)*nEmbd]
		}
	}

	// The mirror batch must hold (token id, embd row) for each position.
	// Token ids come from the target batch slice; embd rows come from
	// the pre-norm buffer, SHIFTED RIGHT BY 1:
	//
	//   mirror[0]      : token = tgt[start+0],       embd = pendingH (slot's pre-batch h)
	//   mirror[k>0]    : token = tgt[start+k],       embd = embd_tgt[start+k-1]
	//
	// The first mirror row's pendingH is empty for the very first
	// decode in a sequence (no h has been observed yet). We zero that
	// slot — the MTP head's first prediction at position 0 is on a
	// BOS / instruction sentinel where exact h doesn't matter.
	//
	// Token ids: we need to read them out of the target batch. We can't
	// easily slice a C pointer here, so we use unsafe via batch fields.
	tgtTokens := batchTokensAt(e.batch, start, effectiveCount)
	if tgtTokens == nil {
		s.mtpHasBatch = false
		return fmt.Errorf("mtp-mirror: failed to alias target batch tokens at [%d..%d)", start, start+effectiveCount)
	}

	mirror.NTokens = 0
	seqIDs := s.seqIDs

	// Decode the mirror in chunks of cap(mirror) (which is draft.mirrorBatchMTP's
	// allocated capacity == NBatch).
	maxPer := int(draft.mirrorBatchCapacity())
	if maxPer <= 0 {
		s.mtpHasBatch = false
		return fmt.Errorf("mtp-mirror: mirror batch has zero capacity")
	}

	for chunkStart := 0; chunkStart < effectiveCount; chunkStart += maxPer {
		chunkEnd := min(chunkStart+maxPer, effectiveCount)
		chunkLen := chunkEnd - chunkStart

		mirror.NTokens = 0

		// Build the chunk: for each position k in [chunkStart..chunkEnd),
		// add token tgtTokens[k] at position basePos+k, then write the
		// shifted-right-by-1 embd row at slot k of mirror.Embd.
		for k := range chunkLen {
			pos := s.targetBatchBasePos + llama.Pos(chunkStart+k)
			// logits flag: we don't need draft logits for mirror rows;
			// only the LAST mirror across the whole effective range
			// produces the pre-norm row we want as the next pending_h
			// (the masked draft ctx only stores logits-flagged rows in
			// its pre-norm buffer). Set logits=true on the very last
			// row to guarantee pending_h is readable, and false elsewhere.
			isLast := (chunkStart+k == effectiveCount-1)
			mirror.Add(tgtTokens[chunkStart+k], pos, seqIDs, isLast)

			// Write the embd row for this mirror slot.
			dst := draft.mirrorEmbdSlice[k*nEmbd : (k+1)*nEmbd]
			srcGlobal := chunkStart + k - 1 // slot-relative index of previous position
			switch {
			case srcGlobal < 0:
				// Slot 0 of the very first chunk: use s.pendingH if we
				// have it from a previous decode, else zero.
				if len(s.pendingH) == nEmbd {
					copy(dst, s.pendingH)
				} else {
					for i := range dst {
						dst[i] = 0
					}
				}
			default:
				// Use h from the slot's pre-norm row at index srcGlobal
				// (the row of the previous position in the slot's
				// captured window).
				copy(dst, preNormRow(srcGlobal))
			}
		}

		ret, err := llama.Decode(draft.lctx, mirror)
		if err != nil || ret != 0 {
			s.mtpHasBatch = false
			return fmt.Errorf("mtp-mirror: draft decode failed at chunk [%d..%d): %w",
				chunkStart, chunkEnd, decodeError(ret, err))
		}

		// Synchronize INSIDE the loop, BEFORE the next chunk overwrites
		// mirror.Embd. mirrorEmbdSlice aliases C-owned mirrorBatchMTP.Embd
		// which the draft decode reads on async backends (Metal/CUDA);
		// without a sync the next chunk's per-row copy() into the slice
		// races the in-flight read and corrupts the embd input. The
		// single post-loop sync was insufficient when effectiveCount
		// exceeded mirror capacity (NBatch).
		llama.Synchronize(draft.lctx)
	}

	// Advance draft nPast and update pendingH to the last position's
	// target hidden state (so the NEXT mirror or draft step sees it as
	// "the previous position's h").
	s.draftNPast = s.targetBatchBasePos + llama.Pos(effectiveCount)
	if cap(s.pendingH) < nEmbd {
		s.pendingH = make([]float32, nEmbd)
	} else {
		s.pendingH = s.pendingH[:nEmbd]
	}
	copy(s.pendingH, preNormRow(effectiveCount-1))

	// verifyH (if it was the source) has been fully consumed. Reset
	// length so the next slot.reset() / Phase A capture starts clean;
	// retain cap for the next request on this slot.
	s.verifyH = s.verifyH[:0]

	s.mtpHasBatch = false
	return nil
}

// generateDraftTokensMTP is the MTP analogue of generateDraftTokens. It
// runs an auto-regressive draft loop on the MTP draft context, feeding
// each step (token id, pre-norm hidden state) and reading back the next
// pre-norm row via GetEmbeddingsPreNormIth.
//
// PRECONDITIONS
//   - The most recent target decode for this slot has been mirrored
//     into the draft context (mirrorTargetBatchToMTPDraft).
//   - s.pendingH holds the pre-norm hidden state of the just-sampled
//     token (s.sampled at position s.nPast-1) from the target side.
//   - s.draftNPast == s.nPast (mirror left them in sync).
//
// Returns the generated draft tokens (also stored in s.draftTokensBuf
// per existing convention).
func (e *batchEngine) generateDraftTokensMTP(s *slot) []llama.Token {
	draft := e.model.draft
	nEmbd := draft.nEmbd

	nDraft := chooseNDraft(s, draft.nDraft)
	if nDraft == 0 {
		s.draftTokensBuf = s.draftTokensBuf[:0]
		return s.draftTokensBuf
	}

	if cap(s.draftTokensBuf) < nDraft {
		s.draftTokensBuf = make([]llama.Token, nDraft)
	}
	s.draftTokensBuf = s.draftTokensBuf[:0]

	// We expect a populated pendingH because the mirror step that ran
	// after the last target decode wrote it. If it isn't sized, we
	// can't safely run MTP for this round.
	if len(s.pendingH) != nEmbd {
		return s.draftTokensBuf
	}

	// Greedy sampler for MTP. Non-greedy MTP requires the same per-slot
	// sampler-rebuild dance as separate-GGUF drafts; for the initial
	// MTP delivery we keep it greedy to match the reference impl's
	// hot path and to keep the verification side simple.
	sampler := draft.sampler

	batch := draft.draftBatchMTP
	seqIDs := s.seqIDs
	curToken := s.sampled
	curEmbd := s.pendingH
	pos := s.draftNPast

	for range nDraft {
		batch.NTokens = 0
		batch.Add(curToken, pos, seqIDs, true)

		// Write the embd row for this single-token batch. Slot 0 of
		// draftBatchMTP.Embd is the only row, and Embd was pinned at
		// loadDraftModelMTP to point at draftEmbdSlice — we copy into
		// the pinned slice directly to avoid synthesizing a Go slice
		// from the C pointer every iteration.
		copy(draft.draftEmbdSlice, curEmbd)

		ret, err := llama.Decode(draft.lctx, batch)
		if err != nil || ret != 0 {
			break
		}
		// Synchronize before reading logits / pre-norm rows. On async
		// backends (Metal, CUDA) the decode may still be in-flight; the
		// pre-norm and logit buffers aren't safe to read until the
		// device has finished.
		llama.Synchronize(draft.lctx)
		pos++

		// Sample the next draft token from the MTP head.
		nextTok := llama.SamplerSample(sampler, draft.lctx, -1)

		// Read back the next pre-norm hidden row for the next draft
		// step. The draft ctx was created masked=true, so we index by
		// the row's position in the output table; with a single-token
		// batch and logits=true, that's index 0.
		nextEmbd := GetEmbeddingsPreNormIth(draft.lctx, 0, nEmbd)
		if nextEmbd == nil {
			s.draftTokensBuf = append(s.draftTokensBuf, nextTok)
			break
		}

		// Copy out because the C buffer is overwritten on the next
		// decode.
		if cap(s.pendingH) < nEmbd {
			s.pendingH = make([]float32, nEmbd)
		} else {
			s.pendingH = s.pendingH[:nEmbd]
		}
		copy(s.pendingH, nextEmbd)

		// EOG check — stop drafting past end of generation.
		if llama.VocabIsEOG(e.model.vocab, nextTok) {
			s.draftTokensBuf = append(s.draftTokensBuf, nextTok)
			break
		}

		s.draftTokensBuf = append(s.draftTokensBuf, nextTok)
		curToken = nextTok
		curEmbd = s.pendingH
	}

	s.draftNPast = pos
	s.specDraftedTotal += len(s.draftTokensBuf)
	return s.draftTokensBuf
}

// mirrorBatchCapacity returns the per-call capacity of the MTP mirror
// batch. We stored it implicitly by allocating mirrorBatchMTP with
// (NBatch, nEmbd, 1) but llama.Batch doesn't expose the original
// capacity, so we derive it from the size of the alias slice.
func (d *draftModel) mirrorBatchCapacity() int32 {
	if d.nEmbd <= 0 {
		return 0
	}
	return int32(len(d.mirrorEmbdSlice) / d.nEmbd)
}

// decodeTokensIntoCacheMTP is the MTP-aware analogue of
// Model.decodeTokensIntoCache. It decodes a tokens slice into the
// target seq's KV (just like the plain version) AND mirrors each
// just-decoded chunk into the MTP draft seq's KV via
// mirrorBuildChunkToMTPDraft. After this call returns, both the
// target and draft KV caches hold the cached prefix at positions
// [startPos .. startPos+len(tokens)), and s.pendingH carries the
// pre-norm hidden row of the last cached position so the next MTP
// draft round (or the post-snapshot restore on a later request) can
// condition correctly on the prefix.
//
// Holds e.model.decodeMu for the entire loop. Other speculative /
// processBatch paths already serialize draft access via the single
// processBatch loop, so holding decodeMu here is sufficient.
func (e *batchEngine) decodeTokensIntoCacheMTP(ctx context.Context, s *slot, tokens []llama.Token, startPos int) error {
	nBatch := int(e.model.ctxParams.NBatch)
	if nBatch <= 0 {
		nBatch = e.model.cfg.NBatch()
	}

	nTokens := len(tokens)
	if nTokens == 0 {
		return nil
	}

	e.model.log(ctx, "cache", "status", "decoding tokens into cache (mtp-mirror)",
		"seq", s.seqID, "tokens", nTokens, "start_pos", startPos, "nbatch", nBatch)

	batchSize := int32(min(nBatch, nTokens))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{s.seqID}

	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()

	for i := 0; i < nTokens; i += nBatch {
		end := min(i+nBatch, nTokens)
		batch.Clear()
		for j := i; j < end; j++ {
			pos := llama.Pos(startPos + j)
			batch.Add(tokens[j], pos, seqIDs, false)
		}

		if _, err := llama.Decode(e.model.lctx, batch); err != nil {
			return fmt.Errorf("imc-mtp: target decode at pos %d: %w", startPos+i, err)
		}
		llama.Synchronize(e.model.lctx)

		// Mirror the just-decoded chunk into the MTP draft seq KV.
		// Failure here means the draft seq is partially-populated; the
		// caller treats that as a build failure and clears both seqs.
		if err := e.mirrorBuildChunkToMTPDraft(s, tokens[i:end], llama.Pos(startPos+i)); err != nil {
			return fmt.Errorf("imc-mtp: mirror at pos %d: %w", startPos+i, err)
		}
	}

	e.model.log(ctx, "cache", "status", "finished (decoding tokens into cache (mtp-mirror))",
		"seq", s.seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// mirrorBuildChunkToMTPDraft mirrors a freshly-decoded standalone
// target batch (NOT e.batch) into the MTP draft seq KV. Used by the
// IMC cache-build path where the target decode happens outside of
// processBatch with its own local batch.
//
// PRECONDITIONS
//   - llama.Decode(target, <local batch with these tokens>) has just
//     succeeded and llama.Synchronize(target) has been called.
//   - len(tokens) matches the row count of the target's just-decoded
//     pre-norm buffer (rows are read at indices [0..len(tokens))).
//   - basePos is the absolute sequence position of tokens[0].
//
// On success the draft seq KV holds positions
// [basePos .. basePos+len(tokens)), s.draftNPast is advanced to that
// end, and s.pendingH is updated to the pre-norm row of the last
// decoded position. On failure the draft seq may be partially
// advanced; the caller should treat the build as failed and clear
// both seqs.
func (e *batchEngine) mirrorBuildChunkToMTPDraft(s *slot, tokens []llama.Token, basePos llama.Pos) error {
	draft := e.model.draft
	if draft == nil || !draft.mtp {
		return nil
	}

	nTokens := len(tokens)
	if nTokens == 0 {
		return nil
	}

	nEmbd := draft.nEmbd
	mirror := draft.mirrorBatchMTP

	embd := GetEmbeddingsPreNorm(e.model.lctx, nTokens, nEmbd)
	if embd == nil {
		return fmt.Errorf("mtp-build-mirror: target pre-norm buffer is nil (SetEmbeddingsPreNorm may not have been enabled)")
	}

	maxPer := int(draft.mirrorBatchCapacity())
	if maxPer <= 0 {
		return fmt.Errorf("mtp-build-mirror: mirror batch has zero capacity")
	}

	seqIDs := s.seqIDs

	for chunkStart := 0; chunkStart < nTokens; chunkStart += maxPer {
		chunkEnd := min(chunkStart+maxPer, nTokens)
		chunkLen := chunkEnd - chunkStart

		mirror.NTokens = 0

		for k := range chunkLen {
			pos := basePos + llama.Pos(chunkStart+k)
			// Only mark the very last row (across the whole effective
			// range) with logits=true so the masked draft ctx stores
			// its pre-norm row for the final position — that row is
			// what we read into s.pendingH below as the carry-over
			// for the next decode.
			isLast := chunkStart+k == nTokens-1
			mirror.Add(tokens[chunkStart+k], pos, seqIDs, isLast)

			// Shift-right-by-1 embd alignment per the reference MTP
			// impl: slot 0 of the very first chunk uses s.pendingH
			// (or zero if empty); subsequent slots use the previous
			// position's pre-norm row from the just-decoded target.
			dst := draft.mirrorEmbdSlice[k*nEmbd : (k+1)*nEmbd]
			srcGlobal := chunkStart + k - 1
			switch {
			case srcGlobal < 0:
				if len(s.pendingH) == nEmbd {
					copy(dst, s.pendingH)
				} else {
					for i := range dst {
						dst[i] = 0
					}
				}
			default:
				copy(dst, embd[srcGlobal*nEmbd:(srcGlobal+1)*nEmbd])
			}
		}

		ret, err := llama.Decode(draft.lctx, mirror)
		if err != nil || ret != 0 {
			return fmt.Errorf("mtp-build-mirror: draft decode at chunk [%d..%d): %w",
				chunkStart, chunkEnd, decodeError(ret, err))
		}

		// Synchronize inside the loop so the next chunk's per-row
		// copy() into mirror.Embd does not race the in-flight decode
		// on async backends (Metal/CUDA).
		llama.Synchronize(draft.lctx)
	}

	// Update pendingH to the last row of the just-decoded target batch
	// (so the NEXT mirror or draft step sees it as "the previous
	// position's h"), and advance draft nPast to end-of-chunk.
	if cap(s.pendingH) < nEmbd {
		s.pendingH = make([]float32, nEmbd)
	} else {
		s.pendingH = s.pendingH[:nEmbd]
	}
	copy(s.pendingH, embd[(nTokens-1)*nEmbd:nTokens*nEmbd])
	s.draftNPast = basePos + llama.Pos(nTokens)

	return nil
}
