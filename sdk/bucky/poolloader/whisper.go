// Package poolloader provides the whisper-backed loader.Loader
// implementation that plugs the bucky / whisper.cpp runtime into the
// generic pool core.
//
// It owns the model.bin path resolution against the whisper catalog
// and the construction of a *bucky.Whisper handle. The pool core
// invokes it for every load/unload/display operation, leaving the
// cache, eviction, and budget logic entirely backend-agnostic in
// sdk/pool/internal/core.
package poolloader

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/pool/loader"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// whisperOverhead is the additional resident memory we reserve on top
// of the raw model file size to account for the encoder + decoder
// activations and the small whisper.cpp compute buffer. The figure
// is conservative for every model up through large-v3.
const whisperOverhead int64 = 200 * 1000 * 1000

// Whisper is the loader.Loader[*bucky.Whisper] implementation for the
// whisper.cpp backend. It is constructed by sdk/pool and any future
// programs that want to build a pool around whisper models manually.
type Whisper struct {
	log    applog.Logger
	models *models.Models
	resman *resman.Manager
}

// New constructs a whisper loader.
func New(log applog.Logger, mdls *models.Models, rm *resman.Manager) *Whisper {
	w := Whisper{
		log:    log,
		models: mdls,
		resman: rm,
	}
	return &w
}

// Models returns the underlying models system. Pool wrappers expose
// this for catalog-flavored APIs.
func (w *Whisper) Models() *models.Models {
	return w.models
}

// Plan implements loader.Loader.Plan for the whisper backend.
//
// Whisper has no slots or KV cache: the resident footprint is the
// weight file plus a small encoder/decoder overhead. The estimate is
// charged to VRAM when the resman has GPUs (Metal counts as GPU even
// on unified-memory devices, so the entire footprint lands on the
// GPU bucket on Apple Silicon) and to system RAM otherwise.
func (w *Whisper) Plan(ctx context.Context, req loader.LoadRequest) (resman.PlanRequest, error) {
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
		"vram", planReq.VRAMBytes,
		"ram", planReq.RAMBytes,
	)

	return planReq, nil
}

// Load implements loader.Loader.Load for the whisper backend.
func (w *Whisper) Load(ctx context.Context, req loader.LoadRequest) (*bucky.Whisper, error) {
	cfg, err := w.resolveConfig(req)
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
// Whisper does not maintain a KV cache and serves one transcribe at a
// time per handle, so KVCache is zero and Slots is one. VRAMTotal is
// the file size plus the same overhead Plan used so the observability
// figure tracks the budget reservation.
func (w *Whisper) Display(h *bucky.Whisper, modelID string) loader.Display {
	_ = h

	out := loader.Display{
		Slots: 1,
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
		return cfg, nil
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
