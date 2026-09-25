package model

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// defMTPNDraft is the default number of speculative tokens to draft per
// round when MTP is auto-enabled and no explicit count was configured.
// This matches llama.cpp's MTP default.
const defMTPNDraft = 3

// modelFilesLoadMTP reports whether the first GGUF shard declares one or
// more MTP prediction layers. GGUF metadata lives in the first shard, so no
// other file is read. This check must run before llama loads the model:
// llama.cpp skips gated MTP tensors unless ModelParams.LoadMTP is enabled.
func modelFilesLoadMTP(modelFiles []string) (bool, error) {
	if len(modelFiles) == 0 {
		return false, fmt.Errorf("no model files provided")
	}

	data, err := gguf.ReadHeaderBytes(modelFiles[0])
	if err != nil {
		return false, err
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return false, err
	}

	return modelprofile.Resolve(metadata).Speculation.NextNPredictLayers > 0, nil
}

// metadataHasMTP reports whether metadata contains a positive numeric
// nextn_predict_layers value. The architecture prefix is intentionally not
// constrained because llama.cpp uses the same metadata suffix across model
// families.
func metadataHasMTP(metadata map[string]string) bool {
	return modelprofile.Resolve(metadata).Speculation.NextNPredictLayers > 0
}

func metadataHasAssistantMTP(metadata map[string]string) bool {
	return modelprofile.Resolve(metadata).Speculation.SharedKVCompanion
}

func metadataHasOwnKVCompanionMTP(metadata map[string]string) bool {
	return modelprofile.Resolve(metadata).Speculation.OwnKVCompanion
}

// mtpNDraft returns the starting (ceiling) number of draft tokens for the
// auto-detected MTP drafter. An MTP nDraft override — a DraftModel block
// with no model files — sets the count explicitly; otherwise the default
// defMTPNDraft is used.
func mtpNDraft(cfg Config) int {
	if cfg.PtrDraftModel != nil && !cfg.PtrDraftModel.IsSeparate() && cfg.PtrDraftModel.NDraft > 0 {
		return cfg.PtrDraftModel.NDraft
	}
	return defMTPNDraft
}

// RecurrentStateCopies returns the number of current and rollback recurrent
// state planes allocated for the configured speculative-decoding mode.
// embeddedMTP selects the auto-detected MTP path; false selects an explicit
// separate drafter or companion MTP file.
func RecurrentStateCopies(cfg Config, embeddedMTP bool) int64 {
	mode := cfg.SpeculationMode()
	if mode == SpeculationDisabled {
		return 1
	}

	if embeddedMTP {
		if mode != SpeculationClassic && yzmaspec.Available() {
			return int64(1 + mtpNDraft(cfg))
		}
		return 1
	}

	if mode != SpeculationMTP && cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() {
		nDraft := cfg.PtrDraftModel.NDraft
		if nDraft <= 0 {
			nDraft = defNDraft
		}
		return int64(1 + nDraft)
	}
	if mode != SpeculationClassic && cfg.MTPDrafterFile != "" && yzmaspec.Available() {
		return int64(1 + mtpNDraft(cfg))
	}

	return 1
}

