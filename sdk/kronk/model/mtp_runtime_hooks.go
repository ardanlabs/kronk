package model

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// batchTokensAt aliases a generation batch's token-id range [start..start+count) of a
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

func (e *batchEngine) mtpDraftInput(s *slot) (mtpengine.DraftInput, error) {
	draft := e.model.draft.core()
	nEmbd := draft.mtp.EmbeddingSize()
	nDraft := e.maxDraftForSlot(s, draft.nDraft)
	if nDraft == 0 || len(s.mtp.PendingHidden) != nEmbd {
		return mtpengine.DraftInput{}, nil
	}

	_, shared := e.model.draft.(*sharedMTPDrafter)
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
		FixedPosition: shared,
		Candidates:    s.draftTokensBuf,
		HiddenScratch: s.mtp.DraftHidden,
		IsEOG: func(token llama.Token) bool {
			return llama.VocabIsEOG(e.model.vocab, token)
		},
		DecodeStep: func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error) {
			batch.NTokens = 0
			if err := batch.Add(token, position, s.seqIDs, true); err != nil {
				return 0, nil, false, fmt.Errorf("%s draft: add token at pos %d: %w", mode, position, err)
			}
			copy(draft.mtp.DraftHidden, hidden)

			ret, err := llama.Decode(draft.lctx, batch)
			if err != nil || ret != 0 {
				return 0, nil, false, nil
			}
			llama.Synchronize(draft.lctx)

			nextToken := llama.SamplerSample(draft.sampler, draft.lctx, -1)
			nextHidden := GetEmbeddingsPreNormIth(draft.lctx, 0, nEmbd)
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

	batchSize := int32(min(nBatch, nTokens))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{s.seqID}

	decodeWaitStart := time.Now()
	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()
	decodeWaitElapsed := time.Since(decodeWaitStart)

	var targetDecodeElapsed time.Duration
	var mtpSyncElapsed time.Duration
	var chunks int

	err := decodeMTPMirrorChunks(nTokens, nBatch, func(i, end int) error {
		batch.Clear()
		for j := i; j < end; j++ {
			pos := llama.Pos(startPos + j)
			if err := batch.Add(tokens[j], pos, seqIDs, false); err != nil {
				return fmt.Errorf("imc-mtp: add target token at pos %d: %w", pos, err)
			}
		}

		targetDecodeStart := time.Now()
		ret, err := llama.Decode(e.model.lctx, batch)
		if err != nil || ret != 0 {
			return fmt.Errorf("imc-mtp: target decode at pos %d: %w", startPos+i, decodeError(ret, err))
		}
		llama.Synchronize(e.model.lctx)
		targetDecodeElapsed += time.Since(targetDecodeStart)
		return nil
	}, func(i, end int) error {

		mtpSyncStart := time.Now()
		hiddenRows := GetEmbeddingsPreNorm(e.model.lctx, end-i, e.model.draft.core().mtp.EmbeddingSize())
		if hiddenRows == nil {
			return fmt.Errorf("imc-mtp: target pre-norm rows unavailable at pos %d", startPos+i)
		}
		if err := e.syncMTPCacheRows(s, tokens[i:end], hiddenRows, llama.Pos(startPos+i)); err != nil {
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
	totalRows := int(e.batch.NTokens)
	start := int(s.mtp.TargetRange.Start)
	if start < 0 || start+count > totalRows {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: slot range [%d..%d) out of target batch (size %d)", start, start+count, totalRows)
	}

	embd := GetEmbeddingsPreNorm(e.model.lctx, totalRows, nEmbd)
	if embd == nil {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: target pre-norm buffer is nil (SetEmbeddingsPreNorm may not be enabled)")
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
