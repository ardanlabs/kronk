package pool

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// planRequest derives a resource-manager plan request for the given resolved
// model config. modelID is used to fetch GGUF metadata for the VRAM
// calculation; key is the cache/reservation key (modelID for catalog loads,
// or a composite key for custom/playground sessions).
//
// The function charges the predicted VRAM and system-RAM footprints to
// the resman independently so MoE models — whose routed experts can live
// on either side depending on the runtime placement — are accounted for
// accurately. Charging only the GPU side (the previous behavior) silently
// dropped the CPU-resident expert weights, producing under-counts of the
// real resident footprint and exposing the pool to OOM on multi-load
// scenarios.
func (p *Pool) planRequest(ctx context.Context, modelID, key string, cfg model.Config) (resman.PlanRequest, error) {
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

	result, source, err := predictResult(p.models, modelID, vramCfg)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan-request: modelID[%s]: %w", modelID, err)
	}

	req := resman.PlanRequest{
		Key:         key,
		Devices:     gpuDevices(cfg.Devices),
		TensorSplit: cfg.TensorSplit,
	}

	// Map the calculator's GPU/CPU split onto resman buckets. On systems
	// with GPUs and the standard full-offload mode we charge VRAM and
	// system RAM independently so MoE expert offload is reflected in the
	// budget. On CPU-only systems (or when the user explicitly requested
	// NGpuLayers=-1) the entire footprint is system RAM.
	switch {
	case p.resman.HasGPUs() && cfg.NGpuLayers() != -1:
		req.VRAMBytes = result.TotalVRAM
		req.RAMBytes = result.TotalSystemRAMEst
	default:
		req.VRAMBytes = 0
		req.RAMBytes = result.TotalVRAM + result.TotalSystemRAMEst
	}

	// Log enough to reproduce the prediction outside the pool.
	p.log(ctx, "plan-request",
		"key", key,
		"model-id", modelID,
		"source", source,
		"predicted-total", HumanBytes(req.VRAMBytes+req.RAMBytes),
		"predicted-vram", HumanBytes(result.TotalVRAM),
		"predicted-system", HumanBytes(result.TotalSystemRAMEst),
		"context-window", ctxWin,
		"slots", nseq,
		"bytes-per-element", bpe,
		"experts-on-gpu", vramCfg.ExpertLayersOnGPU,
		"vram", HumanBytes(req.VRAMBytes),
		"ram", HumanBytes(req.RAMBytes),
		"devices", req.Devices,
		"tensor-split", req.TensorSplit,
	)

	return req, nil
}

// predictResult returns the full VRAM calculator result for a given
// model along with a source label identifying which estimator produced
// it.
//
// "calculate-vram" is the preferred path: it understands KV cache,
// compute buffer, and MoE expert placement for standard transformer
// architectures.
//
// "file-size" is the fallback used when the model's metadata is missing
// the keys that the calculator needs (e.g. BERT-based rerankers and
// embedders). The raw on-disk size is returned in TotalVRAM so the
// caller's bucket-mapping logic still gates concurrent loads, even
// though the breakdown is unavailable.
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
// (GGMLTypeAuto) F16 is assumed, mirroring llama.cpp's default. The max
// of K and V is used so a budget never undercounts the heavier half.
// The shared lookup lives in sdk/kronk/gguf so the SDK, tools and pool
// sides stay in sync.
func bytesPerElement(k, v model.GGMLType) int64 {
	return int64(gguf.MaxBytesPerElement(int32(k), int32(v)))
}

// gpuDevices filters out a "CPU" entry that some configs leave alongside
// real GPU device names. resman only tracks GPUs so we drop CPU here.
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
