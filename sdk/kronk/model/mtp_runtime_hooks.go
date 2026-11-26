package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// MTPDisableReason exposes the selected request's MTP fallback status.
func (e *batchEngine) MTPDisableReason(slotID int) string {
	return e.slots[slotID].mtp.DisableReason
}

// MTPPrefillInput borrows the completed target rows under the decode lock.
// MTP owns synchronization policy; the engine preserves media positions and
// supplies either copied hidden values or a device row view.
func (e *batchEngine) MTPPrefillInput(slotID int, prefill speculation.Prefill) (mtpengine.SyncInput, error) {
	s := e.slots[slotID]
	if s.mtp.Disabled || prefill.Count == 0 {
		return mtpengine.SyncInput{}, nil
	}
	batch := e.decodedPrefill
	resources := mtpResources(e.model.draft)
	hidden := resources.TargetHidden(batch.len())
	start, end := int(prefill.Start), int(prefill.Start+prefill.Count)
	if hidden != nil {
		hidden = hidden[start*resources.EmbeddingSize() : end*resources.EmbeddingSize()]
	} else if !resources.DeviceRows() {
		return mtpengine.SyncInput{}, fmt.Errorf("target pre-norm rows unavailable for prefill [%d..%d)", start, end)
	}
	return e.mtpSyncInput(s, batch.entries[start:end], hidden, prefill.NextPosition, start)
}

// mtpSyncInput is the common live-prefill/verification and cache-replay handoff.
// It borrows existing batch entries rather than rebuilding token-only rows.
// The caller must capture target hidden states before another target decode,
// and keep both buffers valid until synchronization completes.
func (e *batchEngine) mtpSyncInput(s *slot, rows []extendedBatchEntry, hiddenRows []float32, nextPosition llama.Pos, deviceRowStart int) (mtpengine.SyncInput, error) {
	draft := e.model.draft.core()
	resources := mtpResources(e.model.draft)
	nEmbd := resources.EmbeddingSize()
	input := mtpengine.SyncInput{
		RowCount:      len(rows),
		HiddenRows:    hiddenRows,
		PendingHidden: s.mtp.PendingHidden,
		HiddenScratch: resources.MirrorHidden,
		NextPosition:  nextPosition,
		EmbeddingSize: nEmbd,
		ChunkSize:     resources.MirrorCapacity(),
		SharedKV:      mtpUsesSharedKV(e.model.draft),
	}
	if input.SharedKV {
		// Shared-KV handoff updates only the pending hidden state and logical
		// position. It must not write a replay batch through target aliases.
		return input, nil
	}

	device := resources.DeviceRows() && hiddenRows == nil
	var pendingRow int32
	if device {
		var err error
		pendingRow, err = resources.PreparePendingRow(s.seqID, s.mtp.PendingHidden)
		if err != nil {
			return input, err
		}
	}

	input.DecodeOwnChunk = func(start, end int, shiftedHidden []float32) error {
		if len(s.seqIDs) == 0 {
			return errors.New("MTP replay: no sequence ID")
		}
		mirror := extendedBatch{native: resources.MirrorBatch}
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
			if device {
				stateRow := int32(deviceRowStart + start + i - 1)
				if start+i == 0 {
					stateRow = pendingRow
				}
				if err := resources.SetMirrorState(idx, nil, stateRow); err != nil {
					return err
				}
			} else {
				hidden := shiftedHidden[i*nEmbd : (i+1)*nEmbd]
				if err := resources.SetMirrorState(idx, hidden, 0); err != nil {
					return err
				}
			}
		}
		ret, err := resources.DecodeMirror()
		if err != nil || ret != 0 {
			e.model.log(s.job.ctx, "speculative", "status", "mtp-mirror-decode-error",
				"slot", s.id, "seq", s.seqID, "ret", ret, "err", err)
			return decodeError(ret, err)
		}
		return nil
	}
	if device {
		input.Device = &mtpengine.DeviceSync{
			DecodeChunk: func(start, end int) error {
				return input.DecodeOwnChunk(start, end, nil)
			},
			ReadLastRow: func(out []float32) error {
				return resources.ReadLastRow(deviceRowStart+len(rows)-1, out)
			},
		}
	}
	return input, nil
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
	resources := mtpResources(e.model.draft)
	nEmbd := resources.EmbeddingSize()
	nDraft := e.maxDraftForSlot(s, draft.nDraft)
	if nDraft == 0 || len(s.mtp.PendingHidden) != nEmbd {
		return mtpengine.DraftInput{}, nil
	}

	shared := mtpUsesSharedKV(e.model.draft)
	mode := "mtp"
	if shared {
		mode = "mtp shared"
	}
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
			if err := resources.PrepareDraft(token, position, s.seqIDs, hidden); err != nil {
				return 0, nil, false, err
			}

			ret, err := resources.DecodeDraft()
			if err != nil || ret != 0 {
				e.model.log(s.job.ctx, "speculative", "status", "mtp-draft-decode-error",
					"slot", s.id, "seq", s.seqID, "mode", mode, "position", position, "token", token, "ret", ret, "err", err)
				if disableErr := e.disableMTPForRequestSpec(s.job.ctx, s, "draft-decode-error", 0); disableErr != nil {
					return 0, nil, false, fmt.Errorf("%s draft decode at pos %d: %w", mode, position, errors.Join(decodeError(ret, err), disableErr))
				}
				return 0, nil, false, nil
			}
			nextToken, nextHidden := resources.SampleDraft()
			return nextToken, nextHidden, true, nil
		},
	}, nil
}

func (e *batchEngine) captureVerifyPreNorm(s *slot, count int) error {
	if count <= 0 {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return nil
	}

	resources := mtpResources(e.model.draft)
	nEmbd := resources.EmbeddingSize()
	totalRows := e.batch.len()
	start := int(s.mtp.TargetRange.Start)
	if start < 0 || start+count > totalRows {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: slot range [%d..%d) out of target batch (size %d)", start, start+count, totalRows)
	}

	need := count * nEmbd
	if cap(s.mtp.VerifyHidden) < need {
		s.mtp.VerifyHidden = make([]float32, need)
	} else {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:need]
	}
	copied, err := resources.CopyTargetHidden(totalRows, start, count, s.mtp.VerifyHidden)
	if err != nil {
		return err
	}
	if !copied {
		s.mtp.VerifyHidden = s.mtp.VerifyHidden[:0]
		return fmt.Errorf("verify-prenorm-capture: target pre-norm buffer is nil (SetEmbeddingsNextN may not be enabled)")
	}
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
