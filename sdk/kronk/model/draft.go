package model

import (
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Shared drafter contracts and classic cleanup live here. Internal MTP owns its
// native resources; its model-facing adapters are defined in mtp.go.

// drafter is the model-owned resource adapter for speculative decoding. A nil
// drafter means no speculation. Algorithm controllers own the decoding lifecycle;
// their model-facing host methods access native resources through core().
type drafter interface {
	// sealedDrafter keeps native resource adapters in the model package.
	// Algorithm controllers live behind the internal speculation contract.
	sealedDrafter()

	// kind reports the strategy for logging/metrics only.
	kind() draftKind

	// mtp reports whether this is an MTP (multi-token-prediction) head
	// strategy. True for any MTP mode (own- or shared-KV). It does not
	// grant permission to mutate or externalize the draft KV cache.
	mtp() bool

	// core returns the shared llama resources (context, memory, sampler,
	// batches, reusable buffers). Operations on these handles must respect
	// the adapter's KV ownership and externalization capabilities.
	core() *draftCore

	// unload releases owned resources without freeing borrowed target state.
	unload()
}

// draftKVExternalizer is implemented ONLY by strategies that OWN their
// draft KV cache and participate in IMC draft-KV state externalization
// (snapshot to / restore from a RAM checkpoint in lock-step with the target seq).
//
// A shared-KV strategy (Gemma4 gemma4-assistant, ctx_other==target)
// MUST NOT implement this: llama.StateSeqSetData on a shared draft context
// writes through aliased tensor pointers into the TARGET's KV buffer and
// corrupts it. Because the shared-KV type does not satisfy this interface,
// the IMC draft snapshot/restore blocks — the only sites that perform
// StateSeqGetData/StateSeqSetData on a draft context — are unreachable for
// it at compile time.
type draftKVExternalizer interface {
	drafter

	// draftKVCtx returns the draft context whose per-seq KV state may be
	// safely serialized/restored. Only own-draft-KV strategies expose it.
	draftKVCtx() llama.Context

	// draftCheckpointState returns the borrowed algorithm state paired with
	// the exact accepted prefix, or nil when it cannot be checkpointed.
	draftCheckpointState(s *slot, position llama.Pos) []float32

	// restoreDraftCheckpoint validates and restores algorithm state after a
	// complete native KV transfer. It must not retain the borrowed state slice.
	restoreDraftCheckpoint(s *slot, position llama.Pos, state []float32) bool

	// clearDraftCheckpoint invalidates algorithm state after failed recovery.
	// The caller has already removed the independent native sequence.
	clearDraftCheckpoint(s *slot)
}

// =============================================================================

// freeCommon releases classic draft resources: the per-slot
// draft sampler registration, the greedy sampler, and the token batch.
// It does NOT free the context or the model — unload sequences
// those. MTP uses its internal resource owner's cleanup instead.
func (dc *draftCore) freeCommon() {
	if dc.registeredSampler != 0 {
		llama.SetSampler(dc.lctx, dc.registeredSeqID, 0)
		dc.registeredSampler = 0
	}

	llama.SamplerFree(dc.sampler)
	dc.batch.free()
}

// =============================================================================

// classicDrafter is a separate-GGUF, vocab-matched draft model. It owns
// both its llama_model and its KV cache.
type classicDrafter struct {
	c *draftCore
}

func (*classicDrafter) sealedDrafter()     {}
func (*classicDrafter) kind() draftKind    { return draftClassic }
func (*classicDrafter) mtp() bool          { return false }
func (d *classicDrafter) core() *draftCore { return d.c }

func (d *classicDrafter) unload() {
	d.c.freeCommon()
	llama.Free(d.c.lctx)
	// Classic drafts own their llama_model — free it.
	llama.ModelFree(d.c.model)
}

// =============================================================================

var _ drafter = (*classicDrafter)(nil)