// SpeculativeContextCount returns the native contexts known from configuration
// before the target GGUF metadata is inspected. A companion MTP model creates
// a second target-shaped context. A separate classic drafter is estimated from
// its own GGUF, while embedded MTP is detected from the target's NextN metadata.
func SpeculativeContextCount(cfg Config) int64 {
	if cfg.SpeculationMode() == SpeculationDisabled {
		return 1
	}
	if cfg.MTPDrafterFile != "" {
		return 2
	}
	return 1
}

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
//  1. speculative.SetEmbeddingsNextN enables pre-norm hidden-state extraction:
//     - target ctx:  (true, false) — dense, every row available by
//     raw batch index for the mirror step.
//     - draft  ctx:  (true, true)  — masked, only logits-flagged rows
//     stored; indexed via the output_ids table.
//  2. Extended batches carry both the token ID and pre-norm hidden row for
//     each MTP input without mutating the legacy llama.Batch layout.
//
// On success the returned *mtpDrafter shares the target's llama_model, so
// its unload skips the model free.
func loadDraftModelMTP(ctx context.Context, log applog.Logger, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, nDraft int) (*mtpDrafter, error) {
	params := embeddedMTPContextParams(llama.ContextDefaultParams(), targetCtxParams)

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
		"nSeqMax", params.NSeqMax,
		"nRsSeq", params.NRsSeq,
		"nOutputsMax", params.NOutputsMax,
		"nOutputsMaxPerSeq", params.NOutputsMaxPerSeq)

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

	// Enable NextN hidden-state extraction. Order is:
	//   target  → masked=false (dense, all rows accessible by raw batch idx)
	//   draft   → masked=true  (sparse, only logits-flagged rows)
	//
	// This mirrors common_speculative_impl_draft_mtp (common/speculative.cpp).
	// Must be set BEFORE any decode on either context — the cparams flag
	// is read at graph build time.
	yzmaspec.SetEmbeddingsNextN(targetCtx, true, false)
	yzmaspec.SetEmbeddingsNextN(lctx, true, true)

	// Greedy sampler for the draft (temperature=0 for speed).
	targetVocab := llama.ModelGetVocab(targetModel)
	suppressTokens := copySuppressTokens(targetVocab)
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	addSuppressTokenSampler(sampler, targetVocab, suppressTokens)
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())

	// MTP-specific extended batches carry both a token ID and a pre-norm
	// hidden-state row per entry. Yzma copies each hidden row into llama.cpp's
	// batch storage, so these resources need no pinned Go memory. The draft
	// batch is reused for one autoregressive token at a time; the mirror batch
	// replays target rows into the draft KV in NBatch-sized chunks.
	//
	// Note: draftBuf and targetProbs are intentionally left nil/empty
	// for MTP. Speculative verification
	// forces greedy verification on the MTP path (the MTP head does
	// not produce per-token distributions), so the probabilistic
	// sampling branches that read those buffers are unreachable. The
	// lazy sortIndices / filterBuf scratch buffers stay zero for the
	// same reason. Skipping the full-vocab allocations avoids ~1-2 MB
	// of unused memory per drafter on large-vocab models.

	resources, err := mtpengine.NewResources(lctx, int(params.NBatch), nEmbd)
	if err != nil {
		llama.SamplerFree(sampler)
		llama.Free(lctx)
		return nil, fmt.Errorf("init-mtp-batches: %w", err)
	}

	dm := &draftCore{
		model:          targetModel,
		vocab:          targetVocab,
		suppressTokens: suppressTokens,
		lctx:           lctx,
		mem:            mem,
		sampler:        sampler,
		// batch and prefillBatch stay zero for MTP. MTP code paths use the
		// extended batches in resources, which own their native handles.
		mtp:    resources,
		nDraft: nDraft,
	}

	return &mtpDrafter{c: dm}, nil
}

// embeddedMTPContextParams builds the embedded MTP context to match the
// target's execution and positional-encoding layout while limiting graph
// outputs to one row per sequence. Mirror-only MTP decodes do not request
// logits, and draft steps request at most one output per active sequence.
// Leaving these limits at zero makes llama.cpp reserve NBatch vocabulary
// outputs unnecessarily.
func embeddedMTPContextParams(params, target llama.ContextParams) llama.ContextParams {
	params.CtxType = llama.ContextTypeMTP
	params.NCtx = target.NCtx
	params.NBatch = target.NBatch
	params.NUbatch = target.NUbatch
	params.NSeqMax = target.NSeqMax
	params.NRsSeq = target.NRsSeq
	params.NOutputsMax = target.NSeqMax
	params.NOutputsMaxPerSeq = 1
	params.NThreads = target.NThreads
	params.NThreadsBatch = target.NThreadsBatch
	params.FlashAttentionType = target.FlashAttentionType
	params.TypeK = target.TypeK
	params.TypeV = target.TypeV
	params.Offload_kqv = target.Offload_kqv
	params.OpOffload = target.OpOffload
	params.KVUnified = target.KVUnified
	params.SwaFull = target.SwaFull
	params.RopeScalingType = target.RopeScalingType
	params.RopeFreqBase = target.RopeFreqBase
	params.RopeFreqScale = target.RopeFreqScale
	params.YarnExtFactor = target.YarnExtFactor
	params.YarnAttnFactor = target.YarnAttnFactor
	params.YarnBetaFast = target.YarnBetaFast
	params.YarnBetaSlow = target.YarnBetaSlow
	params.YarnOrigCtx = target.YarnOrigCtx
	return params
}

