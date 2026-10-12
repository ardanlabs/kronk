package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func addMTPBatchEntry(batch llama.BatchExt, token llama.Token, position llama.Pos, seqIDs []llama.SeqId, hidden []float32, nEmbd int, outputLogits bool) error {
	if len(seqIDs) == 0 {
		return errors.New("adding MTP batch entry: no sequence ID")
	}

	idx, err := llama.BatchExtAddToken(batch, seqIDs[0], token)
	if err != nil {
		return fmt.Errorf("adding token: %w", err)
	}
	for _, seqID := range seqIDs[1:] {
		if err := llama.BatchExtAddSeq(batch, idx, seqID); err != nil {
			return fmt.Errorf("adding sequence %d: %w", seqID, err)
		}
	}
	if err := llama.BatchExtSetEmbdState(batch, idx, hidden, nEmbd); err != nil {
		return fmt.Errorf("setting MTP state embedding: %w", err)
	}
	if err := llama.BatchExtSetPos(batch, idx, position); err != nil {
		return fmt.Errorf("setting position %d: %w", position, err)
	}
	if outputLogits {
		if err := llama.BatchExtSetOutputLogits(batch, idx, true); err != nil {
			return fmt.Errorf("requesting logits: %w", err)
		}
	}

	return nil
}

// mtpSyncInput is the common live-prefill/verification and cache-replay handoff.
// It borrows existing batch entries rather than rebuilding token-only rows.
// The caller must capture target hidden states before another target decode,
// and keep both buffers valid until synchronization completes.
func (e *batchEngine) mtpSyncInput(s *slot, rows []extendedBatchEntry, hiddenRows []float32, nextPosition llama.Pos) (mtpengine.SyncInput, error) {
	draft := e.model.draft.core()
	nEmbd := draft.mtp.EmbeddingSize()
	input := mtpengine.SyncInput{
		RowCount:      len(rows),
		HiddenRows:    hiddenRows,
		PendingHidden: s.mtp.PendingHidden,
		HiddenScratch: draft.mtp.MirrorHidden,
		NextPosition:  nextPosition,
		EmbeddingSize: nEmbd,
		ChunkSize:     draft.mtp.MirrorCapacity(),
		SharedKV:      mtpUsesSharedKV(e.model.draft),
	}
	if input.SharedKV {
		// Shared-KV handoff updates only the pending hidden state and logical
		// position. It must not write a replay batch through target aliases.
		return input, nil
	}

	input.DecodeOwnChunk = func(start, end int, shiftedHidden []float32) error {
		if len(s.seqIDs) == 0 {
			return errors.New("MTP replay: no sequence ID")
		}
		mirror := extendedBatch{native: draft.mtp.MirrorBatch}
		if err := llama.BatchExtClear(mirror.native); err != nil {
			return fmt.Errorf("clearing MTP mirror batch: %w", err)
		}
		for i, row := range rows[start:end] {
			// Projected input embeddings and MTP hidden states are separate
			// native inputs. Keep the former unshifted, including M-RoPE
			// coordinates; Synchronize shifts only the latter.
			if len(row.embedding) > 0 && row.embdWidth != int(llama.ModelNEmbdInp(draft.model)) {
				return fmt.Errorf("MTP media input width %d does not match draft width %d", row.embdWidth, llama.ModelNEmbdInp(draft.model))
			}
			row.sequenceID, row.extraSequenceIDs = s.seqIDs[0], s.seqIDs[1:]
			row.output = extendedBatchOutputNone
			idx, err := mirror.renderEntry(row)
			if err != nil {
				return fmt.Errorf("adding MTP mirror row: %w", err)
			}
			hidden := shiftedHidden[i*nEmbd : (i+1)*nEmbd]
			if err := llama.BatchExtSetEmbdState(mirror.native, idx, hidden, nEmbd); err != nil {
				return fmt.Errorf("setting MTP mirror state: %w", err)
			}
		}
		ret, err := llama.Process(draft.lctx, llama.ProcessTypeDecode, mirror.native)
		if err != nil || ret != 0 {
			e.model.log(s.job.ctx, "speculative", "status", "mtp-mirror-decode-error",
				"slot", s.id, "seq", s.seqID, "ret", ret, "err", err)
			return decodeError(ret, err)
		}
		llama.Synchronize(draft.lctx)
		return nil
	}
	return input, nil
}

func (e *batchEngine) syncMTPRows(s *slot, rows []extendedBatchEntry, hiddenRows []float32, nextPosition llama.Pos) error {
	input, err := e.mtpSyncInput(s, rows, hiddenRows, nextPosition)
	if err != nil {
		return err
	}
	result, err := mtpengine.Synchronize(input)
	if err != nil {
		return err
	}
	s.mtp.PendingHidden = result.PendingHidden
	s.draftNPast = result.Position
	return nil
}

