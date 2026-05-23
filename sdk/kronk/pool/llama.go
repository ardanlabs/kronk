// This file provides the llama-backed loader.Loader implementation
// that plugs the llama runtime (sdk/kronk + yzma) into the generic
// pool core. It owns the GGUF-driven memory prediction, model.Config
// resolution, and *kronk.Kronk construction. The pool core invokes
// it for every load/unload/display operation, leaving the cache,
// eviction, and budget logic entirely backend-agnostic in
// sdk/pool/core.

package pool

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Llama is the loader.Loader[*kronk.Kronk] implementation for the
// llama.cpp backend. It is constructed by sdk/pool and any future
// programs that want to build a pool around llama models manually.
type Llama struct {
	log             applog.Logger
	models          *models.Models
	modelConfig     map[string]models.ModelConfig
	resman          *resman.Manager
	insecureLogging bool
}

// newLlama constructs a llama loader.
//
// modelConfig may be nil; an empty map will be used.
func newLlama(log applog.Logger, mdls *models.Models, modelConfig map[string]models.ModelConfig, rm *resman.Manager, insecureLogging bool) *Llama {
	if modelConfig == nil {
		modelConfig = map[string]models.ModelConfig{}
	}

	l := Llama{
		log:             log,
		models:          mdls,
		modelConfig:     modelConfig,
		resman:          rm,
		insecureLogging: insecureLogging,
	}

	return &l
}

// Models returns the underlying models system. Pool wrappers expose
// this for catalog-flavored APIs (ModelStatus, ModelConfig lookup).
func (l *Llama) Models() *models.Models {
	return l.models
}

// ModelConfig returns the loaded per-model configuration overrides.
func (l *Llama) ModelConfig() map[string]models.ModelConfig {
	return l.modelConfig
}

// Plan implements loader.Loader.Plan for the llama backend.
//
// It charges the predicted VRAM and system-RAM footprints to the resman
// independently so MoE models — whose routed experts can live on either
// side depending on the runtime placement — are accounted for
// accurately. Charging only the GPU side silently drops the
// CPU-resident expert weights, producing under-counts of the real
// resident footprint and exposing the pool to OOM on multi-load
// scenarios.
func (l *Llama) Plan(ctx context.Context, req loader.LoadRequest) (resman.PlanRequest, error) {
	cfg, err := l.resolveConfig(req)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: %w", err)
	}

	bpe := bytesPerElement(cfg.CacheTypeK, cfg.CacheTypeV)

	ctxWin := int64(cfg.ContextWindow())
	if ctxWin <= 0 {
		ctxWin = int64(vram.ContextWindow4K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:     ctxWin,
		BytesPerElement:   bpe,
		Slots:             nseq,
		ExpertLayersOnGPU: cfg.ExpertLayersOnGPU(),
	}

	result, source, err := predictResult(l.models, req.ModelID, vramCfg)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: modelID[%s]: %w", req.ModelID, err)
	}

	planReq := resman.PlanRequest{
		Key:         req.Key,
		Devices:     gpuDevices(cfg.Devices),
		TensorSplit: cfg.TensorSplit,
	}

	// Map the calculator's GPU/CPU split onto resman buckets.
	//
	// Unified memory (Apple Silicon Metal) is special-cased first.
	// The GPU and CPU share one physical pool, and llama.cpp mmaps
	// the GGUF — so even an MoE model with "experts on CPU" will
	// eventually have every page resident in the same shared pool
	// once exercised. Charging only the planner's TotalVRAM (which
	// drops the always-inactive expert weights) would let the
	// resman admit far more concurrent models than the box can
	// actually hold, then OOM when the experts page in. We instead
	// charge the full loaded footprint:
	//
	//   model_bytes + KV cache + compute buffer
	//
	// to the system RAM bucket.
	//
	// Discrete-GPU systems keep the existing MoE-aware split so
	// expert-offload-to-CPU and per-GPU tensor splits are still
	// accounted for accurately.
	switch {
	case l.resman.UnifiedMemory():
		planReq.VRAMBytes = 0
		planReq.RAMBytes = result.UnifiedFootprint()
	case l.resman.HasGPUs() && cfg.NGpuLayers() != -1:
		planReq.VRAMBytes = result.TotalVRAM
		planReq.RAMBytes = result.TotalSystemRAMEst
	default:
		planReq.VRAMBytes = 0
		planReq.RAMBytes = result.TotalVRAM + result.TotalSystemRAMEst
	}

	l.log(ctx, "plan-request",
		"key", req.Key,
		"model-id", req.ModelID,
		"source", source,
		"predicted-total", humanBytes(planReq.VRAMBytes+planReq.RAMBytes),
		"predicted-vram", humanBytes(result.TotalVRAM),
		"predicted-system", humanBytes(result.TotalSystemRAMEst),
		"context-window", ctxWin,
		"slots", nseq,
		"bytes-per-element", bpe,
		"experts-on-gpu", vramCfg.ExpertLayersOnGPU,
		"vram", humanBytes(planReq.VRAMBytes),
		"ram", humanBytes(planReq.RAMBytes),
		"devices", planReq.Devices,
		"tensor-split", planReq.TensorSplit,
	)

	return planReq, nil
}