// probeMTPCompanion reports the supported runtime shape declared by a
// separate-file MTP companion. The first result identifies shared-KV
// assistants such as Gemma4; the second identifies Qwen35 heads that own
// their draft KV. Unsupported architectures return false, false.
func probeMTPCompanion(ctx context.Context, log applog.Logger, file string) (bool, bool) {
	data, err := gguf.ReadHeaderBytes(file)
	if err != nil {
		log(ctx, "draft-model-mtp", "status", "probe-skip", "file", file, "err", err)
		return false, false
	}

	md, err := gguf.ParseMetadata(data)
	if err != nil {
		log(ctx, "draft-model-mtp", "status", "probe-skip", "file", file, "err", err)
		return false, false
	}

	return metadataHasAssistantMTP(md), metadataHasOwnKVCompanionMTP(md)
}

// loadDraftModelMTPSeparate loads a Qwen35 MTP-only GGUF into its own model
// and context. It owns its KV cache and consumes the same target hidden-state
// rows as an embedded Qwen MTP head.
func loadDraftModelMTPSeparate(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, nDraft int) (*separateMTPDrafter, error) {
	cfgCopy := cfg
	mParams, ka, err := buildModelParams(ctx, &cfgCopy, true, log)
	if err != nil {
		return nil, fmt.Errorf("mtp-separate-build-model-params: %w", err)
	}

	log(ctx, "draft-model-mtp-separate", "status", "loading",
		"file", cfg.MTPDrafterFile, "gpu_layers", mParams.NGpuLayers)

	draftModel, err := loadModelFromFiles(ctx, log, []string{cfg.MTPDrafterFile}, mParams)
	runtime.KeepAlive(ka)
	if err != nil {
		return nil, fmt.Errorf("mtp-separate-load-model: %w", err)
	}

	targetVocab := llama.ModelGetVocab(targetModel)
	draftVocab := llama.ModelGetVocab(draftModel)
	if targetTokens, draftTokens := llama.VocabNTokens(targetVocab), llama.VocabNTokens(draftVocab); targetTokens != draftTokens {
		llama.ModelFree(draftModel)
		return nil, fmt.Errorf("mtp-separate vocabulary mismatch: target has %d tokens, draft has %d tokens", targetTokens, draftTokens)
	}

	nEmbd := int(llama.ModelNEmbdOut(targetModel))
	draftNEmbd := int(llama.ModelNEmbdOut(draftModel))
	if nEmbd <= 0 || draftNEmbd != nEmbd {
		llama.ModelFree(draftModel)
		return nil, fmt.Errorf("mtp-separate output embedding width %d does not match target output embedding width %d", draftNEmbd, nEmbd)
	}

	params := embeddedMTPContextParams(llama.ContextDefaultParams(), targetCtxParams)
	params.CtxOther = targetCtx
	lctx, err := llama.InitFromModel(draftModel, params)
	if err != nil {
		llama.ModelFree(draftModel)
		return nil, fmt.Errorf("mtp-separate-init-context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		llama.ModelFree(draftModel)
		return nil, fmt.Errorf("mtp-separate-get-memory: %w", err)
	}
	llama.MemoryClear(mem, true)

	yzmaspec.SetEmbeddingsNextN(targetCtx, true, false)
	yzmaspec.SetEmbeddingsNextN(lctx, true, true)

	suppressTokens := copySuppressTokens(draftVocab)
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	addSuppressTokenSampler(sampler, draftVocab, suppressTokens)
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())
	resources, err := mtpengine.NewResources(lctx, int(params.NBatch), nEmbd)
	if err != nil {
		llama.SamplerFree(sampler)
		llama.Free(lctx)
		llama.ModelFree(draftModel)
		return nil, fmt.Errorf("mtp-separate-init-batches: %w", err)
	}

	dm := &draftCore{
		model:          draftModel,
		vocab:          targetVocab,
		suppressTokens: suppressTokens,
		lctx:           lctx,
		mem:            mem,
		sampler:        sampler,
		mtp:            resources,
		nDraft:         nDraft,
	}

	return &separateMTPDrafter{c: dm}, nil
}

