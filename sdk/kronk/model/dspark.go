package model

import (
	"context"
	"fmt"
	"runtime"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	dsparkengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/dspark"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// dsparkDrafter exposes borrowed handles and checkpoints independent KV together
// with its accepted-prefix Markov state through the shared IMC contract.
type dsparkDrafter struct {
	c   *draftCore
	r   *dsparkengine.Resources
	log applog.Logger
}

func (*dsparkDrafter) sealedDrafter()              {}
func (*dsparkDrafter) kind() draftKind             { return draftDSpark }
func (*dsparkDrafter) mtp() bool                   { return false }
func (d *dsparkDrafter) core() *draftCore          { return d.c }
func (d *dsparkDrafter) draftKVCtx() llama.Context { return d.r.Context }
func (d *dsparkDrafter) draftCheckpointState(s *slot, position llama.Pos) []float32 {
	return d.r.CheckpointState(s.seqID, position)
}
func (d *dsparkDrafter) restoreDraftCheckpoint(s *slot, position llama.Pos, state []float32) bool {
	return d.r.RestoreCheckpoint(s.seqID, position, state)
}
func (d *dsparkDrafter) clearDraftCheckpoint(s *slot) {
	d.r.ClearCheckpoint(s.seqID)
}
func (d *dsparkDrafter) unload() {
	if err := d.r.Free(); err != nil {
		d.log(context.Background(), "draft-model-dspark", "status", "cleanup-error", "err", err)
	}
}

func loadDSpark(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetParams llama.ContextParams, plan internalspec.Plan) (drafter, error) {
	var metadata [2]map[string]string
	var tensors []gguf.TensorInfo
	for i, file := range []string{cfg.ModelFiles[0], cfg.DrafterFile} {
		data, err := gguf.ReadHeaderBytes(file)
		if err != nil {
			return nil, fmt.Errorf("DSpark header %q: %w", file, err)
		}
		metadata[i], err = gguf.ParseMetadata(data)
		if err != nil {
			return nil, fmt.Errorf("DSpark metadata %q: %w", file, err)
		}
		if i == 1 {
			_, tensors, err = gguf.ParseHeaderAndTensors(data, 0)
			if err != nil {
				return nil, fmt.Errorf("DSpark tensors %q: %w", file, err)
			}
		}
	}
	r, err := dsparkengine.Load(dsparkengine.LoadRequest{
		TargetContext: targetCtx, TargetModel: targetModel, TargetParams: targetParams,
		TargetMetadata: metadata[0], DraftMetadata: metadata[1], DraftTensors: tensors,
		NDraft: plan.NDraft,
		LoadCompanion: func() (llama.Model, error) {
			cfgCopy := cfg
			params, ka, err := buildModelParams(ctx, &cfgCopy, false, log)
			if err != nil {
				return 0, fmt.Errorf("DSpark model parameters: %w", err)
			}
			model, err := loadModelFromFiles(ctx, log, []string{cfg.DrafterFile}, params)
			runtime.KeepAlive(ka)
			return model, err
		},
	})
	if err != nil {
		return nil, err
	}
	d := dsparkDrafter{
		c: &draftCore{model: r.Model, vocab: llama.ModelGetVocab(r.Model), lctx: r.Context, mem: r.Memory, nDraft: plan.NDraft},
		r: r, log: log,
	}
	log(ctx, "draft-model-dspark", "status", "loaded", "file", cfg.DrafterFile,
		"nDraft", plan.NDraft, "nCtx", llama.NCtx(r.Context), "execution", "text")
	return &d, nil
}

var _ draftKVExternalizer = (*dsparkDrafter)(nil)