// syncMTPDecodedRows captures and consumes target state before another target
// decode can replace it. The caller holds decodeMu. A failed draft handoff
// falls back to target-only execution for this request, not other slots.
func (e *batchEngine) syncMTPDecodedRows(s *slot, batch *extendedBatch, start int, nextPosition llama.Pos) error {
	if e.model.draft == nil || !e.model.draft.mtp() || s.mtp.Disabled || batch.len() == start {
		return nil
	}
	nEmbd := e.model.draft.core().mtp.EmbeddingSize()
	hidden := yzmaspec.GetEmbeddingsNextN(e.model.lctx, batch.len(), nEmbd)
	if hidden != nil {
		hidden = hidden[start*nEmbd:]
	}
	err := e.syncMTPRows(s, batch.entries[start:], hidden, nextPosition)
	if err == nil {
		return nil
	}
	e.model.log(s.job.ctx, "speculative", "status", "mtp-sync-error", "slot", s.id, "err", err)
	return e.disableMTPForRequestSpec(s.job.ctx, s, "sync-error", 0)
}

// resetMTPMediaPrefix pairs a full target media rebuild with an empty draft
// prefix. The caller holds decodeMu; shared-KV memory is cleared only through
// the target, never through its draft alias.
func (e *batchEngine) resetMTPMediaPrefix(s *slot) error {
	if e.model.draft == nil || !e.model.draft.mtp() {
		return nil
	}
	if ext, ok := e.model.draft.(draftKVExternalizer); ok {
		if _, err := llama.MemorySeqRm(ext.core().mem, s.seqID, -1, -1); err != nil {
			return fmt.Errorf("clearing MTP media prefix: %w", err)
		}
	}
	s.mtp.Reset()
	s.draftNPast = 0
	return nil
}

func (e *batchEngine) mtpDraftInput(s *slot) (mtpengine.DraftInput, error) {
	draft := e.model.draft.core()
	nEmbd := draft.mtp.EmbeddingSize()
	nDraft := e.maxDraftForSlot(s, draft.nDraft)
	if nDraft == 0 || len(s.mtp.PendingHidden) != nEmbd {
		return mtpengine.DraftInput{}, nil
	}

	shared := mtpUsesSharedKV(e.model.draft)
	mode := "mtp"
	if shared {
		mode = "mtp shared"
	}
	batch := draft.mtp.DraftBatch

	return mtpengine.DraftInput{
		Token:         s.sampled,
		Position:      s.draftNPast,
		Hidden:        s.mtp.PendingHidden,
		Count:         nDraft,
		FixedPosition: mtpUsesFixedDraftPosition(e.model.draft),
		Candidates:    s.draftTokensBuf,
		HiddenScratch: s.mtp.DraftHidden,
		IsEOG: func(token llama.Token) bool {
			return llama.VocabIsEOG(e.model.vocab, token)
		},
		DecodeStep: func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error) {
			if err := llama.BatchExtClear(batch); err != nil {
				return 0, nil, false, fmt.Errorf("%s draft: clearing batch: %w", mode, err)
			}
			if err := addMTPBatchEntry(batch, token, position, s.seqIDs, hidden, nEmbd, true); err != nil {
				return 0, nil, false, fmt.Errorf("%s draft: add token at pos %d: %w", mode, position, err)
			}

			ret, err := llama.Process(draft.lctx, llama.ProcessTypeDecode, batch)
			if err != nil || ret != 0 {
				e.model.log(s.job.ctx, "speculative", "status", "mtp-draft-decode-error",
					"slot", s.id, "seq", s.seqID, "mode", mode, "position", position, "token", token, "ret", ret, "err", err)
				if disableErr := e.disableMTPForRequestSpec(s.job.ctx, s, "draft-decode-error", 0); disableErr != nil {
					return 0, nil, false, fmt.Errorf("%s draft decode at pos %d: %w", mode, position, errors.Join(decodeError(ret, err), disableErr))
				}
				return 0, nil, false, nil
			}
			llama.Synchronize(draft.lctx)

			nextToken := llama.SamplerSample(draft.sampler, draft.lctx, -1)
			nextHidden := yzmaspec.GetEmbeddingsNextNIth(draft.lctx, 0, nEmbd)
			return nextToken, nextHidden, true, nil
		},
	}, nil
}

