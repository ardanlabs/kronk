package models

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
)

// CalculateVRAM retrieves model metadata and computes the VRAM
// requirements for the local model identified by modelID. The pure
// math lives in sdk/kronk/vram; this method just gathers GGUF metadata
// off the local file and forwards a vram.Input to vram.Calculate.
func (m *Models) CalculateVRAM(modelID string, cfg vram.Config) (vram.Result, error) {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return vram.Result{}, fmt.Errorf("calculate-vram: failed to retrieve model info: %w", err)
	}

	arch := gguf.DetectArchitecture(info.Metadata)
	if arch == "" {
		return vram.Result{}, fmt.Errorf("calculate-vram: unable to detect model architecture")
	}

	if gguf.IsVisionEncoder(arch) {
		return vram.Result{
			Input:     vram.Input{ModelSizeBytes: int64(info.Size)},
			TotalVRAM: int64(info.Size),
		}, nil
	}

	blockCount, err := gguf.ParseInt64WithFallback(info.Metadata, arch+".block_count", ".block_count")
	if err != nil {
		return vram.Result{}, fmt.Errorf("calculate-vram: failed to parse block_count: %w", err)
	}

	headCountKV, err := gguf.ParseInt64OrArrayAvg(info.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return vram.Result{}, fmt.Errorf("calculate-vram: failed to parse head_count_kv: %w", err)
	}

	keyLength, valueLength, err := gguf.ResolveKVLengths(info.Metadata, arch)
	if err != nil {
		return vram.Result{}, fmt.Errorf("calculate-vram: %w", err)
	}

	input := vram.Input{
		ModelSizeBytes:  int64(info.Size),
		ContextWindow:   cfg.ContextWindow,
		BlockCount:      blockCount,
		HeadCountKV:     headCountKV,
		KeyLength:       keyLength,
		ValueLength:     valueLength,
		BytesPerElement: cfg.BytesPerElement,
		Slots:           cfg.Slots,
	}

	return vram.Calculate(input), nil
}
