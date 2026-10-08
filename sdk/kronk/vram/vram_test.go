package vram

import (
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

func TestCalculateMoECache(t *testing.T) {
	input := Input{
		ModelSizeBytes: 1003, BlockCount: 3, ContextWindow: 512,
		HeadCountKV: 2, KeyLength: 8, ValueLength: 4, BytesPerElement: 2,
		Slots: 5, ComputeContexts: 2, EmbeddingLength: 128,
		MoE: &gguf.MoEInfo{IsMoE: true, ExpertCount: 7, ExpertUsedCount: 2},
		Weights: &gguf.WeightBreakdown{
			AlwaysActiveBytes: 103, ExpertBytesTotal: 900,
			ExpertBytesByLayer: []int64{200, 300, 400},
		},
	}
	base := Calculate(input)
	input.MoECacheSize = 0
	if got := Calculate(input); !reflect.DeepEqual(got, base) {
		t.Fatal("zero cache changed estimate")
	}
	const cache int64 = 12345
	const maps int64 = 4 * 7 * 3
	input.MoECacheSize = cache
	got := Calculate(input)
	if got.TotalVRAM-base.TotalVRAM != cache || got.ComputeBufferEst != base.ComputeBufferEst || got.SlotMemory != base.SlotMemory {
		t.Fatal("cache must be charged once independently of slots and compute contexts")
	}
	if got.ExpertCPUBytes != 900 || got.ModelWeightsCPU != base.ModelWeightsCPU || got.ExpertGPUBytes != 0 {
		t.Fatal("cache changed expert residency")
	}
	if got.MoECacheHostBytes != maps || got.TotalSystemRAMEst-base.TotalSystemRAMEst != maps {
		t.Fatalf("host maps: got %d, want %d", got.MoECacheHostBytes, maps)
	}
	if got.UnifiedFootprint()-base.UnifiedFootprint() != cache+maps {
		t.Fatal("unified footprint must count cache and host maps exactly once")
	}
	for _, count := range []int64{1, 2} {
		main := int(count - 1)
		devices := CalculatePerDevice(got.ModelWeightsGPU, got.KVVRAMBytes, got.ComputeBufferEst, count, []float64{1, 3}, nil, main, cache)
		var total int64
		for i, device := range devices {
			total += device.TotalBytes
			wantCache := int64(0)
			if i == main {
				wantCache = cache
			}
			if device.MoECacheBytes != wantCache || device.TotalBytes != device.WeightsBytes+device.KVBytes+device.ComputeBytes+device.MoECacheBytes {
				t.Fatal("incorrect main-device cache allocation")
			}
		}
		if total != got.TotalVRAM {
			t.Fatalf("device total: got %d, want %d", total, got.TotalVRAM)
		}
	}
}

func TestBuildFromMetadataMoECache(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "llama", "llama.block_count": "3",
		"llama.embedding_length": "128", "llama.attention.head_count": "4",
		"llama.attention.head_count_kv": "2", "llama.expert_count": "7",
	}
	got, err := buildFromMetadata(metadata, nil, 1003, Config{MoECacheSize: 12345, Slots: 5, ComputeContexts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.Input.MoECacheSize != 12345 || got.MoECacheHostBytes != 4*7*3 {
		t.Fatalf("cache configuration or metadata not propagated: %+v", got)
	}
}

func TestEstimateComputeBufferUsesNUBatch(t *testing.T) {
	base := EstimateComputeBuffer(Input{
		NUBatch:         DefaultNUBatch,
		EmbeddingLength: 4096,
	})
	doubled := EstimateComputeBuffer(Input{
		NUBatch:         2 * DefaultNUBatch,
		EmbeddingLength: 4096,
	})
	defaulted := EstimateComputeBuffer(Input{
		EmbeddingLength: 4096,
	})

	if doubled <= base {
		t.Errorf("doubled NUBatch estimate: got %d, want greater than %d", doubled, base)
	}
	if defaulted != base {
		t.Errorf("default NUBatch estimate: got %d, want %d", defaulted, base)
	}
}

func TestCalculateChargesEveryComputeContext(t *testing.T) {
	input := Input{
		ModelSizeBytes:  100,
		NUBatch:         DefaultNUBatch,
		EmbeddingLength: 4096,
		ComputeContexts: 2,
	}

	result := Calculate(input)
	want := 2 * EstimateComputeBuffer(input)
	if result.ComputeBufferEst != want {
		t.Errorf("ComputeBufferEst: got %d, want %d", result.ComputeBufferEst, want)
	}
}

func TestAutoFitCPUOnly(t *testing.T) {
	const mib = 1024 * 1024

	tests := []struct {
		name  string
		input Input
	}{
		{
			name: "dense",
			input: Input{
				ModelSizeBytes: 100 * mib,
				BlockCount:     1,
			},
		},
		{
			name: "moe",
			input: Input{
				ModelSizeBytes: 200 * mib,
				BlockCount:     1,
				MoE:            &gguf.MoEInfo{IsMoE: true},
				Weights: &gguf.WeightBreakdown{
					AlwaysActiveBytes:  100 * mib,
					ExpertBytesTotal:   100 * mib,
					ExpertBytesByLayer: []int64{100 * mib},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuLayers, expertLayers, result, fit := AutoFit(tt.input, FitConstraints{
				CombinedFreeBytes: 320 * mib,
				SystemRAMBytes:    300 * mib,
			})

			if !fit {
				t.Fatal("fit: got false, want true")
			}
			if gpuLayers != -1 {
				t.Errorf("gpuLayers: got %d, want -1", gpuLayers)
			}
			if expertLayers != 0 {
				t.Errorf("expertLayers: got %d, want 0", expertLayers)
			}
			if result.ModelWeightsGPU != 0 {
				t.Errorf("ModelWeightsGPU: got %d, want 0", result.ModelWeightsGPU)
			}
		})
	}
}

func TestAutoFitDenseNoFitPreservesCPUOnlySentinel(t *testing.T) {
	gpuLayers, _, result, fit := AutoFit(Input{
		ModelSizeBytes: 100 * 1024 * 1024,
		BlockCount:     1,
	}, FitConstraints{
		CombinedFreeBytes: 1,
		SystemRAMBytes:    1,
	})

	if fit {
		t.Fatal("fit: got true, want false")
	}
	if gpuLayers != -1 {
		t.Errorf("gpuLayers: got %d, want -1", gpuLayers)
	}
	if result.Input.GPULayers != -1 {
		t.Errorf("result.Input.GPULayers: got %d, want -1", result.Input.GPULayers)
	}
}

func TestAutoFitUnifiedMemory(t *testing.T) {
	input := Input{
		ModelSizeBytes: 1000,
		BlockCount:     2,
		MoE: &gguf.MoEInfo{
			IsMoE: true,
		},
		Weights: &gguf.WeightBreakdown{
			AlwaysActiveBytes:  100,
			ExpertBytesTotal:   900,
			ExpertBytesByLayer: []int64{450, 450},
		},
	}
	wantFootprint := Calculate(input).UnifiedFootprint()

	tests := []struct {
		name     string
		ramBytes int64
		wantFit  bool
	}{
		{name: "fits", ramBytes: wantFootprint * 2, wantFit: true},
		{name: "does not fit", ramBytes: 1, wantFit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuLayers, expertLayers, result, fit := AutoFit(input, FitConstraints{
				SystemRAMBytes: tt.ramBytes,
				UnifiedMemory:  true,
			})

			if gpuLayers != input.BlockCount {
				t.Errorf("gpuLayers: got %d, want %d", gpuLayers, input.BlockCount)
			}
			if expertLayers != input.BlockCount {
				t.Errorf("expertLayers: got %d, want %d", expertLayers, input.BlockCount)
			}
			if fit != tt.wantFit {
				t.Errorf("fit: got %t, want %t", fit, tt.wantFit)
			}
			if got := result.UnifiedFootprint(); got != wantFootprint {
				t.Errorf("UnifiedFootprint: got %d, want %d", got, wantFootprint)
			}
			if result.ExpertGPUBytes != input.Weights.ExpertBytesTotal {
				t.Errorf("ExpertGPUBytes: got %d, want %d", result.ExpertGPUBytes, input.Weights.ExpertBytesTotal)
			}
		})
	}
}

func TestAssessFitUnifiedMemory(t *testing.T) {
	result := Result{Input: Input{ModelSizeBytes: 90}}

	tests := []struct {
		name       string
		capacity   int64
		wantStatus FitStatus
		wantFit    bool
	}{
		{name: "comfortable", capacity: 200, wantStatus: FitStatusFits, wantFit: true},
		{name: "tight", capacity: 100, wantStatus: FitStatusTight, wantFit: true},
		{name: "outside safe budget", capacity: 94, wantStatus: FitStatusDoesNotFit, wantFit: false},
		{name: "unknown capacity", wantStatus: FitStatusUnknown, wantFit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := AssessFit(result, FitConstraints{
				SystemRAMBytes: tt.capacity,
				UnifiedMemory:  true,
			})

			if assessment.Status != tt.wantStatus {
				t.Errorf("Status: got %q, want %q", assessment.Status, tt.wantStatus)
			}
			if assessment.Fits != tt.wantFit {
				t.Errorf("Fits: got %t, want %t", assessment.Fits, tt.wantFit)
			}
			if assessment.Unified.HeadroomBytes != tt.capacity-result.UnifiedFootprint() {
				t.Errorf("HeadroomBytes: got %d, want %d", assessment.Unified.HeadroomBytes, tt.capacity-result.UnifiedFootprint())
			}
		})
	}
}

func TestAssessFitRequiresCapacityForRequiredSystemRAM(t *testing.T) {
	assessment := AssessFit(Result{
		TotalVRAM:         50,
		TotalSystemRAMEst: 25,
	}, FitConstraints{CombinedFreeBytes: 100})

	if assessment.Status != FitStatusUnknown {
		t.Errorf("Status: got %q, want %q", assessment.Status, FitStatusUnknown)
	}
	if assessment.Fits {
		t.Error("Fits: got true, want false")
	}
}