// loadDraftModelMTPShared loads a separate-file MTP assistant (Gemma4
// gemma4-assistant) from cfg.MTPDrafterFile as its OWN llama_model, then
// creates its context with ctx_other==targetCtx so it SHARES the target's
// llama_memory. This is the is_mem_shared path (common/speculative.cpp):
// the target's decode populates the shared KV directly and the assistant
// reads it, so there is no separate draft KV to mirror into or roll back.
//
// Distinguishing plumbing vs. a normal draft context:
//
//  1. params.CtxOther = targetCtx — REQUIRED; the assistant graph pulls the
//     target's token embeddings through ctx_other and shares its memory.
//  2. params.CtxType = MTP and speculative.SetEmbeddingsNextN enable pre-norm
//     hidden-state extraction (target dense, draft masked), exactly as the
//     embedded-MTP path.
//  3. Each AR extended-batch entry carries a hidden row with the TARGET's
//     embedding width (== ModelNEmbdOut(assistant)), the row width the MTP
//     head consumes.
//  4. The shared memory is NOT cleared here — clearing it would wipe the
//     TARGET's KV.
//
// On success the returned *sharedMTPDrafter owns the assistant llama_model,
// so its unload frees it.
func loadDraftModelMTPShared(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, nDraft int) (*sharedMTPDrafter, error) {

	// Load the assistant GGUF with the same hardware placement as the
	// target (NOT a DraftModelConfig — there is no user knob). buildModelParams
	// mutates the cfg copy; pass a copy so the live config is untouched.
	cfgCopy := cfg
	mParams, ka, err := buildModelParams(ctx, &cfgCopy, false, log)
	if err != nil {
		return nil, fmt.Errorf("mtp-shared-build-model-params: %w", err)
	}

	log(ctx, "draft-model-mtp-shared", "status", "loading-assistant",
		"file", cfg.MTPDrafterFile, "gpu_layers", mParams.NGpuLayers)

	asstModel, err := loadModelFromFiles(ctx, log, []string{cfg.MTPDrafterFile}, mParams)
	runtime.KeepAlive(ka)
	if err != nil {
		return nil, fmt.Errorf("mtp-shared-load-assistant: %w", err)
	}

	// The MTP head consumes rows of the TARGET's hidden width. llama.cpp
	// asserts n_embd_out(assistant) == n_embd(target); enforce the same
	// here so a mismatched companion fails loudly instead of corrupting.
	nEmbd := int(llama.ModelNEmbd(targetModel))
	nEmbdOut := int(llama.ModelNEmbdOut(asstModel))
	if nEmbd <= 0 {
		llama.ModelFree(asstModel)
		return nil, fmt.Errorf("mtp-shared: invalid target nEmbd %d", nEmbd)
	}
	if nEmbdOut != nEmbd {
		llama.ModelFree(asstModel)
		return nil, fmt.Errorf("mtp-shared: assistant n_embd_out %d != target n_embd %d", nEmbdOut, nEmbd)
	}

	// Context params: shared memory with the target via CtxOther, MTP type,
	// inheriting thread layout, KV cache types, offload, and the sequence /
	// batch dimensions from the target.
	params := llama.ContextDefaultParams()
	params.CtxType = llama.ContextTypeMTP
	params.CtxOther = targetCtx
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

	// KVUnified and SwaFull define the shared cache topology. They MUST match
	// the target: the assistant borrows the target's KV tensors (CtxOther), so
	// a stream-layout mismatch makes llama_kv_cache::get_k compute a 4-D view
	// that overruns the shared tensor and trips ggml_view_4d's bounds assert.
	params.KVUnified = targetCtxParams.KVUnified
	params.SwaFull = targetCtxParams.SwaFull

	log(ctx, "draft-model-mtp-shared", "status", "init-context",
		"nDraft", nDraft, "nEmbd", nEmbd,
		"nCtx", params.NCtx, "nBatch", params.NBatch,
		"nUbatch", params.NUbatch, "nSeqMax", params.NSeqMax)

	lctx, err := llama.InitFromModel(asstModel, params)
	if err != nil {
		llama.ModelFree(asstModel)
		return nil, fmt.Errorf("mtp-shared-init-context: %w", err)
	}

	// Shared memory: this returns the TARGET's llama_memory. Do NOT clear
	// or trim it through the assistant — that would mutate the target's KV.
	// The handle is retained only for draftCore's uniform ownership shape.
	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		llama.ModelFree(asstModel)
		return nil, fmt.Errorf("mtp-shared-get-memory: %w", err)
	}

	// Enable NextN hidden-state extraction: target dense (all rows),
	// draft masked (only logits-flagged rows). Must be set before any
	// decode on either context.
	yzmaspec.SetEmbeddingsNextN(targetCtx, true, false)
	yzmaspec.SetEmbeddingsNextN(lctx, true, true)

	// Greedy sampler (temperature=0) — matches the embedded-MTP hot path;
	// MTP verification is greedy.
	assistantVocab := llama.ModelGetVocab(asstModel)
	suppressTokens := copySuppressTokens(assistantVocab)
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	addSuppressTokenSampler(sampler, assistantVocab, suppressTokens)
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())
	resources, err := mtpengine.NewResources(lctx, 0, nEmbd)
	if err != nil {
		llama.SamplerFree(sampler)
		llama.Free(lctx)
		llama.ModelFree(asstModel)
		return nil, fmt.Errorf("mtp-shared-init-batches: %w", err)
	}

	// Shared-KV needs only the AR draft batch (capacity 1) — there is no
	// mirror replay, so Resources does not allocate a mirror batch.
	dm := &draftCore{
		model:          asstModel,
		vocab:          llama.ModelGetVocab(targetModel),
		suppressTokens: suppressTokens,
		lctx:           lctx,
		mem:            mem,
		sampler:        sampler,
		mtp:            resources,
		nDraft:         nDraft,
	}

	return &sharedMTPDrafter{c: dm}, nil
}

