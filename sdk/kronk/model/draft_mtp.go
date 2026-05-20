package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// defMTPNDraft is the default number of speculative tokens to draft per
// round when MTP is auto-enabled and no explicit count was configured.
// Conservative default — MTP heads typically have high acceptance for
// the first 1-3 tokens and rapidly decay beyond that. The adaptive
// chooseNDraft EMA will scale down further if acceptance is poor.
const defMTPNDraft = 4

// mtpNextNLayers returns the number of NextN (MTP) prediction layers
// declared in the target GGUF's metadata. A return value of 0 means the
// model does not contain an MTP head and the MTP drafter must not be
// loaded.
//
// The canonical signal (per llama.cpp src/models/qwen35.cpp) is the
// metadata key "<arch>.nextn_predict_layers", a uint32. We match by the
// unique substring "nextn_predict_layers" so the same lookup works for
// every architecture variant (qwen35, qwen35moe, future) without needing
// to read general.architecture first.
func mtpNextNLayers(model llama.Model) int {
	raw, ok := searchModelMeta(model, "nextn_predict_layers")
	if !ok {
		return 0
	}

	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0
	}

	return n
}

// loadDraftModelMTP creates an MTP draft context against an already-loaded
// target model. The MTP head weights live inside the target GGUF and share
// the target's llama_model pointer — there is no extra file to load.
//
// Two pieces of plumbing distinguish MTP from a normal draft context and
// MUST be done here:
//
//  1. SetEmbeddingsPreNorm enables pre-norm hidden-state extraction:
//     - target ctx:  (true, false) — dense, every row available by
//     raw batch index for the mirror step.
//     - draft  ctx:  (true, true)  — masked, only logits-flagged rows
//     stored; indexed via the output_ids table.
//  2. Batches are allocated with embd=nEmbd so Batch.Embd is a real
//     C buffer the MTP graph can read. (BatchInit with embd=0 only
//     allocates the token slot, no embd buffer.)
//
// On success the returned *draftModel has mtp=true so Unload skips the
// shared llama_model free.
func loadDraftModelMTP(ctx context.Context, log applog.Logger, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, nDraft int) (*draftModel, error) {

	// Build context params for the MTP draft context. We inherit thread
	// layout, KV cache types, and offload behavior from the target so
	// the MTP head runs on the same backend. NCtx / NBatch / NUbatch /
	// NSeqMax are inherited so the drafter can host the same number of
	// concurrent sequences the target serves and score the same prompts
	// the target sees.
	params := llama.ContextDefaultParams()
	params.CtxType = llama.ContextTypeMTP
	params.NCtx = targetCtxParams.NCtx
	params.NBatch = targetCtxParams.NBatch
	params.NUbatch = targetCtxParams.NUbatch
	params.NSeqMax = targetCtxParams.NSeqMax
	params.NThreads = targetCtxParams.NThreads
	params.NThreadsBatch = targetCtxParams.NThreadsBatch
	params.FlashAttentionType = targetCtxParams.FlashAttentionType
	params.TypeK = targetCtxParams.TypeK
	params.TypeV = targetCtxParams.TypeV
	params.Offload_kqv = targetCtxParams.Offload_kqv
	params.OpOffload = targetCtxParams.OpOffload

	nEmbd := int(llama.ModelNEmbd(targetModel))
	if nEmbd <= 0 {
		return nil, fmt.Errorf("invalid nEmbd %d from target model", nEmbd)
	}

	log(ctx, "draft-model-mtp", "status", "loading",
		"nDraft", nDraft,
		"nEmbd", nEmbd,
		"nCtx", params.NCtx,
		"nBatch", params.NBatch,
		"nUbatch", params.NUbatch,
		"nSeqMax", params.NSeqMax)

	lctx, err := llama.InitFromModel(targetModel, params)
	if err != nil {
		return nil, fmt.Errorf("init-mtp-context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		return nil, fmt.Errorf("get-mtp-memory: %w", err)
	}

	llama.MemoryClear(mem, true)

	// Enable pre-norm hidden-state extraction. Order is:
	//   target  → masked=false (dense, all rows accessible by raw batch idx)
	//   draft   → masked=true  (sparse, only logits-flagged rows)
	//
	// This mirrors common_speculative_impl_draft_mtp (common/speculative.cpp).
	// Must be set BEFORE any decode on either context — the cparams flag
	// is read at graph build time.
	SetEmbeddingsPreNorm(targetCtx, true, false)
	SetEmbeddingsPreNorm(lctx, true, true)

	// Greedy sampler for the draft (temperature=0 for speed). The
	// HEAP-CORRUPTION WORKAROUND mirrors loadDraftModel; see the
	// detailed comment on toSampler in params.go.
	sampler := llama.SamplerChainInit(llama.SamplerChainParams{NoPerf: 1})
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())

	// MTP-specific batches: every position carries both a token id and a
	// pre-norm hidden-state vector.
	//
	// llama_batch_init allocates EITHER the token buffer OR the embd
	// buffer, never both — controlled by its embd arg. MTP needs both
	// per position, so we call BatchInit(N, 0, 1) to get a token-only
	// batch (with pos/seq_id/logits arrays sized to N) and then attach
	// a Go-allocated []float32 of size N*nEmbd as the embd buffer.
	//
	//   draftBatchMTP   : capacity 1 token, used by generateDraftTokensMTP.
	//   mirrorBatchMTP  : capacity NBatch tokens, used to mirror a target
	//                     decode into the draft KV (prefill chunk, gen
	//                     token, or spec verify accepted prefix).
	//
	// The Go slices must be pinned (runtime.Pinner) for the lifetime of
	// the batch — the C side reads through Batch.Embd repeatedly across
	// decode calls, and Go's GC is allowed to move heap objects. The
	// pin and the slice are both stored on draftModel so Unload can
	// release them in lock-step with BatchFree.
	//
	// batch and prefillBatch remain zero because MTP doesn't use them,
	// but they're kept allocated for type-uniformity with the
	// separate-GGUF draft (BatchFree on a zero struct is harmless).
	nVocab := int(llama.VocabNTokens(llama.ModelGetVocab(targetModel)))

	draftProbs := make([][]float32, nDraft)
	for i := range draftProbs {
		draftProbs[i] = make([]float32, nVocab)
	}

	// Construct the *draftModel BEFORE pinning so the runtime.Pinner
	// fields stay at their final addresses. runtime.Pinner is invalid
	// to copy once it holds pinned pointers, so we can't Pin into a
	// local var and then move the Pinner into the struct literal.
	dm := &draftModel{
		model:   targetModel,
		vocab:   llama.ModelGetVocab(targetModel),
		mtp:     true,
		nEmbd:   nEmbd,
		lctx:    lctx,
		mem:     mem,
		sampler: sampler,
		// batch and prefillBatch stay zero for MTP — MTP code paths use
		// draftBatchMTP / mirrorBatchMTP directly. Unload's
		// BatchFree(zero) is a safe no-op (llama_batch_free NULL-checks
		// each member), and avoids the double-free that would happen if
		// we aliased batch:=draftBatchMTP and then freed both.
		draftBatchMTP:   llama.BatchInit(1, 0, 1),
		mirrorBatchMTP:  llama.BatchInit(int32(params.NBatch), 0, 1),
		draftEmbdSlice:  make([]float32, nEmbd),
		mirrorEmbdSlice: make([]float32, int(params.NBatch)*nEmbd),
		nDraft:          nDraft,
		draftBuf:        make([]llama.Token, 0, nDraft),
		draftProbs:      draftProbs,
		targetProbs:     make([]float32, nVocab),
		adjusted:        make([]float32, nVocab),
	}

	// Pin the Go-owned embd buffers and attach them to the batches.
	// The Pinners live on dm, so Unpin in Unload releases them; the
	// batches' Embd pointers are cleared before BatchFree there too.
	if len(dm.draftEmbdSlice) > 0 {
		dm.draftEmbdPin.Pin(&dm.draftEmbdSlice[0])
		dm.draftBatchMTP.Embd = (*float32)(unsafe.Pointer(&dm.draftEmbdSlice[0]))
	}
	if len(dm.mirrorEmbdSlice) > 0 {
		dm.mirrorEmbdPin.Pin(&dm.mirrorEmbdSlice[0])
		dm.mirrorBatchMTP.Embd = (*float32)(unsafe.Pointer(&dm.mirrorEmbdSlice[0]))
	}

	return dm, nil
}

