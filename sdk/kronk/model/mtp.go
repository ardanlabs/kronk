package model

import (
	"context"
	"fmt"
	"runtime"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// mtpAdapter exposes borrowed native handles to the model engine. Internal MTP
// owns their lifetime; this adapter must never call draftCore.freeCommon.
type mtpAdapter struct {
	c *draftCore
	b mtpengine.Backend
	r *mtpengine.Resources
}

func (*mtpAdapter) sealedDrafter()                       {}
func (*mtpAdapter) mtp() bool                            { return true }
func (d *mtpAdapter) core() *draftCore                   { return d.c }
func (d *mtpAdapter) mtpBackend() mtpengine.Backend      { return d.b }
func (d *mtpAdapter) mtpResources() *mtpengine.Resources { return d.r }
func (d *mtpAdapter) unload()                            { d.r.Free() }

func (d *mtpAdapter) draftCheckpointState(s *slot, position llama.Pos) []float32 {
	if s.draftNPast != position {
		return nil
	}
	return s.mtp.CheckpointState(d.r.EmbeddingSize())
}

func (d *mtpAdapter) restoreDraftCheckpoint(s *slot, position llama.Pos, state []float32) bool {
	if !s.mtp.RestoreCheckpoint(state, d.r.EmbeddingSize()) {
		return false
	}
	s.draftNPast = position
	return true
}

func (*mtpAdapter) clearDraftCheckpoint(s *slot) {
	s.draftNPast = 0
	s.mtp.Disable("imc-hit")
}

// mtpDrafter borrows the target model and owns a separate draft KV cache.
type mtpDrafter struct{ mtpAdapter }

func (*mtpDrafter) kind() draftKind             { return draftMTPQwen }
func (d *mtpDrafter) draftKVCtx() llama.Context { return d.c.lctx }

// separateMTPDrafter owns a companion model and its separate draft KV cache.
type separateMTPDrafter struct{ mtpAdapter }

func (*separateMTPDrafter) kind() draftKind             { return draftMTPQwenSeparate }
func (d *separateMTPDrafter) draftKVCtx() llama.Context { return d.c.lctx }

// sharedMTPDrafter owns its assistant model but aliases target KV. It must not
// implement draftKVExternalizer: restoring its memory would overwrite target KV.
type sharedMTPDrafter struct{ mtpAdapter }

func (*sharedMTPDrafter) kind() draftKind { return draftMTPGemma4 }

type mtpBackendDrafter interface {
	drafter
	mtpBackend() mtpengine.Backend
	mtpResources() *mtpengine.Resources
}

func mtpResources(d drafter) *mtpengine.Resources {
	return d.(mtpBackendDrafter).mtpResources()
}

func mtpUsesSharedKV(d drafter) bool {
	backend, ok := d.(mtpBackendDrafter)
	return ok && backend.mtpBackend().SharedKV()
}

func mtpUsesFixedDraftPosition(d drafter) bool {
	backend, ok := d.(mtpBackendDrafter)
	return ok && backend.mtpBackend().FixedDraftPosition()
}

// loadMTP supplies common placement/loading and suppression policy, then wraps
// the internal owner with the model engine's sealed KV capabilities.
func loadMTP(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetParams llama.ContextParams, plan internalspec.Plan) (drafter, error) {
	backend, err := mtpengine.BackendForPlan(plan)
	if err != nil {
		return nil, err
	}
	r, err := backend.Load(ctx, log, mtpengine.LoadRequest{
		TargetContext: targetCtx,
		TargetModel:   targetModel,
		TargetParams:  targetParams,
		NDraft:        mtpNDraft(cfg),
		Configured:    cfg.PtrDraftModel != nil && !cfg.PtrDraftModel.IsSeparate(),
		DrafterFile:   cfg.DrafterFile,
		LoadCompanion: func(sharedKV bool) (llama.Model, error) {
			prefix, event, status, loadName := "mtp-separate", "draft-model-mtp-separate", "loading", "load-model"
			if sharedKV {
				prefix, event, status, loadName = "mtp-shared", "draft-model-mtp-shared", "loading-assistant", "load-assistant"
			}
			cfgCopy := cfg
			params, ka, err := buildModelParams(ctx, &cfgCopy, !sharedKV, log)
			if err != nil {
				return 0, fmt.Errorf("%s-build-model-params: %w", prefix, err)
			}
			log(ctx, event, "status", status, "file", cfg.DrafterFile, "gpu_layers", params.NGpuLayers)
			model, err := loadModelFromFiles(ctx, log, []string{cfg.DrafterFile}, params)
			runtime.KeepAlive(ka)
			if err != nil {
				return 0, fmt.Errorf("%s-%s: %w", prefix, loadName, err)
			}
			return model, nil
		},
		NewSampler: func(vocab llama.Vocab) (llama.Sampler, []llama.Token) {
			tokens := copySuppressTokens(vocab)
			sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
			addSuppressTokenSampler(sampler, vocab, tokens)
			llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())
			return sampler, tokens
		},
	})
	if err != nil || r == nil {
		return nil, err
	}
	adapter := mtpAdapter{
		c: &draftCore{
			model: r.Model, vocab: r.Vocab, suppressTokens: r.SuppressTokens,
			lctx: r.Context, mem: r.Memory, sampler: r.Sampler, nDraft: mtpNDraft(cfg),
		},
		b: backend,
		r: r,
	}
	switch {
	case backend.SharedKV():
		return &sharedMTPDrafter{mtpAdapter: adapter}, nil
	case plan.MTPArtifact == mtpArtifactCompanion:
		return &separateMTPDrafter{mtpAdapter: adapter}, nil
	default:
		return &mtpDrafter{mtpAdapter: adapter}, nil
	}
}

var (
	_ mtpBackendDrafter   = (*mtpDrafter)(nil)
	_ mtpBackendDrafter   = (*separateMTPDrafter)(nil)
	_ mtpBackendDrafter   = (*sharedMTPDrafter)(nil)
	_ draftKVExternalizer = (*mtpDrafter)(nil)
	_ draftKVExternalizer = (*separateMTPDrafter)(nil)
)
