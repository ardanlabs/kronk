package model

import (
	"context"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// finishSlotHybrid handles IMC state restore for Hybrid models. Full clear +
// snapshot restore replaces partial MemorySeqRm which corrupts recurrent state.
func (e *batchEngine) finishSlotHybrid(ctx context.Context, s *slot, slotID int, seqID llama.SeqId, trimPos llama.Pos) {
	switch {
	case len(s.imcSavedState) > 0:
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		nRead := llama.StateSeqSetData(e.model.lctx, s.imcSavedState, s.seqID)
		e.model.decodeMu.Unlock()

		switch {
		case nRead == 0:
			e.model.log(ctx, "finish-slot", "status", "imc-hybrid-restore-failed",
				"slot", slotID, "seq", seqID, "trim_pos", trimPos,
				"snapshot_bytes", len(s.imcSavedState))

			// Guardrail: clear IMC metadata so the slot isn't
			// reused with a corrupt sequence.
			e.model.cache.InvalidateSlot(slotID)

		default:
			e.model.log(ctx, "finish-slot", "status", "imc-hybrid-restore",
				"slot", slotID, "seq", seqID, "trim_pos", trimPos,
				"snapshot_bytes", len(s.imcSavedState), "restored_bytes", nRead)
		}

	default:
		// No snapshot available: full clear + invalidate metadata
		// to prevent reuse with corrupted recurrent state.
		e.model.log(ctx, "finish-slot", "status", "imc-hybrid-no-snapshot",
			"slot", slotID, "seq", seqID, "trim_pos", trimPos)

		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()

		e.model.cache.InvalidateSlot(slotID)
	}
}
