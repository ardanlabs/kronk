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
func (p *Pool) planRequest(ctx context.Context, modelID, key string, cfg model.Config) (resman.PlanRequest, error) {
	bpe := bytesPerElement(cfg.CacheTypeK)

	ctxWin := int64(cfg.ContextWindow())
	if ctxWin <= 0 {
		ctxWin = int64(vram.ContextWindow4K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:   ctxWin,
		BytesPerElement: bpe,
		Slots:           nseq,
	}

	predicted, source, err := predictBytes(p.models, modelID, vramCfg)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan-request: modelID[%s]: %w", modelID, err)
	}

	req := resman.PlanRequest{
		Key:         key,
		Devices:     gpuDevices(cfg.Devices),
		TensorSplit: cfg.TensorSplit,
	}

	// On systems with GPUs and full GPU offload (the only mode supported
	// today) we charge the entire predicted footprint against VRAM. On
	// CPU-only systems we charge the same footprint against system RAM so
	// the budget still gates loads.
	switch {
	case p.resman.HasGPUs() && cfg.NGpuLayers() != -1:
		req.VRAMBytes = predicted
	default:
		req.RAMBytes = predicted
	}

	// Log enough to reproduce the prediction outside the pool.
	p.log(ctx, "plan-request",
		"key", key,
		"model-id", modelID,
		"source", source,
		"predicted", humanBytes(predicted),
		"context-window", ctxWin,
		"slots", nseq,
		"bytes-per-element", bpe,
		"vram", humanBytes(req.VRAMBytes),
		"ram", humanBytes(req.RAMBytes),
		"devices", req.Devices,
		"tensor-split", req.TensorSplit,
	)

	return req, nil
}

// predictBytes returns the predicted memory footprint in bytes for a given
// model along with a source label identifying which estimator produced it.
//
// "calculate-vram" is the preferred path: it understands KV cache + compute
// buffer math for standard transformer architectures.
//
// "file-size" is the fallback used when the model's metadata is missing
// the keys that calculation needs (e.g. BERT-based rerankers and embedders).
// The raw on-disk size is a conservative under-estimate but enough to gate
// concurrent loads.
func predictBytes(m *models.Models, modelID string, cfg vram.Config) (int64, string, error) {
	if v, err := m.CalculateVRAM(modelID, cfg); err == nil {
		return v.TotalVRAM, "calculate-vram", nil
	}

	info, err := m.ModelInformation(modelID)
	if err != nil {
		return 0, "", fmt.Errorf("predict-bytes: model-information: %w", err)
	}
	return int64(info.Size), "file-size", nil
}

// bytesPerElement returns the per-element width of a KV cache type. When
// the type is unset (GGMLTypeAuto) F16 is assumed, which mirrors
// llama.cpp's default. The shared lookup lives in sdk/kronk/gguf so the
// SDK and tools sides stay in sync.
func bytesPerElement(t model.GGMLType) int64 {
	return gguf.BytesPerElement(int32(t))
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
