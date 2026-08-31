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
	"math"
	"os"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Llama is the loader.Loader[*kronk.Kronk] implementation for the
// llama.cpp backend. It is constructed by sdk/pool and any future
// programs that want to build a pool around llama models manually.
type Llama struct {
	log             applog.Logger
	models          *models.Models
	modelConfig     map[string]models.ModelConfig
	resman          *resman.Manager
	startupDevices  devices.Devices
	insecureLogging bool
}

// newLlama constructs a llama loader.
//
// modelConfig may be nil; an empty map will be used.
func newLlama(log applog.Logger, mdls *models.Models, modelConfig map[string]models.ModelConfig, rm *resman.Manager, startupDevices devices.Devices, insecureLogging bool) *Llama {
	if modelConfig == nil {
		modelConfig = map[string]models.ModelConfig{}
	}

	l := Llama{
		log:             log,
		models:          mdls,
		modelConfig:     modelConfig,
		resman:          rm,
		startupDevices:  startupDevices,
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

// ResolvedModelConfig returns the same budgeted configuration used to prepare
// a model for planning and loading.
func (l *Llama) ResolvedModelConfig(modelID string) (models.ModelConfig, error) {
	return l.models.ResolvedModelConfigWithBudget(
		modelID,
		l.modelConfig,
		l.autoTuneBudget(modelID),
		effectiveSWAFull(model.Config{}),
	)
}

// Prepare resolves the model configuration once for both planning and loading.
func (l *Llama) Prepare(_ context.Context, req loader.LoadRequest) (any, error) {
	return l.resolveConfig(req)
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
	cfg, err := l.configForRequest(req)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: %w", err)
	}

	bpe := bytesPerElement(cfg.CacheTypeK, cfg.CacheTypeV)

	// When the resolved config leaves the context window unset (e.g. the
	// hardware analysis could not run), the model loads with the runtime
	// default applied by model.adjustContextWindow: min(trained_ctx, 8K).
	// Reserve against 8K so the plan never under-counts KV relative to what
	// the load actually uses.
	ctxWin := int64(cfg.ContextWindow())
	if ctxWin <= 0 {
		ctxWin = int64(vram.ContextWindow8K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:          ctxWin,
		BytesPerElement:        bpe,
		TypeK:                  int32(cfg.CacheTypeK),
		TypeV:                  int32(cfg.CacheTypeV),
		Slots:                  nseq,
		NUBatch:                effectivePrefillBatchSize(cfg),
		ExpertLayersOnGPU:      cfg.ExpertLayersOnGPU(),
		GPULayers:              int64(cfg.NGpuLayers()),
		KVCacheOnCPU:           cfg.PtrOffloadKQV != nil && !*cfg.PtrOffloadKQV,
		SWAFull:                effectiveSWAFull(cfg),
		VTransposed:            cfg.FlashAttention() == model.FlashAttentionDisabled,
		RecurrentStateCopies:   model.RecurrentStateCopies(cfg, false),
		EmbeddedMTPStateCopies: model.RecurrentStateCopies(cfg, true),
		ComputeContexts:        model.SpeculativeContextCount(cfg),
	}

	result, source, err := predictResult(l.models, req.ModelID, vramCfg)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: modelID[%s]: %w", req.ModelID, err)
	}

	planReq := resman.PlanRequest{
		Key:         req.Key,
		Devices:     gpuDevices(cfg.Devices),
		TensorSplit: cfg.TensorSplit,
		// Allow the resman to account an unpinned load across all GPUs
		// unless the user explicitly pinned a single GPU via
		// SplitModeNone. When the split mode is unset (nil), the runtime
		// defaults to layer splitting, so the model is auto-distributed
		// across every card — matching how llama.cpp actually places it.
		AllowSplit: cfg.PtrSplitMode == nil || *cfg.PtrSplitMode != model.SplitModeNone,
	}

	// Map the calculator's GPU/CPU split onto resman buckets.
	//
	// Unified memory is special-cased first.
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

	extraVRAM, extraRAM, err := l.additionalMemory(cfg, vramCfg)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: additional memory: %w", err)
	}
	planReq.VRAMBytes += extraVRAM
	planReq.RAMBytes += extraRAM

	memoryTopology := "cpu-only"
	switch {
	case l.resman.UnifiedMemory():
		memoryTopology = "unified"
	case l.resman.HasGPUs():
		memoryTopology = "discrete"
	}

	gpuLayerPlacement := "partial"
	switch vramCfg.GPULayers {
	case -1:
		gpuLayerPlacement = "cpu-only"
	case 0:
		gpuLayerPlacement = "all"
	}

	expertLayerPlacement := "not-applicable"
	if result.MoE != nil && result.MoE.IsMoE {
		expertLayerPlacement = "partial"
		switch vramCfg.ExpertLayersOnGPU {
		case 0:
			expertLayerPlacement = "cpu-only"
		case model.ExpertsAllOnGPU:
			expertLayerPlacement = "all"
		}
	}

	l.log(ctx, "plan-request",
		"key", req.Key,
		"model-id", req.ModelID,
		"source", source,
		"memory-topology", memoryTopology,
		"predicted-total", humanBytes(planReq.VRAMBytes+planReq.RAMBytes),
		"predicted-vram", humanBytes(result.TotalVRAM),
		"predicted-system", humanBytes(result.TotalSystemRAMEst),
		"context-window", ctxWin,
		"slots", nseq,
		"bytes-per-element", bpe,
		"expert-layer-placement", expertLayerPlacement,
		"experts-on-gpu-config", vramCfg.ExpertLayersOnGPU,
		"gpu-layer-placement", gpuLayerPlacement,
		"gpu-layers-config", vramCfg.GPULayers,
		"kv-cache-on-cpu", vramCfg.KVCacheOnCPU,
		"swa-full", vramCfg.SWAFull,
		"reserved-vram", humanBytes(planReq.VRAMBytes),
		"reserved-ram", humanBytes(planReq.RAMBytes),
		"devices", planReq.Devices,
		"tensor-split", planReq.TensorSplit,
		"allow-split", planReq.AllowSplit,
	)

	return planReq, nil
}