// Load implements loader.Loader.Load for the llama backend.
func (l *Llama) Load(ctx context.Context, req loader.LoadRequest) (*kronk.Kronk, error) {
	cfg, err := l.resolveConfig(req)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	if l.insecureLogging {
		cfg.PtrInsecureLogging = new(true)
	}

	cfg.Log = l.log

	krn, err := kronk.NewWithContext(ctx, model.WithConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("load: unable to create inference model: %w", err)
	}

	totalEntries := len(krn.SystemInfo())*2 + (5 * 2)
	info := make([]any, 0, totalEntries)
	for k, v := range krn.SystemInfo() {
		info = append(info, k)
		info = append(info, v)
	}

	info = append(info, "status")
	info = append(info, "load new model")
	info = append(info, "model-name")
	info = append(info, req.ModelID)
	info = append(info, "contextWindow")
	info = append(info, krn.ModelConfig().ContextWindow())
	info = append(info, "isGPTModel")
	info = append(info, krn.ModelInfo().IsGPTModel)
	info = append(info, "isEmbedModel")
	info = append(info, krn.ModelInfo().IsEmbedModel)
	info = append(info, "isRerankModel")
	info = append(info, krn.ModelInfo().IsRerankModel)

	l.log(ctx, "load", info...)

	return krn, nil
}

// Display implements loader.Loader.Display for the llama backend.
//
// It returns the KV cache and total VRAM values to surface in
// BUI/observability output for a loaded model. Both this path and the
// SDK-internal calculateVRAMDiag route through vram.FromFiles, so the
// two computations are byte-identical for any well-formed local model.
// The dedicated lookup is retained so a hypothetical resman-side
// failure (e.g. an index miss) cleanly falls back to the values the
// SDK stored at load time rather than zeroing out the BUI display.
func (l *Llama) Display(krn *kronk.Kronk, modelID string) loader.Display {
	cfg := krn.ModelConfig()
	mi := krn.ModelInfo()

	ctxWin := int64(cfg.ContextWindow())
	if ctxWin <= 0 {
		ctxWin = int64(vram.ContextWindow4K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:     ctxWin,
		BytesPerElement:   bytesPerElement(cfg.CacheTypeK, cfg.CacheTypeV),
		Slots:             nseq,
		ExpertLayersOnGPU: cfg.ExpertLayersOnGPU(),
	}

	out := loader.Display{
		Slots: max(int(cfg.NSeqMax()), 1),
	}

	if v, err := l.models.CalculateVRAM(modelID, vramCfg); err == nil {
		out.KVCache = v.SlotMemory
		if l.resman.UnifiedMemory() {
			out.VRAMTotal = v.UnifiedFootprint()
		} else {
			out.VRAMTotal = v.TotalVRAM
		}
		return out
	}

	out.KVCache = mi.SlotMemory
	out.VRAMTotal = mi.VRAMTotal
	return out
}

// =============================================================================

// resolveConfig produces a model.Config for the request. When the
// caller has supplied a pre-built config via req.Custom (the
// playground path) it is used as-is. Otherwise the catalog resolver is
// consulted with the per-model overrides Llama was constructed with.
func (l *Llama) resolveConfig(req loader.LoadRequest) (model.Config, error) {
	if req.Custom != nil {
		cfg, ok := req.Custom.(model.Config)
		if !ok {
			return model.Config{}, fmt.Errorf("resolve-config: custom config is %T, want model.Config", req.Custom)
		}
		return cfg, nil
	}

	cfg, err := l.models.KronkResolvedConfig(req.ModelID, l.modelConfig)
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: unable to retrieve model config: %w", err)
	}

	return cfg, nil
}

// predictResult returns the full VRAM calculator result for a given
// model along with a source label identifying which estimator produced
// it.
//
// "calculate-vram" is the preferred path: it understands KV cache,
// compute buffer, and MoE expert placement for standard transformer
// architectures.
//
// "file-size" is the fallback used when the model's metadata is
// missing the keys that the calculator needs (e.g. BERT-based
// rerankers and embedders). The raw on-disk size is returned in
// TotalVRAM so the caller's bucket-mapping logic still gates
// concurrent loads, even though the breakdown is unavailable.
func predictResult(m *models.Models, modelID string, cfg vram.Config) (vram.Result, string, error) {
	if v, err := m.CalculateVRAM(modelID, cfg); err == nil {
		return v, "calculate-vram", nil
	}

	info, err := m.ModelInformation(modelID)
	if err != nil {
		return vram.Result{}, "", fmt.Errorf("predict-result: model-information: %w", err)
	}
	return vram.Result{TotalVRAM: int64(info.Size)}, "file-size", nil
}

// bytesPerElement returns the per-element width to use for KV-cache
// budgeting given the K and V cache types. When either type is unset
// (GGMLTypeAuto) F16 is assumed, mirroring llama.cpp's default. The
// max of K and V is used so a budget never undercounts the heavier
// half.
func bytesPerElement(k, v model.GGMLType) int64 {
	return int64(gguf.MaxBytesPerElement(int32(k), int32(v)))
}

// gpuDevices filters out a "CPU" entry that some configs leave
// alongside real GPU device names. resman only tracks GPUs so we drop
// CPU here.
func gpuDevices(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, d := range in {
		if strings.EqualFold(d, "CPU") {
			continue
		}
		out = append(out, d)
	}
	return out
}

// humanBytes is a local copy of core.HumanBytes used in plan logging.
// Duplicated to avoid pulling the internal core package into the
// loader's import graph.
func humanBytes(n int64) string {
	return formatBytes(n)
}

func formatBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.1f%s", float64(n)/float64(div), suffixes[exp])
}
