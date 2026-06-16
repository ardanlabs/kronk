package model

import "github.com/hybridgroup/yzma/pkg/llama"

// This file defines the speculative-decoding strategy types. The goal is
// CLEAR CODE-PATH SEPARATION between the runtime drafting modes so that
// each mode is a distinct Go type rather than a set of boolean flags
// (mtp/ownsModel/sharedKV) interpreted at every call site.
//
// Modes today:
//
//	nil              — no drafting / no speculation.
//	*classicDrafter  — separate-GGUF, vocab-matched draft model. Owns its
//	                   own llama_model AND its own KV cache.
//	*mtpDrafter      — embedded MTP head living inside the TARGET GGUF
//	                   (Qwen3.5 / Qwen3.6, arch qwen35). Shares the
//	                   target's llama_model but has its OWN draft KV; the
//	                   engine mirror-replays each target batch into it.
//	*sharedMTPDrafter — separate-file MTP "assistant" (Gemma4
//	                   gemma4-assistant). Loads its OWN llama_model from
//	                   Config.MTPDrafterFile but creates its context with
//	                   ctx_other==target, SHARING the target's
//	                   llama_memory. There is no separate draft KV to
//	                   mirror into: the target's decode populates the
//	                   shared KV directly, so the engine only captures the
//	                   last pre-norm hidden row (pendingH) after each
//	                   target decode and drafts at a fixed KV position.
//
// For the shared-KV mode, restoring draft KV state writes THROUGH aliased
// tensor pointers into the TARGET's KV buffer and corrupts it. That is why
// draft-KV state externalization lives behind the draftKVExternalizer
// capability below: a shared-KV strategy simply does not implement it, so
// the unsafe restore call site is unreachable for it at compile time
// rather than guarded by a runtime flag. The post-target-decode sync
// (mirror vs. capture) is dispatched through the mtpSyncer capability.

// draftKind identifies the speculative-decoding strategy. It is used only
// for logging/metrics; behavioral dispatch is via concrete types and
// capability interfaces, never a switch on this value.
type draftKind uint8

const (
	draftClassic   draftKind = iota // separate-GGUF, vocab-matched draft
	draftMTPQwen                    // embedded MTP head, own draft KV (Qwen)
	draftMTPGemma4                  // separate-file MTP assistant, shared KV (Gemma4)
)

func (k draftKind) String() string {
	switch k {
	case draftClassic:
		return "classic-separate"
	case draftMTPQwen:
		return "mtp-qwen"
	case draftMTPGemma4:
		return "mtp-gemma4-shared"
	default:
		return "unknown"
	}
}

// drafter is the speculative-decoding strategy held by a Model. A nil
// drafter means no speculation. The engine dispatches the mode-varying
// operations (generate, unload) through this interface; the heavy
// per-token bodies remain batchEngine methods that read draft resources
// via core().
type drafter interface {
	// sealedDrafter prevents types outside this package from satisfying
	// the interface, so the set of drafting modes is closed and every
	// mode is accounted for here.
	sealedDrafter()

	// kind reports the strategy for logging/metrics only.
	kind() draftKind

	// mtp reports whether this is an MTP (multi-token-prediction) head
	// strategy. True for any MTP mode (own- or shared-KV). MTP-common
	// behavior that is safe regardless of KV ownership (target-batch
	// mirroring, draft-KV trim via MemorySeqRm, the generate dispatch)
	// is gated on this.
	mtp() bool

	// core returns the shared llama resources (context, memory, sampler,
	// batches, reusable buffers). Decode and MemorySeqRm on these are
	// safe for every mode, including a future shared-KV mode.
	core() *draftCore

	// generate produces draft tokens for the slot's next speculative
	// round. Dispatched per-mode so adding a new mode does not require
	// editing the engine's generate call site.
	generate(e *batchEngine, s *slot) []llama.Token

	// unload releases the strategy's resources. Implementations differ in
	// whether they free the llama_model (classic owns it; MTP shares it
	// with the target) and whether MTP batches/pins exist.
	unload()
}

// draftKVExternalizer is implemented ONLY by strategies that OWN their
// draft KV cache and participate in IMC draft-KV state externalization
// (snapshot to / restore from host RAM in lock-step with the target seq).
//
// A shared-KV strategy (future Gemma4 gemma4-assistant, ctx_other==target)
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
}

// mtpSyncer is implemented by every MTP strategy (own- and shared-KV). It
// dispatches the two MTP "sync" operations whose body differs by KV
// ownership so the engine's call sites do not switch on concrete type:
//
//   - own-KV (Qwen): mirror-replay the just-decoded target batch into the
//     separate draft KV so the draft head's KV stays in lock-step.
//   - shared-KV (Gemma4): there is no separate draft KV; the target's
//     decode already populated the shared memory, so the syncer only
//     captures the last pre-norm hidden row into s.pendingH and advances
//     s.draftNPast.
//
// The heavy per-token bodies remain batchEngine methods; these methods
// just select which body runs for the slot.
type mtpSyncer interface {
	drafter

	// syncAfterTargetDecode runs after a successful target decode+sync for
	// the slot, propagating the just-decoded range into the MTP head's
	// view. effectiveCount is the number of positions whose target KV
	// survived (== targetBatchCount for prefill/gen; 1+accepted for spec
	// verify).
	syncAfterTargetDecode(e *batchEngine, s *slot, effectiveCount int) error

	// syncCacheBuildChunk is the IMC-cache-build analogue, called for each
	// freshly-decoded standalone target batch (not e.batch) during a cold
	// cache build.
	syncCacheBuildChunk(e *batchEngine, s *slot, tokens []llama.Token, basePos llama.Pos) error
}