func (l *Llama) additionalMemory(cfg model.Config, targetCfg vram.Config) (vramBytes, ramBytes int64, err error) {
	addFile := func(path string, onCPU bool) error {
		if path == "" {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if l.resman.UnifiedMemory() || onCPU {
			ramBytes += info.Size()
		} else {
			vramBytes += info.Size()
		}
		return nil
	}

	projOnCPU := !l.resman.HasGPUs() || (cfg.PtrProjOnCPU != nil && *cfg.PtrProjOnCPU)
	if err := addFile(cfg.ProjFile, projOnCPU); err != nil {
		return 0, 0, err
	}
	if cfg.SpeculationMode() != model.SpeculationDisabled {
		mtpOnCPU := !l.resman.HasGPUs() || cfg.NGpuLayers() == -1
		if err := addFile(cfg.MTPDrafterFile, mtpOnCPU); err != nil {
			return 0, 0, err
		}
	}

	if cfg.PtrDraftModel == nil || !cfg.PtrDraftModel.IsSeparate() || cfg.SpeculationMode() == model.SpeculationDisabled {
		return vramBytes, ramBytes, nil
	}

	draftCfg := targetCfg
	draftCfg.Slots = 1
	draftCfg.GPULayers = int64(cfg.PtrDraftModel.NGpuLayers())
	draftCfg.RecurrentStateCopies = 1
	draftCfg.EmbeddedMTPStateCopies = 1
	draftCfg.ComputeContexts = 1
	draft, err := vram.FromFiles(cfg.PtrDraftModel.ModelFiles, draftCfg)
	if err != nil {
		return 0, 0, fmt.Errorf("draft: %w", err)
	}
	switch {
	case l.resman.UnifiedMemory():
		ramBytes += draft.UnifiedFootprint()
	case l.resman.HasGPUs() && cfg.PtrDraftModel.NGpuLayers() != -1:
		vramBytes += draft.TotalVRAM
		ramBytes += draft.TotalSystemRAMEst
	default:
		ramBytes += draft.TotalVRAM + draft.TotalSystemRAMEst
	}

	return vramBytes, ramBytes, nil
}

// Load implements loader.Loader.Load for the llama backend.
func (l *Llama) Load(ctx context.Context, req loader.LoadRequest) (*kronk.Kronk, error) {
	cfg, err := l.configForRequest(req)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	if l.insecureLogging {
		cfg.PtrInsecureLogging = new(true)
	}

	cfg.Log = l.log
	status := "disabled"
	if cfg.AutoTune {
		status = "deferred-to-sdk"
		if cfg.AutoTuned {
			status = "pre-applied"
		}
	}
	l.log(ctx, "AUTO-TUNE",
		"status", status,
		"enabled", cfg.AutoTune,
		"pre_applied", cfg.AutoTuned,
		"model_id", req.ModelID,
		"context_window", cfg.ContextWindow(),
		"nseq_max", cfg.NSeqMax(),
		"cache_type_k", cfg.CacheTypeK,
		"cache_type_v", cfg.CacheTypeV,
		"flash_attention", cfg.FlashAttention(),
		"split_mode", splitModeName(cfg.PtrSplitMode),
		"ngpu_layers", nGpuLayersName(cfg.PtrNGpuLayers),
	)

	krn, err := kronk.NewWithContext(ctx, model.WithConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("load: unable to create inference model: %w", err)
	}
	effective := krn.ModelConfig()
	l.log(ctx, "AUTO-TUNE",
		"status", "effective",
		"enabled", effective.AutoTune,
		"applied", effective.AutoTuned,
		"model_id", req.ModelID,
		"context_window", effective.ContextWindow(),
		"nseq_max", effective.NSeqMax(),
		"cache_type_k", effective.CacheTypeK,
		"cache_type_v", effective.CacheTypeV,
		"flash_attention", effective.FlashAttention(),
		"split_mode", splitModeName(effective.PtrSplitMode),
		"ngpu_layers", nGpuLayersName(effective.PtrNGpuLayers),
	)

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
	info = append(info, "isEmbedModel")
	info = append(info, krn.ModelInfo().IsEmbedModel)
	info = append(info, "isRerankModel")
	info = append(info, krn.ModelInfo().IsRerankModel)

	l.log(ctx, "load", info...)

	return krn, nil
}

// Validate checks live backend headroom after every native context has been
// created but before the pool publishes the handle. CUDA and HIP report
// runtime device memory. Metal remains advisory because GGML reports
// recommendedMaxWorkingSetSize rather than physical unified-memory capacity.
// Vulkan also remains advisory because drivers without VK_EXT_memory_budget
// report the whole heap as free.
func (l *Llama) Validate(ctx context.Context, req loader.LoadRequest, _ *kronk.Kronk) error {
	cfg, err := l.configForRequest(req)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	usage := l.resman.Usage()
	checks := backendMemoryChecks(cfg, devices.List(), usage.BudgetPercent, usage.HeadroomBytes)
	for _, check := range checks {
		status := "ok"
		if !check.enforced {
			status = "advisory"
		}
		if check.exhausted {
			status = "insufficient"
		}
		l.log(ctx, "backend-memory",
			"status", status,
			"model-id", req.ModelID,
			"device", check.name,
			"backend", check.backend,
			"free", backendMemoryBytes(check.displayFree()),
			"total", backendMemoryBytes(check.total),
			"required-free", backendMemoryBytes(check.requiredFree),
			"report-wrapped", check.wrapped,
			"budget-percent", usage.BudgetPercent,
			"context-window", cfg.ContextWindow(),
			"slots", cfg.NSeqMax(),
		)
		if check.enforced && check.exhausted {
			return fmt.Errorf("validate: model[%s] does not fit backend memory budget: device[%s] backend[%s] free[%s] total[%s] required-free[%s] report-wrapped[%t] context-window[%d] slots[%d]: reduce nseq-max, context-window, or cache precision: %w",
				req.ModelID, check.name, check.backend,
				backendMemoryBytes(check.displayFree()), backendMemoryBytes(check.total), backendMemoryBytes(check.requiredFree), check.wrapped,
				cfg.ContextWindow(), cfg.NSeqMax(), resman.ErrNoCapacity)
		}
	}

	return nil
}

type backendMemoryCheck struct {
	name         string
	backend      string
	free         uint64
	total        uint64
	requiredFree uint64
	enforced     bool
	exhausted    bool
	wrapped      bool
}

func (check backendMemoryCheck) displayFree() uint64 {
	if check.wrapped {
		return 0
	}
	return check.free
}

func backendMemoryBytes(value uint64) string {
	return humanBytes(int64(min(value, uint64(math.MaxInt64))))
}

func backendMemoryChecks(cfg model.Config, snapshot devices.Devices, budgetPercent int, headroomBytes int64) []backendMemoryCheck {
	if !configUsesGPU(cfg) {
		return nil
	}

	selected := selectedBackendDevices(cfg, snapshot.Devices)
	checks := make([]backendMemoryCheck, 0, len(selected))
	for _, device := range selected {
		if check, ok := backendMemoryCheckForDevice(device, budgetPercent, headroomBytes); ok {
			checks = append(checks, check)
		}
	}

	return checks
}

func backendMemoryCheckForDevice(device devices.DeviceInfo, budgetPercent int, headroomBytes int64) (backendMemoryCheck, bool) {
	switch device.Type {
	case "gpu_metal":
		return metalBackendMemoryCheck(device, budgetPercent, headroomBytes)
	case "gpu_cuda":
		return cudaBackendMemoryCheck(device, budgetPercent, headroomBytes)
	case "gpu_rocm":
		return rocmBackendMemoryCheck(device, budgetPercent, headroomBytes)
	case "gpu_vulkan":
		return vulkanBackendMemoryCheck(device, budgetPercent, headroomBytes)
	case "cpu":
		return cpuBackendMemoryCheck(device, budgetPercent, headroomBytes)
	default:
		return unsupportedBackendMemoryCheck(device, budgetPercent, headroomBytes)
	}
}

// metalBackendMemoryCheck is advisory because llama.cpp reports
// recommendedMaxWorkingSetSize and currentAllocatedSize from Metal. The former
// is a performance recommendation, not a hard limit on unified memory.
func metalBackendMemoryCheck(device devices.DeviceInfo, budgetPercent int, headroomBytes int64) (backendMemoryCheck, bool) {
	return reportedBackendMemoryCheck(device, budgetPercent, headroomBytes, false)
}

// cudaBackendMemoryCheck is enforceable because the CUDA runtime reports live
// free and total device memory.
func cudaBackendMemoryCheck(device devices.DeviceInfo, budgetPercent int, headroomBytes int64) (backendMemoryCheck, bool) {
	return reportedBackendMemoryCheck(device, budgetPercent, headroomBytes, true)
}

// rocmBackendMemoryCheck is enforceable because the HIP runtime reports live
// free and total device memory.
func rocmBackendMemoryCheck(device devices.DeviceInfo, budgetPercent int, headroomBytes int64) (backendMemoryCheck, bool) {
	return reportedBackendMemoryCheck(device, budgetPercent, headroomBytes, true)
}

// vulkanBackendMemoryCheck is advisory. Without VK_EXT_memory_budget, Vulkan
// drivers may report the entire heap as free instead of live available memory.
func vulkanBackendMemoryCheck(device devices.DeviceInfo, budgetPercent int, headroomBytes int64) (backendMemoryCheck, bool) {
	return reportedBackendMemoryCheck(device, budgetPercent, headroomBytes, false)
}

// cpuBackendMemoryCheck intentionally skips validation. llama.cpp does not
// currently expose a portable live host-memory value through this device API.
func cpuBackendMemoryCheck(devices.DeviceInfo, int, int64) (backendMemoryCheck, bool) {
	return backendMemoryCheck{}, false
}

// unsupportedBackendMemoryCheck is the boundary for future GGML backends.
// Unknown memory-reporting semantics must remain non-enforcing until verified.
func unsupportedBackendMemoryCheck(devices.DeviceInfo, int, int64) (backendMemoryCheck, bool) {
	return backendMemoryCheck{}, false
}

func reportedBackendMemoryCheck(device devices.DeviceInfo, budgetPercent int, headroomBytes int64, enforced bool) (backendMemoryCheck, bool) {
	if device.TotalBytes == 0 && device.FreeBytes == 0 {
		return backendMemoryCheck{}, false
	}

	reservePercent := max(100-budgetPercent, 0)
	requiredFree := device.TotalBytes * uint64(reservePercent) / 100
	if headroomBytes > 0 {
		requiredFree += uint64(headroomBytes)
	}

	return backendMemoryCheck{
		name:         device.Name,
		backend:      device.Backend,
		free:         device.FreeBytes,
		total:        device.TotalBytes,
		requiredFree: requiredFree,
		enforced:     enforced,
		exhausted:    device.FreeBytes > device.TotalBytes || device.FreeBytes < requiredFree,
		wrapped:      device.FreeBytes > device.TotalBytes,
	}, true
}

func configUsesGPU(cfg model.Config) bool {
	if cfg.NGpuLayers() != -1 {
		return true
	}
	if cfg.ProjFile != "" && (cfg.PtrProjOnCPU == nil || !*cfg.PtrProjOnCPU) {
		return true
	}
	if cfg.MTPDrafterFile != "" {
		return true
	}
	return cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() && cfg.PtrDraftModel.NGpuLayers() != -1
}

func selectedBackendDevices(cfg model.Config, available []devices.DeviceInfo) []devices.DeviceInfo {
	gpus := slices.DeleteFunc(slices.Clone(available), func(device devices.DeviceInfo) bool {
		return !strings.HasPrefix(device.Type, "gpu_")
	})

	names := append([]string(nil), cfg.Devices...)
	if cfg.NGpuLayers() != -1 && len(gpuDevices(cfg.Devices)) == 0 {
		if cfg.PtrSplitMode == nil || *cfg.PtrSplitMode != model.SplitModeNone {
			return gpus
		}
		if mainGPU := cfg.MainGPU(); mainGPU >= 0 && mainGPU < len(gpus) {
			names = append(names, gpus[mainGPU].Name)
		}
	}

	if cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() {
		if cfg.PtrDraftModel.NGpuLayers() != -1 && len(gpuDevices(cfg.PtrDraftModel.Devices)) == 0 {
			return gpus
		}
		names = append(names, cfg.PtrDraftModel.Devices...)
	}
	if cfg.ProjDevice != "" {
		names = append(names, cfg.ProjDevice)
	} else if cfg.ProjFile != "" && (cfg.PtrProjOnCPU == nil || !*cfg.PtrProjOnCPU) {
		return gpus
	}

	if len(names) > 0 {
		return slices.DeleteFunc(slices.Clone(available), func(device devices.DeviceInfo) bool {
			return !slices.Contains(names, device.Name)
		})
	}

	if cfg.PtrSplitMode != nil && *cfg.PtrSplitMode == model.SplitModeNone {
		if mainGPU := cfg.MainGPU(); mainGPU >= 0 && mainGPU < len(gpus) {
			return gpus[mainGPU : mainGPU+1]
		}
	}

	return gpus
}

func splitModeName(p *model.SplitMode) string {
	if p == nil {
		return "auto"
	}
	return p.String()
}

func nGpuLayersName(p *int) string {
	if p == nil {
		return "all"
	}
	return fmt.Sprintf("%d", *p)
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
		ctxWin = int64(vram.ContextWindow8K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:          ctxWin,
		BytesPerElement:        bytesPerElement(cfg.CacheTypeK, cfg.CacheTypeV),
		TypeK:                  int32(cfg.CacheTypeK),
		TypeV:                  int32(cfg.CacheTypeV),
		Slots:                  nseq,
		NUBatch:                effectivePrefillBatchSize(cfg),
		ExpertLayersOnGPU:      cfg.ExpertLayersOnGPU(),
		GPULayers:              int64(cfg.NGpuLayers()),
		KVCacheOnCPU:           cfg.PtrOffloadKQV != nil && !*cfg.PtrOffloadKQV,
		SWAFull:                effectiveSWAFull(cfg),
		VTransposed:            cfg.FlashAttention() == model.FlashAttentionDisabled,
		RecurrentStateCopies:   model.RecurrentStateCopies(cfg, false),
		EmbeddedMTPStateCopies: model.RecurrentStateCopies(cfg, true),
		ComputeContexts:        model.SpeculativeContextCount(cfg),
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

// effectiveSWAFull mirrors modelCtxParams without mutating the user config:
// explicit values win, while nil uses the default exported by the loaded
// llama.cpp library.
func effectiveSWAFull(cfg model.Config) bool {
	if cfg.PtrSWAFull != nil {
		return *cfg.PtrSWAFull
	}
	return llama.ContextDefaultParams().SwaFull != 0
}

func effectivePrefillBatchSize(cfg model.Config) int64 {
	prefillBatchSize := cfg.PrefillBatchSize()
	if prefillBatchSize <= 0 {
		return int64(model.DefaultPrefillBatchSize)
	}
	return int64(prefillBatchSize)
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

	cfg, err := l.models.KronkResolvedConfigWithBudget(req.ModelID, l.modelConfig, l.autoTuneBudget(req.ModelID), effectiveSWAFull(model.Config{}))
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: unable to retrieve model config: %w", err)
	}
	cfg.ResponseModelID = req.ModelID

	return cfg, nil
}

func (l *Llama) configForRequest(req loader.LoadRequest) (model.Config, error) {
	if req.Prepared == nil {
		return l.resolveConfig(req)
	}

	cfg, ok := req.Prepared.(model.Config)
	if !ok {
		return model.Config{}, fmt.Errorf("prepared config is %T, want model.Config", req.Prepared)
	}

	return cfg, nil
}

func (l *Llama) autoTuneBudget(modelID string) models.AutoTuneBudget {
	usage := l.resman.Usage()
	budget := models.AutoTuneBudget{
		Devices: l.startupDevices,

		SystemRAMBytes: min(
			int64(float64(l.startupDevices.SystemRAMBytes)*models.AutoTuneBudgetPercent/100),
			usage.RAMBudget,
		)}

	cfg := l.modelConfig[modelID]
	selected := gpuDevices(cfg.Devices)
	devicesInUse := usage.Devices
	if len(selected) > 0 {
		devicesInUse = make([]resman.DeviceUsage, 0, len(selected))
		for _, name := range selected {
			idx := slices.IndexFunc(usage.Devices, func(device resman.DeviceUsage) bool {
				return device.Name == name
			})
			if idx >= 0 {
				devicesInUse = append(devicesInUse, usage.Devices[idx])
			}
		}
	}

	availableDevices := make([]resman.DeviceUsage, 0, len(devicesInUse))
	deviceBudgets := make([]int64, 0, len(devicesInUse))
	for _, device := range devicesInUse {
		startup := slices.IndexFunc(l.startupDevices.Devices, func(candidate devices.DeviceInfo) bool {
			return candidate.Name == device.Name
		})
		if startup < 0 {
			continue
		}

		startupBudget := int64(float64(l.startupDevices.Devices[startup].FreeBytes) * models.AutoTuneBudgetPercent / 100)
		availableDevices = append(availableDevices, device)
		deviceBudgets = append(deviceBudgets, min(startupBudget, device.BudgetBytes))
	}

	budget.GPUBytes = autoTuneGPUCapacity(cfg, availableDevices, deviceBudgets)

	return budget
}

func autoTuneGPUCapacity(cfg models.ModelConfig, devs []resman.DeviceUsage, budgets []int64) int64 {
	if len(budgets) == 0 {
		return 0
	}

	if len(budgets) == 1 || len(gpuDevices(cfg.Devices)) == 0 && cfg.PtrSplitMode != nil && *cfg.PtrSplitMode == model.SplitModeNone {
		return slices.Max(budgets)
	}

	weights := make([]float64, len(budgets))
	var weightTotal float64
	if len(cfg.TensorSplit) == len(budgets) {
		for i, split := range cfg.TensorSplit {
			weights[i] = float64(split)
			weightTotal += weights[i]
		}
	}
	if weightTotal <= 0 {
		for i, device := range devs {
			weights[i] = float64(device.TotalBytes)
			weightTotal += weights[i]
		}
	}

	var capacity int64
	for i, weight := range weights {
		if weight <= 0 {
			continue
		}

		deviceCapacity := int64(float64(budgets[i]) * weightTotal / weight)
		if capacity == 0 || deviceCapacity < capacity {
			capacity = deviceCapacity
		}
	}

	return capacity
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
