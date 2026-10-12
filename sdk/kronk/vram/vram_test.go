package vram

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestCalculateGemmaWorkspaceScaling(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name                         string
		context, batch, vocab, slots int64
		compact, nonFlash            bool
		wantGPU, wantCPU             int64
	}{
		// Small vocabulary makes attention-mask storage the GPU peak.
		{"base", 4096, 128, 1024, 1, false, false, 259 * mib, 17 * mib / 8},
		{"double batch", 4096, 256, 1024, 1, false, false, 262 * mib, 17 * mib / 4},
		{"double context", 8192, 128, 1024, 1, false, false, 261 * mib, 33 * mib / 8},
		{"compact SWA", 4096, 128, 1024, 1, true, false, 258*mib + 5*mib/16, 23 * mib / 16},
		{"two streams, same batch", 4096, 128, 1024, 2, false, false, 259*mib + mib/4, 17 * mib / 8},
		{"without flash attention", 4096, 128, 1024, 1, false, true, 325 * mib, 33 * mib / 8},
		// A larger vocabulary makes logits, not masks, the GPU peak.
		{"logits peak", 4096, 128, 8192, 1, false, false, 261 * mib, 17 * mib / 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := Calculate(Input{ComputeKind: modelprofile.ComputeGemma4, ContextWindow: tt.context, Slots: tt.slots, NUBatch: tt.batch, VocabularySize: tt.vocab, EmbeddingLength: 256, HeadCount: 16, SlidingWindow: 1024, SWAFull: !tt.compact, VTransposed: tt.nonFlash})
			wantGPU, wantCPU := tt.wantGPU+tt.wantGPU/10, tt.wantCPU+tt.wantCPU/10
			if r.ComputeBufferEst != wantGPU || r.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", r.ComputeBufferEst, r.ComputeBufferCPU, wantGPU, wantCPU)
			}
			wantMemory := ComputeMemory{
				WorkspaceGPU:        tt.wantGPU - 256*mib,
				WorkspaceCPU:        tt.wantCPU,
				RuntimeAllowanceGPU: 256 * mib,
				MarginGPU:           tt.wantGPU / 10,
				MarginCPU:           tt.wantCPU / 10,
			}
			if r.ComputeMemory != wantMemory {
				t.Errorf("compute breakdown: got %+v, want %+v", r.ComputeMemory, wantMemory)
			}
		})
	}
}

func TestCalculateGemmaMetalWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend                    string
		slots, batch, swaValue, phaseMiB int64
		unified, cpuWeights, cpuCache    bool
	}{
		{"one slot logits peak", "metal", 1, 2052, 256, 2052, false, false, false},
		{"two slots attention peak", "metal", 2, 2056, 256, 3076, false, false, false},
		{"three slots attention peak", "metal", 3, 2060, 256, 4102, false, false, false},
		{"unequal K and V", "metal", 2, 2056, 128, 2564, false, false, false},
		{"unified KV pool", "metal", 2, 2056, 256, 2056, true, false, false},
		{"CUDA control", "cuda", 2, 2056, 256, 2056, false, false, false},
		{"CPU control", "metal", 2, 2056, 256, 2056, false, true, false},
		{"CPU KV control", "metal", 2, 2056, 256, 2056, false, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{
				ComputeBackend: tt.backend, ComputeKind: modelprofile.ComputeGemma4,
				ContextWindow: 131072, Slots: tt.slots, NUBatch: tt.batch,
				VocabularySize: 262144, EmbeddingLength: 2816,
				BlockCount: 3, HeadCount: 16, HeadCountKV: 2,
				KeyLength: 512, ValueLength: 512,
				SWAHeadCountKV: 8, SWAKeyLength: 256, SWAValueLength: tt.swaValue,
				HeadCountKVByLayer: []int64{8, 8, 2},
				SWAPattern:         []bool{true, true, false}, SlidingWindowLayers: 2,
				SlidingWindow: 1024, SWAFull: true,
				KVUnified: tt.unified, KVCacheOnCPU: tt.cpuCache,
				BytesPerElement: 2,
			}
			if tt.cpuWeights {
				input.GPULayers = -1
			}
			result := Calculate(input)
			// Each 262144-row F32 logits token is 1 MiB. Attention keeps
			// two F16 masks plus scratch for the largest layer, not all
			// layers. SWA has 8 heads; full attention has only 2.
			activation := tt.batch * 2816 * (32 + 8*(tt.slots-1))
			wantWorkspace := activation + tt.phaseMiB*mib
			wantGPU := wantWorkspace + 256*mib
			wantGPU += wantGPU / 10
			if result.ComputeMemory.WorkspaceGPU != wantWorkspace || result.ComputeBufferEst != wantGPU {
				t.Fatalf("workspace/reservation: got %d/%d, want %d/%d", result.ComputeMemory.WorkspaceGPU, result.ComputeBufferEst, wantWorkspace, wantGPU)
			}
			// The assistant borrows KV but still reserves the same scratch.
			input.SharedKVLayers = input.BlockCount
			assistant := Calculate(input)
			if assistant.SlotMemory != 0 || assistant.ComputeBufferEst != wantGPU {
				t.Fatalf("shared KV cache/compute: got %d/%d, want 0/%d", assistant.SlotMemory, assistant.ComputeBufferEst, wantGPU)
			}
			if tt.name == "two slots attention peak" {
				for _, control := range []string{"compact SWA", "partial offload", "without flash attention"} {
					t.Run(control, func(t *testing.T) {
						controlInput := input
						controlInput.SharedKVLayers = 0
						switch control {
						case "compact SWA":
							controlInput.SWAFull = false
						case "partial offload":
							controlInput.GPULayers = 2 // Output and the last full-attention layer.
						case "without flash attention":
							controlInput.VTransposed = true
						}
						metal := Calculate(controlInput)
						controlInput.ComputeBackend = "cuda"
						cuda := Calculate(controlInput)
						if metal.ComputeBufferEst != cuda.ComputeBufferEst {
							t.Errorf("Metal/CUDA control: got %d/%d, want equal", metal.ComputeBufferEst, cuda.ComputeBufferEst)
						}
					})
				}
			}
		})
	}
}

func TestCalculateMistral3Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend        string
		slots, value, gpuMiB int64
	}{
		{"one stream", "metal", 1, 128, 1600},
		{"two streams", "metal", 2, 128, 2192},
		{"unequal K/V widths", "metal", 1, 64, 1472},
		{"CUDA control", "cuda", 1, 128, 1088},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeMistral3, ComputeBackend: tt.backend,
				ContextWindow: 131072, Slots: tt.slots, NUBatch: 2048, OutputRows: tt.slots,
				EmbeddingLength: 5120, VocabularySize: 196608,
				BlockCount: 40, HeadCount: 32, HeadCountKV: 8, KeyLength: 128, ValueLength: tt.value,
			})
			// One 512 MiB mask, one layer's K/V scratch, and activation
			// storage coexist. Host allowance is the mask + 160 MiB.
			gpu, cpu := tt.gpuMiB*mib, 672*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, gpu, cpu)
			}
		})
	}
}

func TestCalculateGPTOSSWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend          string
		slots, layers, context int64
		compact, unified       bool
		wantGPU, wantCPU       int64
	}{
		{"full SWA", "metal", 1, 0, 131072, false, false, 1716 * mib, 1114 * mib},
		{"compact SWA", "metal", 1, 0, 131072, true, false, 1213 * mib, 611 * mib},
		{"two streams", "metal", 2, 0, 131072, false, false, 2017 * mib, 1114 * mib},
		{"padded per-stream context", "metal", 2, 0, 131073, false, false, 2020 * mib, 1116 * mib},
		{"unified pool", "metal", 2, 0, 131072, false, true, 3041 * mib, 2138 * mib},
		{"CUDA control", "cuda", 1, 0, 131072, false, false, 1460 * mib, 1114 * mib},
		// Only the trailing SWA layer is offloaded: scratch uses its
		// compact extent, not the host full-attention layer's extent.
		{"only SWA offloaded", "metal", 1, 2, 131072, true, false, 961*mib + mib/2, 611 * mib},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeGPTOSS, ComputeBackend: tt.backend,
				ContextWindow: tt.context, Slots: tt.slots, NUBatch: 2048, OutputRows: tt.slots,
				EmbeddingLength: 2880, VocabularySize: 201088,
				BlockCount: 3, HeadCount: 64, HeadCountKV: 8, KeyLength: 64, ValueLength: 64,
				SlidingWindow: 128, SlidingWindowLayers: 2, SWAPattern: []bool{true, false, true},
				SWAFull: !tt.compact, GPULayers: tt.layers, KVUnified: tt.unified,
			})
			wantGPU, wantCPU := tt.wantGPU+tt.wantGPU/10, tt.wantCPU+tt.wantCPU/10
			if result.ComputeBufferEst != wantGPU || result.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, wantGPU, wantCPU)
			}
		})
	}
}