// selectAndLoadDraft chooses the broad speculation source. MTP loading is
// delegated to the architecture backend selected by the immutable plan.
// It returns (nil, nil) when no drafter applies. Selection priority is:
//
//  1. Explicit separate-draft GGUF (cfg.PtrDraftModel) — user override,
//     vocab-matched classic draft.
//  2. Separate-file MTP assistant (cfg.MTPDrafterFile, Gemma4
//     gemma4-assistant): a per-model speculative head that ships alongside
//     the main GGUF and shares the target's KV memory (ctx_other==target).
//  3. Separate-file Qwen35 MTP head: owns its model and draft KV.
//  4. Auto-detect embedded MTP: enable when the target GGUF itself carries
//     an MTP head (nextn_predict_layers > 0).
//
// targetCtx is needed because MTP requires
// llama_set_embeddings_pre_norm(target, true, false) before the next
// target decode — the MTP loaders handle that call internally.
//
// The caller is responsible for cleanup on error; this function only
// owns resources it returns successfully.
func selectAndLoadDraft(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, plan speculationPlan) (drafter, error) {
	switch plan.Source {
	case speculationSourceNone:
		return nil, nil

	case speculationSourceClassic:
		d, err := loadDraftModel(ctx, log, cfg, targetModel, targetCtxParams)
		if err != nil {
			return nil, err
		}
		log(ctx, "draft-model", "status", "loaded",
			"source", "explicit-separate",
			"nDraft", d.c.nDraft, "devices", cfg.PtrDraftModel.Devices,
			"nCtx", llama.NCtx(d.c.lctx))
		return d, nil

	case speculationSourceMTP:
		backend, err := mtpBackendForPlan(plan)
		if err != nil {
			return nil, err
		}
		return backend.load(mtpLoadRequest{
			ctx:             ctx,
			log:             log,
			cfg:             cfg,
			targetCtx:       targetCtx,
			targetModel:     targetModel,
			targetCtxParams: targetCtxParams,
		})
	}

	return nil, fmt.Errorf("unsupported speculation source %d", plan.Source)
}