// =============================================================================

// freeCommon releases the resources every drafter holds: the per-slot
// draft sampler registration, the greedy sampler, and the two token
// batches. It does NOT free the context or the model — unload sequences
// those per strategy.
func (dc *draftCore) freeCommon() {
	if dc.registeredSampler != 0 {
		llama.SetSampler(dc.lctx, dc.registeredSeqID, 0)
		dc.registeredSampler = 0
	}
	llama.SamplerFree(dc.sampler)
	llama.BatchFree(dc.batch)
	llama.BatchFree(dc.prefillBatch)
}

// freeMTPBatches releases the MTP-only mirror/draft batches. Their Embd
// pointers reference Go-owned, runtime.Pinner-pinned slices, so detach
// them before BatchFree (which would otherwise free() Go memory) and
// unpin after.
func (dc *draftCore) freeMTPBatches() {
	dc.draftBatchMTP.Embd = nil
	dc.mirrorBatchMTP.Embd = nil
	llama.BatchFree(dc.draftBatchMTP)
	llama.BatchFree(dc.mirrorBatchMTP)
	dc.draftEmbdPin.Unpin()
	dc.mirrorEmbdPin.Unpin()
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

func (d *classicDrafter) generate(e *batchEngine, s *slot) []llama.Token {
	return e.generateDraftTokens(s)
}

func (d *classicDrafter) unload() {
	d.c.freeCommon()
	llama.Free(d.c.lctx)
	// Classic drafts own their llama_model — free it.
	llama.ModelFree(d.c.model)
}

// =============================================================================

// mtpDrafter is an embedded MTP head (Qwen). It shares the target's
// llama_model but owns its own draft KV cache, so it can externalize draft
// KV state for IMC cache hits.
type mtpDrafter struct {
	c *draftCore
}

func (*mtpDrafter) sealedDrafter()     {}
func (*mtpDrafter) kind() draftKind    { return draftMTPQwen }
func (*mtpDrafter) mtp() bool          { return true }
func (d *mtpDrafter) core() *draftCore { return d.c }

func (d *mtpDrafter) draftKVCtx() llama.Context { return d.c.lctx }

func (d *mtpDrafter) generate(e *batchEngine, s *slot) []llama.Token {
	return e.generateDraftTokensMTP(s)
}

func (d *mtpDrafter) syncAfterTargetDecode(e *batchEngine, s *slot, effectiveCount int) error {
	return e.mirrorTargetBatchToMTPDraft(s, effectiveCount)
}

func (d *mtpDrafter) syncCacheBuildChunk(e *batchEngine, s *slot, tokens []llama.Token, basePos llama.Pos) error {
	return e.mirrorBuildChunkToMTPDraft(s, tokens, basePos)
}

func (d *mtpDrafter) unload() {
	d.c.freeCommon()
	d.c.freeMTPBatches()
	llama.Free(d.c.lctx)
	// MTP shares the target's llama_model — the target's Unload path owns
	// its lifetime. Skip ModelFree to avoid a double-free.
}

// =============================================================================

// sharedMTPDrafter is a separate-file MTP "assistant" head (Gemma4
// gemma4-assistant). It loads its OWN llama_model from Config.MTPDrafterFile,
// but its context is created with ctx_other==target so it SHARES the
// target's llama_memory. There is no separate draft KV to externalize or
// mirror into — hence it implements mtpSyncer (capture-only) but NOT
// draftKVExternalizer.
type sharedMTPDrafter struct {
	c *draftCore
}

func (*sharedMTPDrafter) sealedDrafter()     {}
func (*sharedMTPDrafter) kind() draftKind    { return draftMTPGemma4 }
func (*sharedMTPDrafter) mtp() bool          { return true }
func (d *sharedMTPDrafter) core() *draftCore { return d.c }

func (d *sharedMTPDrafter) generate(e *batchEngine, s *slot) []llama.Token {
	return e.generateDraftTokensMTPShared(s)
}

func (d *sharedMTPDrafter) syncAfterTargetDecode(e *batchEngine, s *slot, effectiveCount int) error {
	return e.captureTargetBatchForSharedMTP(s, effectiveCount)
}

func (d *sharedMTPDrafter) syncCacheBuildChunk(e *batchEngine, s *slot, tokens []llama.Token, basePos llama.Pos) error {
	return e.captureBuildChunkForSharedMTP(s, tokens, basePos)
}

func (d *sharedMTPDrafter) unload() {
	d.c.freeCommon()
	d.c.freeMTPBatches()
	llama.Free(d.c.lctx)
	// Shared MTP loaded its OWN assistant llama_model from MTPDrafterFile —
	// free it (unlike embedded Qwen MTP, which shares the target model).
	llama.ModelFree(d.c.model)
}

// =============================================================================

// Compile-time guarantees about which capabilities each strategy exposes.
// The absence of a draftKVExternalizer assertion for *sharedMTPDrafter is
// intentional and load-bearing: it is what keeps the unsafe draft-KV
// restore path unreachable for shared-KV modes at compile time.
var (
	_ drafter             = (*classicDrafter)(nil)
	_ drafter             = (*mtpDrafter)(nil)
	_ drafter             = (*sharedMTPDrafter)(nil)
	_ draftKVExternalizer = (*mtpDrafter)(nil)
	_ mtpSyncer           = (*mtpDrafter)(nil)
	_ mtpSyncer           = (*sharedMTPDrafter)(nil)
)