func (e *batchEngine) decodeTokensIntoCacheMTP(ctx context.Context, s *slot, tokens []llama.Token, startPos int) error {
	start := time.Now()

	nBatch := int(e.model.ctxParams.NBatch)
	if nBatch <= 0 {
		nBatch = e.model.cfg.EffectiveNBatch()
	}

	nTokens := len(tokens)
	if nTokens == 0 {
		return nil
	}

	e.model.log(ctx, "cache", "status", "decoding tokens into cache (mtp-mirror)",
		"seq", s.seqID, "tokens", nTokens, "start_pos", startPos, "nbatch", nBatch)

	batch, err := newExtendedBatchCapacity(e.model.lctx, min(nBatch, nTokens))
	if err != nil {
		return fmt.Errorf("imc-mtp: create target replay batch: %w", err)
	}
	defer batch.free()

	seqIDs := []llama.SeqId{s.seqID}

	decodeWaitStart := time.Now()
	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()
	decodeWaitElapsed := time.Since(decodeWaitStart)

	var targetDecodeElapsed time.Duration
	var mtpSyncElapsed time.Duration
	var chunks int

	err = decodeMTPMirrorChunks(nTokens, nBatch, func(i, end int) error {
		batch.clear()
		for j := i; j < end; j++ {
			pos := llama.Pos(startPos + j)
			if _, err := batch.addToken(tokens[j], pos, seqIDs, extendedBatchOutputNone); err != nil {
				return fmt.Errorf("imc-mtp: add target token at pos %d: %w", pos, err)
			}
		}

		targetDecodeStart := time.Now()
		ret, err := batch.process(llama.ProcessTypeDecode)
		if err != nil || ret != 0 {
			return fmt.Errorf("imc-mtp: target decode at pos %d: %w", startPos+i, decodeError(ret, err))
		}
		llama.Synchronize(e.model.lctx)
		targetDecodeElapsed += time.Since(targetDecodeStart)
		return nil
	}, func(i, end int) error {

		mtpSyncStart := time.Now()
		hiddenRows := yzmaspec.GetEmbeddingsNextN(e.model.lctx, end-i, e.model.draft.core().mtp.EmbeddingSize())
		if hiddenRows == nil {
			return fmt.Errorf("imc-mtp: target pre-norm rows unavailable at pos %d", startPos+i)
		}
		if err := e.syncMTPRows(s, batch.entries, hiddenRows, llama.Pos(startPos+end)); err != nil {
			return fmt.Errorf("imc-mtp: sync at pos %d: %w", startPos+i, err)
		}
		mtpSyncElapsed += time.Since(mtpSyncStart)
		chunks++
		return nil
	}, func(syncErr error) error {
		e.model.log(ctx, "cache", "status", "mtp-mirror-disabled", "slot", s.id, "seq", s.seqID, "err", syncErr)
		if err := e.disableMTPForRequestSpec(ctx, s, "sync-error", 0); err != nil {
			return fmt.Errorf("disabling MTP after cache sync failure: %w", errors.Join(syncErr, err))
		}
		return nil
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)
	e.model.log(ctx, "cache", "status", "finished (decoding tokens into cache (mtp-mirror))",
		"seq", s.seqID, "tokens", nTokens, "chunks", chunks, "nbatch", nBatch,
		"target_decode_elapsed", fmtDur(targetDecodeElapsed),
		"mtp_sync_elapsed", fmtDur(mtpSyncElapsed),
		"decode_wait_elapsed", fmtDur(decodeWaitElapsed),
		"elapsed", fmtDur(elapsed))

	return nil
}

func decodeMTPMirrorChunks(tokenCount, chunkSize int, decodeTarget, syncMTP func(start, end int) error, disableMTP func(error) error) error {
	mtpEnabled := true
	for start := 0; start < tokenCount; start += chunkSize {
		end := min(start+chunkSize, tokenCount)
		if err := decodeTarget(start, end); err != nil {
			return err
		}
		if !mtpEnabled {
			continue
		}
		if err := syncMTP(start, end); err != nil {
			if disableErr := disableMTP(err); disableErr != nil {
				return disableErr
			}
			mtpEnabled = false
		}
	}
	return nil
}

func (e *batchEngine) captureVerifyPreNorm(s *slot, count int) error {
	if count <= 0 {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return nil
	}

	draft := e.model.draft.core()
	nEmbd := draft.mtp.EmbeddingSize()
	totalRows := e.batch.len()
	start := int(s.mtp.TargetRange.Start)
	if start < 0 || start+count > totalRows {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: slot range [%d..%d) out of target batch (size %d)", start, start+count, totalRows)
	}

	embd := yzmaspec.GetEmbeddingsNextN(e.model.lctx, totalRows, nEmbd)
	if embd == nil {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: target pre-norm buffer is nil (SetEmbeddingsNextN may not be enabled)")
	}

	need := count * nEmbd
	if cap(s.mtp.VerifyHidden) < need {
		s.mtp.VerifyHidden = make([]float32, need)
	} else {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:need]
	}
	copy(s.mtp.VerifyHidden, embd[start*nEmbd:(start+count)*nEmbd])
	return nil
}

func (e *batchEngine) disableMTPForRequestSpec(ctx context.Context, s *slot, reason string, accepted int) error {
	if _, ownDraftKV := e.model.draft.(draftKVExternalizer); ownDraftKV {
		draft := e.model.draft.core()
		removed, err := llama.MemorySeqRm(draft.mem, s.seqID, -1, -1)
		if err != nil {
			return fmt.Errorf("clearing inconsistent MTP draft sequence: %w", err)
		}
		if !removed {
			return fmt.Errorf("clearing inconsistent MTP draft sequence for seq %d", s.seqID)
		}
	}
	s.draftNPast = 0
	if len(s.draftCachedTokens) > 0 {
		s.draftCachedTokens = s.draftCachedTokens[:0]
	}
	s.mtp.Disable(reason)
	e.model.log(ctx, "speculative", "status", "mtp-disabled-"+reason, "slot", s.id, "accepted", accepted)
	return nil
}