func TestCalculateQwen35Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name                  string
		contexts, rows, slots int64
		backend               string
		wantGPU, wantCPU      int64
	}{
		// Ornith's 256K attention mask is 1026 MiB; Metal K/V scratch
		// adds 512 MiB once per arena, not once per attention layer.
		{"target", 1, 4, 1, "metal", 1922*mib + mib/4, 1090*mib + mib/8},
		{"target and embedded MTP", 2, 4, 1, "metal", 4356*mib + mib/2, 2148*mib + 3*mib/16},
		{"uncapped logits", 1, 0, 1, "metal", 2328*mib + 40*mib/1024, 1090*mib + mib/8},
		{"two streams", 1, 8, 2, "metal", 2466*mib + 5*mib/16, 1090*mib + mib/8},
		{"CUDA control", 1, 4, 1, "cuda", 1410*mib + mib/4, 1090*mib + mib/8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pattern := make([]bool, 41)
			heads := make([]int64, 41)
			for layer := range 40 {
				pattern[layer] = (layer+1)%4 != 0
				heads[layer] = 2
			}
			// A wider appended layer must affect only the MTP arena,
			// not the target's peak or the other attention layers.
			heads[40] = 4
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeQwen35, ComputeBackend: tt.backend,
				ContextWindow: 262144, Slots: tt.slots, NUBatch: 2052,
				OutputRows: tt.rows, VocabularySize: 248320, EmbeddingLength: 2048,
				BlockCount: 41, HeadCount: 16, HeadCountKV: 2, KeyLength: 256, ValueLength: 256,
				HeadCountKVByLayer: heads,
				RecurrentPattern:   pattern, NextNPredictLayers: 1, ComputeContexts: tt.contexts,
			})
			wantGPU := tt.wantGPU + tt.wantGPU/10
			// Margins are rounded per context, not after combining contexts.
			wantCPU := tt.wantCPU + tt.wantCPU/10
			if tt.contexts == 2 {
				wantGPU--
			}
			if result.ComputeBufferEst != wantGPU || result.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, wantGPU, wantCPU)
			}
		})
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
	wantMemory := ComputeMemory{
		WorkspaceGPU:        512 << 20,
		RuntimeAllowanceGPU: 512 << 20,
		MarginGPU:           2 * ((512 << 20) / 10),
	}
	if result.ComputeMemory != wantMemory {
		t.Errorf("combined compute breakdown: got %+v, want %+v", result.ComputeMemory, wantMemory)
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

			if gpuLayers != input.BlockCount+1 {
				t.Errorf("gpuLayers: got %d, want %d", gpuLayers, input.BlockCount+1)
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

func TestCalculateWeightPlacement(t *testing.T) {
	for _, tt := range []struct {
		name             string
		layers           int64
		tied             bool
		wantGPU, wantCPU int64
	}{
		{name: "untied CPU", layers: -1, wantCPU: 1000},
		{name: "untied output only", layers: 1, wantGPU: 300, wantCPU: 700},
		{name: "untied partial", layers: 3, wantGPU: 550, wantCPU: 450},
		{name: "untied all but first block", layers: 4, wantGPU: 675, wantCPU: 325},
		{name: "untied full", layers: 5, wantGPU: 800, wantCPU: 200},
		{name: "untied default", wantGPU: 800, wantCPU: 200},
		{name: "tied CPU aliases", layers: -1, tied: true, wantCPU: 700},
		{name: "tied output only duplicates", layers: 1, tied: true, wantGPU: 200, wantCPU: 700},
		{name: "tied partial duplicates", layers: 3, tied: true, wantGPU: 450, wantCPU: 450},
		{name: "tied full duplicates", layers: 5, tied: true, wantGPU: 700, wantCPU: 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tensors := []gguf.TensorInfo{{Name: "token_embd.weight", Bytes: 200}, {Name: "blk.0.attn_q.weight", Bytes: 500}}
			if !tt.tied {
				tensors = append(tensors, gguf.TensorInfo{Name: "output.weight", Bytes: 300})
			}
			weights := gguf.CategorizeWeights(tensors, 4)
			result := Calculate(Input{ModelSizeBytes: weights.TotalBytes, BlockCount: 4, GPULayers: tt.layers, Weights: &weights, TiedOutputFallback: true})
			if result.ModelWeightsGPU != tt.wantGPU || result.ModelWeightsCPU != tt.wantCPU {
				t.Fatalf("weights: got GPU/CPU %d/%d, want %d/%d", result.ModelWeightsGPU, result.ModelWeightsCPU, tt.wantGPU, tt.wantCPU)
			}
			if got := result.UnifiedFootprint() - result.ComputeBufferEst; got != weights.TotalBytes {
				t.Errorf("unified weights: got %d, want %d", got, weights.TotalBytes)
			}
		})
	}
}

func TestCalculateExpertsFollowOffloadedLayers(t *testing.T) {
	weights := gguf.WeightBreakdown{AlwaysActiveBytes: 400, ExpertBytesTotal: 1000, ExpertBytesByLayer: []int64{100, 200, 300, 400}}
	for _, tt := range []struct {
		layers, experts, wantGPU int64
	}{
		{-1, 4, 0}, {1, 4, 0}, {3, 4, 700}, {3, 1, 400}, {5, 0, 0}, {5, 4, 1000},
	} {
		result := Calculate(Input{ModelSizeBytes: 1400, BlockCount: 4, Weights: &weights, MoE: &gguf.MoEInfo{IsMoE: true}, GPULayers: tt.layers, ExpertLayersOnGPU: tt.experts})
		if result.ExpertGPUBytes != tt.wantGPU || result.ExpertCPUBytes != 1000-tt.wantGPU {
			t.Errorf("layers=%d experts=%d: got GPU/CPU %d/%d, want %d/%d", tt.layers, tt.experts, result.ExpertGPUBytes, result.ExpertCPUBytes, tt.wantGPU, 1000-tt.wantGPU)
		}
	}
}

func TestAutoFitIncludesOutputLayer(t *testing.T) {
	layers, _, result, fit := AutoFit(Input{ModelSizeBytes: 100, BlockCount: 4}, FitConstraints{CombinedFreeBytes: 1 << 30, SystemRAMBytes: 1 << 30})
	if !fit || layers != 5 || result.ModelWeightsCPU != 0 {
		t.Fatalf("full offload: got fit=%t layers=%d CPU=%d, want true/5/0", fit, layers, result.ModelWeightsCPU)
	}
}

func TestCalculateLazyWeightResidency(t *testing.T) {
	const gib int64 = 1 << 30
	for _, tt := range []struct {
		name, mode, load              string
		tableBytes                    int64
		platform, auto, isolated, cpu bool
		wantResident                  int64
	}{
		{"off", "off", "none", 5 * gib, true, true, false, false, 5 * gib},
		{"auto exactly threshold", "auto", "none", 4 * gib, true, true, false, false, 4 * gib},
		{"auto above threshold", "auto", "none", 4*gib + 1, true, true, false, false, gib / 4},
		{"auto unknown devices", "auto", "none", 5 * gib, true, false, false, false, 5 * gib},
		{"on bypasses device gate", "on", "none", gib, true, false, false, false, gib / 4},
		{"on unsupported platform", "on", "none", gib, false, true, false, false, gib},
		{"Metal overlapping mmap", "on", "mmap", gib, true, true, false, false, gib},
		{"Metal overlapping auto", "on", "auto", gib, true, true, false, false, gib},
		{"Metal isolated mmap", "on", "mmap", gib, true, true, true, false, gib / 4},
		{"CPU only mmap", "on", "mmap", gib, true, true, false, true, gib / 4},
		{"mlock", "on", "mlock", gib, true, true, true, false, gib},
		{"mmap mlock", "on", "mmap+mlock", gib, true, true, true, false, gib},
		{"small table capped", "on", "none", gib / 8, true, true, false, false, gib / 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{
				ModelSizeBytes: tt.tableBytes + 2*gib, BlockCount: 2,
				ComputeBackend: "metal", Slots: 2, NUBatch: 8,
				Weights:       &gguf.WeightBreakdown{InputBytes: tt.tableBytes, OutputBytes: gib},
				LazyTable:     LazyTable{Bytes: tt.tableBytes, RowBytes: 1024, RowsPerToken: 1, Isolated: tt.isolated},
				WeightLoading: WeightLoading{LazyMode: tt.mode, LoadMode: tt.load, MMapSupported: tt.platform, AutoMMapSupported: tt.auto},
			}
			if tt.cpu {
				input.GPULayers = -1
			}
			baselineInput := input
			baselineInput.WeightLoading.LazyMode = "off"
			baseline := Calculate(baselineInput)
			result := Calculate(input)
			wantSaving := tt.tableBytes - tt.wantResident
			if result.LazyTableBytes != tt.tableBytes || result.LazyResidentBytes != tt.wantResident || baseline.UnifiedFootprint()-result.UnifiedFootprint() != wantSaving || baseline.TotalSystemRAMEst-result.TotalSystemRAMEst != wantSaving {
				t.Fatalf("lazy table/resident/unified saving/host saving: got %d/%d/%d/%d, want %d/%d/%d/%d", result.LazyTableBytes, result.LazyResidentBytes, baseline.UnifiedFootprint()-result.UnifiedFootprint(), baseline.TotalSystemRAMEst-result.TotalSystemRAMEst, tt.tableBytes, tt.wantResident, wantSaving, wantSaving)
			}
			if result.TotalVRAM != baseline.TotalVRAM || result.SlotMemory != baseline.SlotMemory || result.ComputeMemory != baseline.ComputeMemory {
				t.Fatal("lazy weight residency changed GPU, slot, or compute memory")
			}
			if wantSaving > 0 {
				constraints := FitConstraints{UnifiedMemory: true, SystemRAMBytes: result.UnifiedFootprint() + wantSaving/2, Threshold: 1}
				if !AssessFit(result, constraints).Fits || AssessFit(baseline, constraints).Fits {
					t.Fatal("unified fitting did not use the lazy working set")
				}
				_, _, fitted, fits := AutoFit(input, constraints)
				wantFit, wantFittedResident := true, tt.wantResident
				if tt.cpu {
					// Unified auto-fit chooses GPU offload, reintroducing the
					// overlapping Metal mapping. Re-evaluate, do not reuse savings.
					wantFit, wantFittedResident = false, tt.tableBytes
				}
				if fits != wantFit || fitted.LazyResidentBytes != wantFittedResident {
					t.Fatalf("auto-fit lazy resident: got fit=%t bytes=%d, want %t/%d", fits, fitted.LazyResidentBytes, wantFit, wantFittedResident)
				}
			}
		})
	}
}

