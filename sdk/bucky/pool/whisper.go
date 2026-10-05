// This file provides the whisper-backed loader.Loader implementation
// that plugs the bucky / whisper.cpp runtime into the generic pool
// core. It owns the model.bin path resolution against the whisper
// catalog and the construction of a *bucky.Bucky handle. The pool
// core invokes it for every load/unload/display operation, leaving
// the cache, eviction, and budget logic entirely backend-agnostic in
// sdk/pool/core.

package pool

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// whisperOverhead is the additional resident memory we reserve on top
// of the raw model file size to account for the encoder + decoder
// activations and the small whisper.cpp compute buffer. The figure
// is conservative for every model up through large-v3.
const whisperOverhead int64 = 200 * 1000 * 1000

// Whisper is the loader.Loader[*bucky.Bucky] implementation for the
// whisper.cpp backend. It is constructed by sdk/pool and any future
// programs that want to build a pool around whisper models manually.
type Whisper struct {
	log         applog.Logger
	models      *models.Models
	modelConfig map[string]modelconfig.BuckyModelConfig
	resman      *resman.Manager
}

// newWhisper constructs a whisper loader.
func newWhisper(log applog.Logger, mdls *models.Models, modelCfg map[string]modelconfig.BuckyModelConfig, rm *resman.Manager) *Whisper {
	w := Whisper{
		log:         log,
		models:      mdls,
		modelConfig: modelCfg,
		resman:      rm,
	}
	return &w
}

// Models returns the underlying models system. Pool wrappers expose
// this for catalog-flavored APIs.
func (w *Whisper) Models() *models.Models {
	return w.models
}

// Prepare resolves the model configuration once for planning and loading.
func (w *Whisper) Prepare(_ context.Context, req loader.LoadRequest) (any, error) {
	return w.resolveConfig(req)
}

// Plan implements loader.Loader.Plan for the whisper backend.
//
// Whisper has no slots or KV cache: the resident footprint is the
// weight file plus a small encoder/decoder overhead. The estimate is
// charged to VRAM when the resman has GPUs (Metal counts as GPU even
// on unified-memory devices, so the entire footprint lands on the
// GPU bucket on Apple Silicon) and to system RAM otherwise.
func (w *Whisper) Plan(ctx context.Context, req loader.LoadRequest) (resman.PlanRequest, error) {
	cfg, err := w.configForRequest(req)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: %w", err)
	}

	size, err := w.modelSize(req.ModelID)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: %w", err)
	}

	planReq := resman.PlanRequest{
		Key: req.Key,
	}

	total := size + whisperOverhead
	if w.resman.HasGPUs() {
		planReq.VRAMBytes = total
	} else {
		planReq.RAMBytes = total
	}

	w.log(ctx, "bucky-plan-request",
		"key", req.Key,
		"model-id", req.ModelID,
		"predicted-total", total,
		"model-size", size,
		"overhead", whisperOverhead,
		"n-seq-max", cfg.NSeqMax,
		"vram", planReq.VRAMBytes,
		"ram", planReq.RAMBytes,
	)

	return planReq, nil
}

// Load implements loader.Loader.Load for the whisper backend.
func (w *Whisper) Load(ctx context.Context, req loader.LoadRequest) (*bucky.Bucky, error) {
	cfg, err := w.configForRequest(req)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	cfg.Log = w.log

	handle, err := bucky.NewWithContext(ctx, model.WithConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("load: unable to create whisper handle: %w", err)
	}

	mi := handle.ModelInfo()

	w.log(ctx, "bucky-load",
		"status", "load new model",
		"model-name", req.ModelID,
		"model-type", mi.Type,
		"multilingual", mi.IsMultilingual,
		"model-path", cfg.ModelPath,
	)

	return handle, nil
}

// Display implements loader.Loader.Display for the whisper backend.
//
// Whisper does not expose a distinct KV-cache measurement. Slots reports the
// configured state count, and VRAMTotal uses the same estimate as Plan.
func (w *Whisper) Display(h *bucky.Bucky, modelID string) loader.Display {
	out := loader.Display{
		Slots: h.ModelConfig().NSeqMax,
	}

	if size, err := w.modelSize(modelID); err == nil {
		out.VRAMTotal = size + whisperOverhead
	}

	return out
}

// =============================================================================

// resolveConfig produces a model.Config for the request. When the
// caller has supplied a pre-built config via req.Custom it is used
// as-is. Otherwise the catalog is consulted to resolve the model
// file path and a default Config is constructed around it.
func (w *Whisper) resolveConfig(req loader.LoadRequest) (model.Config, error) {
	if req.Custom != nil {
		cfg, ok := req.Custom.(model.Config)
		if !ok {
			return model.Config{}, fmt.Errorf("resolve-config: custom config is %T, want model.Config", req.Custom)
		}
		return cfg.WithDefaults(), nil
	}

	path, err := w.models.FullPath(req.ModelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: full-path: %w", err)
	}
	if len(path.ModelFiles) == 0 {
		return model.Config{}, fmt.Errorf("resolve-config: model-id[%s]: no model files on disk", req.ModelID)
	}

	cfg := model.Config{
		ModelPath: path.ModelFiles[0],
		UseGPU:    true,
	}
	if override, ok := w.modelConfig[req.ModelID]; ok {
		if override.NSeqMax != nil {
			cfg.NSeqMax = *override.NSeqMax
		}
		if override.QueueDepth != nil {
			cfg.QueueDepth = *override.QueueDepth
		}
		if override.AdmissionTimeout != nil {
			cfg.AdmissionTimeout = time.Duration(*override.AdmissionTimeout)
		}
		if override.NThreads != nil {
			cfg.NThreads = *override.NThreads
		}
	}

	return cfg.WithDefaults(), nil
}

func (w *Whisper) configForRequest(req loader.LoadRequest) (model.Config, error) {
	if req.Prepared == nil {
		return w.resolveConfig(req)
	}

	cfg, ok := req.Prepared.(model.Config)
	if !ok {
		return model.Config{}, fmt.Errorf("prepared config is %T, want model.Config", req.Prepared)
	}

	return cfg, nil
}

// modelSize returns the on-disk size of the resolved whisper model
// in bytes. The first listed file size is used; whisper models are
// always single-file.
func (w *Whisper) modelSize(modelID string) (int64, error) {
	path, err := w.models.FullPath(modelID)
	if err != nil {
		return 0, fmt.Errorf("model-size: %w", err)
	}
	if len(path.FileSizes) == 0 || path.FileSizes[0] <= 0 {
		return 0, fmt.Errorf("model-size: model-id[%s]: missing file size", modelID)
	}
	return path.FileSizes[0], nil
}
