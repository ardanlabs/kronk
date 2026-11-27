package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// snapshotDraftCheckpoint pairs independent native KV with the algorithm-owned
// state at the same accepted boundary. The caller reserves the destination store
// and publishes both halves only after a complete target checkpoint.
func (e *batchEngine) snapshotDraftCheckpoint(ctx context.Context, ext draftKVExternalizer, s *slot, store *imcCheckpoint, position llama.Pos) (state []float32, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, store.Reset(ctx))
		}
	}()
	e.model.decodeMu.Lock()
	dctx := ext.draftKVCtx()
	llama.Synchronize(dctx)
	state = ext.draftCheckpointState(s, position)
	size := llama.StateSeqGetSize(dctx, s.seqID)
	e.model.decodeMu.Unlock()
	if len(state) == 0 || size == 0 {
		return nil, fmt.Errorf("draft checkpoint unavailable at position %d", position)
	}
	buf, err := prepareSessionSnapshot(ctx, store, size)
	if err != nil {
		return nil, err
	}
	e.model.decodeMu.Lock()
	extracted := llama.StateSeqGetData(dctx, buf, s.seqID)
	e.model.decodeMu.Unlock()
	if extracted != size {
		return nil, fmt.Errorf("extract draft checkpoint: got %d bytes, want %d", extracted, size)
	}
	if err := store.Commit(ctx, int(extracted)); err != nil {
		return nil, fmt.Errorf("commit draft checkpoint: %w", err)
	}
	return state, nil
}

// restoreDraftCheckpoint restores independent KV and its algorithm-owned state.
// An unusable pair is discarded while target-only reuse stays available. Failure
// to clear uncertain native state is fatal; it must not be hidden by fallback.
func (e *batchEngine) restoreDraftCheckpoint(ctx context.Context, ext draftKVExternalizer, s *slot, store *imcCheckpoint, length int, state []float32, position llama.Pos) error {
	var data []byte
	var restoreErr error
	if store != nil {
		data, restoreErr = store.Bytes(ctx)
	}
	if restoreErr == nil && (len(data) == 0 || len(data) != length || len(state) == 0) {
		restoreErr = fmt.Errorf("incomplete draft checkpoint: bytes=%d expected=%d state=%d", len(data), length, len(state))
	}
	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()
	if restoreErr == nil {
		read := llama.StateSeqSetData(ext.draftKVCtx(), data, s.seqID)
		if read != uint64(len(data)) || !ext.restoreDraftCheckpoint(s, position, state) {
			restoreErr = fmt.Errorf("invalid draft checkpoint at position %d: read %d of %d bytes", position, read, len(data))
		}
	}
	if restoreErr == nil {
		e.model.log(ctx, "start-slot", "status", "imc-draft-restore-done",
			"slot", s.id, "seq", s.seqID, "cached_tokens", position,
			"restored_bytes", fmtBytes(uint64(len(data))), "state_values", len(state))
		return nil
	}
	removed, cleanupErr := llama.MemorySeqRm(ext.core().mem, s.seqID, -1, -1)
	ext.clearDraftCheckpoint(s)
	if cleanupErr != nil {
		return errors.Join(restoreErr, fmt.Errorf("clearing failed draft checkpoint: %w", cleanupErr))
	}
	if !removed {
		return errors.Join(restoreErr, errors.New("clearing failed draft checkpoint: native suffix removal failed"))
	}
	e.model.log(ctx, "start-slot", "status", "imc-draft-restore-failed",
		"slot", s.id, "seq", s.seqID, "cached_tokens", position, "err", restoreErr)
	return nil
}
