package vram_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
)

func TestMoECacheBudget(t *testing.T) {
	for _, tt := range []struct {
		name  string
		moe   bool
		slots int64
		want  int64
	}{
		{"dense ignores cache", false, 1, 0},
		{"moe one sequence", true, 1, 1003},
		{"moe sequences share cache", true, 3, 1003},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := vram.Input{
				ModelSizeBytes: 4096, BlockCount: 2, ContextWindow: 16,
				HeadCountKV: 1, KeyLength: 8, ValueLength: 8,
				BytesPerElement: 2, Slots: tt.slots,
				MoE: &gguf.MoEInfo{IsMoE: tt.moe},
			}
			baseline := vram.Calculate(input)
			input.MoECacheSize = 1003
			result := vram.Calculate(input)
			if result.MoECacheBytes != tt.want || result.TotalVRAM-baseline.TotalVRAM != tt.want || result.UnifiedFootprint()-baseline.UnifiedFootprint() != tt.want {
				t.Fatalf("cache reservation: got cache %d, VRAM delta %d, unified delta %d, want %d", result.MoECacheBytes, result.TotalVRAM-baseline.TotalVRAM, result.UnifiedFootprint()-baseline.UnifiedFootprint(), tt.want)
			}
			if result.TotalSystemRAMEst != baseline.TotalSystemRAMEst {
				t.Fatal("cache changed original host memory estimate")
			}
			if !tt.moe {
				if result.Input.MoECacheSize != 0 {
					t.Fatal("dense model retained effective cache size")
				}
				return
			}
			devices := result.CalculatePerDevice(2, []float64{3, 1}, nil, 0)
			if devices[0].MoECacheBytes != 752 || devices[1].MoECacheBytes != 251 || devices[0].TotalBytes+devices[1].TotalBytes != result.TotalVRAM {
				t.Fatalf("global cache split: got %+v, want cache bytes 752/251 and total %d", devices, result.TotalVRAM)
			}
			fit := vram.AssessFit(result, vram.FitConstraints{CombinedFreeBytes: baseline.TotalVRAM + 1002, Threshold: 1})
			if fit.Fits {
				t.Fatal("cache budget was omitted from fit assessment")
			}
		})
	}
}