// selectAndLoadDraft chooses the appropriate draft source and loads it,
// or returns (nil, nil) when no drafter applies. Two sources, checked in
// priority order:
//
//  1. Explicit separate-draft GGUF (cfg.DraftModel) — user override.
//  2. Auto-detect: enable MTP when the target GGUF carries an MTP head
//     (nextn_predict_layers > 0) and the target is single-sequence.
//
// targetCtx is needed because MTP requires
// llama_set_embeddings_pre_norm(target, true, false) before the next
// target decode — the MTP loader handles that call internally.
//
// The caller is responsible for cleanup on error; this function only
// owns resources it returns successfully.
func selectAndLoadDraft(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams) (*draftModel, error) {
	if cfg.DraftModel != nil {
		d, err := loadDraftModel(ctx, log, cfg, targetModel, targetCtxParams)
		if err != nil {
			return nil, err
		}
		log(ctx, "draft-model", "status", "loaded",
			"source", "explicit-separate",
			"nDraft", d.nDraft, "devices", cfg.DraftModel.Devices,
			"nCtx", llama.NCtx(d.lctx))
		return d, nil
	}

	nLayers := mtpNextNLayers(targetModel)
	if nLayers == 0 {
		log(ctx, "draft-model-mtp", "status", "auto-detect-skipped",
			"reason", "no nextn_predict_layers metadata in target GGUF")
		return nil, nil
	}

	// Older llama.cpp builds (pre src/llama-ext.h pre-norm API) don't
	// export the symbols MTP needs. Without those, the MTP head would
	// predict blind. Skip silently — kronk runs without speculation
	// rather than crashing on a missing symbol mid-request.
	if !MTPAvailable() {
		log(ctx, "draft-model-mtp", "status", "auto-detect-skipped",
			"reason", "llama library does not export pre-norm hidden-state APIs (llama_set_embeddings_pre_norm / llama_get_embeddings_pre_norm / _ith); upgrade llama.cpp to a build that includes src/llama-ext.h")
		return nil, nil
	}

	d, err := loadDraftModelMTP(ctx, log, targetCtx, targetModel, targetCtxParams, defMTPNDraft)
	if err != nil {
		return nil, err
	}
	log(ctx, "draft-model-mtp", "status", "loaded",
		"source", "auto-detected",
		"nDraft", d.nDraft, "nextn-layers", nLayers,
		"nEmbd", d.nEmbd,
		"nCtx", llama.NCtx(d.lctx))
	return d, nil
}