func TestCalculateLazyRowWorkingSet(t *testing.T) {
	page := int64(os.Getpagesize())
	input := Input{
		ModelSizeBytes: 10 << 30, BlockCount: 2, NUBatch: 8193, Slots: 1,
		Weights:       &gguf.WeightBreakdown{InputBytes: 8 << 30},
		LazyTable:     LazyTable{Bytes: 8 << 30, RowBytes: 3*page + 1, RowsPerToken: 3},
		WeightLoading: WeightLoading{LazyMode: "on", LoadMode: "none", MMapSupported: true},
	}
	// A row spanning three pages plus one byte can touch five pages at
	// an arbitrary offset. Three gathers/token, two batches of 8193 tokens.
	want := int64(245790) * page
	result := Calculate(input)
	if result.LazyResidentBytes != want {
		t.Fatalf("page-rounded row working set: got %d, want %d", result.LazyResidentBytes, want)
	}
	input.Slots = 2
	if got := Calculate(input).LazyResidentBytes; got != want {
		t.Errorf("slots sharing one physical batch: got %d, want %d", got, want)
	}
	input.ComputeContexts = 2
	if got := Calculate(input).LazyResidentBytes; got != min(8<<30, 2*want) {
		t.Errorf("two native contexts: got %d, want %d", got, min(8<<30, 2*want))
	}
	input.NUBatch = 1 << 20
	if got := Calculate(input).LazyResidentBytes; got != 8<<30 {
		t.Errorf("table cap: got %d, want %d", got, int64(8<<30))
	}
}
