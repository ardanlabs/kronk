package models

import (
	"testing"
)

func TestDetectMoE(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     MoEInfo
	}{
		{
			name: "qwen3-235b-moe",
			metadata: map[string]string{
				"general.architecture":    "qwen3",
				"qwen3.expert_count":      "128",
				"qwen3.expert_used_count": "8",
				"qwen3.block_count":       "94",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     128,
				ExpertUsedCount: 8,
			},
		},
		{
			name: "deepseek-v3-moe",
			metadata: map[string]string{
				"general.architecture":        "deepseek2",
				"deepseek2.expert_count":      "256",
				"deepseek2.expert_used_count": "8",
				"deepseek2.block_count":       "61",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     256,
				ExpertUsedCount: 8,
			},
		},
		{
			name: "dense-llama",
			metadata: map[string]string{
				"general.architecture": "llama",
				"llama.block_count":    "32",
			},
			want: MoEInfo{
				IsMoE:           false,
				ExpertCount:     0,
				ExpertUsedCount: 0,
			},
		},
		{
			name: "dense-qwen3-8b",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"qwen3.block_count":    "36",
			},
			want: MoEInfo{
				IsMoE:           false,
				ExpertCount:     0,
				ExpertUsedCount: 0,
			},
		},
		{
			name: "fallback-suffix-scan",
			metadata: map[string]string{
				"general.architecture":  "unknown",
				"foo.expert_count":      "64",
				"foo.expert_used_count": "4",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     64,
				ExpertUsedCount: 4,
			},
		},
		{
			name: "shared-experts",
			metadata: map[string]string{
				"general.architecture":          "qwen3",
				"qwen3.expert_count":            "128",
				"qwen3.expert_used_count":       "8",
				"qwen3.ffn_shared_expert_count": "4",
			},
			want: MoEInfo{
				IsMoE:            true,
				ExpertCount:      128,
				ExpertUsedCount:  8,
				HasSharedExperts: true,
			},
		},
		{
			name: "missing-expert-used-count",
			metadata: map[string]string{
				"general.architecture": "mixtral",
				"mixtral.expert_count": "8",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     8,
				ExpertUsedCount: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMoE(tt.metadata)

			if got.IsMoE != tt.want.IsMoE {
				t.Errorf("IsMoE = %v, want %v", got.IsMoE, tt.want.IsMoE)
			}
			if got.ExpertCount != tt.want.ExpertCount {
				t.Errorf("ExpertCount = %d, want %d", got.ExpertCount, tt.want.ExpertCount)
			}
			if got.ExpertUsedCount != tt.want.ExpertUsedCount {
				t.Errorf("ExpertUsedCount = %d, want %d", got.ExpertUsedCount, tt.want.ExpertUsedCount)
			}
			if got.HasSharedExperts != tt.want.HasSharedExperts {
				t.Errorf("HasSharedExperts = %v, want %v", got.HasSharedExperts, tt.want.HasSharedExperts)
			}
		})
	}
}

func TestGGMLRowSize(t *testing.T) {
	tests := []struct {
		name     string
		ggmlType uint32
		ne0      int64
		want     int64
	}{
		{"F32-128", 0, 128, 512},
		{"F16-128", 1, 128, 256},
		{"Q4_0-128", 2, 128, 72},
		{"Q8_0-128", 8, 128, 136},
		{"BF16-128", 30, 128, 256},
		{"Q4_0-4096", 2, 4096, 2304},
		{"unknown-type", 255, 128, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ggmlRowSize(tt.ggmlType, tt.ne0)
			if got != tt.want {
				t.Errorf("ggmlRowSize(%d, %d) = %d, want %d", tt.ggmlType, tt.ne0, got, tt.want)
			}
		})
	}
}

func TestGGMLTensorSize(t *testing.T) {
	tests := []struct {
		name     string
		ggmlType uint32
		dims     []int64
		want     int64
	}{
		{"F16-2D-4096x4096", 1, []int64{4096, 4096}, 4096 * 2 * 4096},
		{"Q4_0-2D-4096x4096", 2, []int64{4096, 4096}, 2304 * 4096},
		{"F32-1D-128", 0, []int64{128}, 512},
		{"empty-dims", 0, []int64{}, 0},
		{"F16-3D", 1, []int64{128, 32, 8}, 128 * 2 * 32 * 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ggmlTensorSize(tt.ggmlType, tt.dims)
			if got != tt.want {
				t.Errorf("ggmlTensorSize(%d, %v) = %d, want %d", tt.ggmlType, tt.dims, got, tt.want)
			}
		})
	}
}

func TestCategorizeWeights(t *testing.T) {
	// Simulate a small 2-layer MoE model.
	tensors := []ggufTensorInfo{
		{Name: "token_embd.weight", GGMLType: 1, Dims: []int64{4096, 32000}, Bytes: ggmlTensorSize(1, []int64{4096, 32000})},
		{Name: "blk.0.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: ggmlTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.0.ffn_up_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: ggmlTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.0.ffn_down_exps.weight", GGMLType: 8, Dims: []int64{1024, 4096, 8}, Bytes: ggmlTensorSize(8, []int64{1024, 4096, 8})},
		{Name: "blk.0.ffn_gate_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: ggmlTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.1.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: ggmlTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.1.ffn_up_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: ggmlTensorSize(8, []int64{4096, 1024, 8})},
	}

	wb := categorizeWeights(tensors, 2)

	// Expert tensors: blk.0 has 3 expert tensors, blk.1 has 1.
	// Verify chexps are NOT counted in this standard test (they're tested separately below).
	expertBlk0 := ggmlTensorSize(8, []int64{4096, 1024, 8})*2 + ggmlTensorSize(8, []int64{1024, 4096, 8})
	expertBlk1 := ggmlTensorSize(8, []int64{4096, 1024, 8})
	wantExpertTotal := expertBlk0 + expertBlk1

	// Always-active: token_embd + blk.0.attn_q + blk.1.attn_q.
	wantAlwaysActive := ggmlTensorSize(1, []int64{4096, 32000}) + ggmlTensorSize(1, []int64{4096, 4096})*2

	if wb.ExpertBytesTotal != wantExpertTotal {
		t.Errorf("ExpertBytesTotal = %d, want %d", wb.ExpertBytesTotal, wantExpertTotal)
	}

	if wb.AlwaysActiveBytes != wantAlwaysActive {
		t.Errorf("AlwaysActiveBytes = %d, want %d", wb.AlwaysActiveBytes, wantAlwaysActive)
	}

	if wb.TotalBytes != wb.AlwaysActiveBytes+wb.ExpertBytesTotal {
		t.Errorf("TotalBytes = %d, want %d", wb.TotalBytes, wb.AlwaysActiveBytes+wb.ExpertBytesTotal)
	}

	if len(wb.ExpertBytesByLayer) != 2 {
		t.Fatalf("ExpertBytesByLayer length = %d, want 2", len(wb.ExpertBytesByLayer))
	}

	if wb.ExpertBytesByLayer[0] != expertBlk0 {
		t.Errorf("ExpertBytesByLayer[0] = %d, want %d", wb.ExpertBytesByLayer[0], expertBlk0)
	}

	if wb.ExpertBytesByLayer[1] != expertBlk1 {
		t.Errorf("ExpertBytesByLayer[1] = %d, want %d", wb.ExpertBytesByLayer[1], expertBlk1)
	}
}

func TestCategorizeWeightsChexps(t *testing.T) {
	// Simulate a model using channel expert (chexps) tensors.
	tensors := []ggufTensorInfo{
		{Name: "token_embd.weight", GGMLType: 1, Dims: []int64{4096, 32000}, Bytes: ggmlTensorSize(1, []int64{4096, 32000})},
		{Name: "blk.0.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: ggmlTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.0.ffn_up_chexps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: ggmlTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.0.ffn_down_chexps.weight", GGMLType: 8, Dims: []int64{1024, 4096, 8}, Bytes: ggmlTensorSize(8, []int64{1024, 4096, 8})},
		{Name: "blk.0.ffn_gate_chexps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: ggmlTensorSize(8, []int64{4096, 1024, 8})},
	}

	wb := categorizeWeights(tensors, 1)

	wantExpert := ggmlTensorSize(8, []int64{4096, 1024, 8})*2 + ggmlTensorSize(8, []int64{1024, 4096, 8})
	wantActive := ggmlTensorSize(1, []int64{4096, 32000}) + ggmlTensorSize(1, []int64{4096, 4096})

	if wb.ExpertBytesTotal != wantExpert {
		t.Errorf("ExpertBytesTotal = %d, want %d", wb.ExpertBytesTotal, wantExpert)
	}
	if wb.AlwaysActiveBytes != wantActive {
		t.Errorf("AlwaysActiveBytes = %d, want %d", wb.AlwaysActiveBytes, wantActive)
	}
	if wb.ExpertBytesByLayer[0] != wantExpert {
		t.Errorf("ExpertBytesByLayer[0] = %d, want %d", wb.ExpertBytesByLayer[0], wantExpert)
	}
}

func TestDetectMoEFromTensors(t *testing.T) {
	moeTensors := []ggufTensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_exps.weight"},
	}

	if !detectMoEFromTensors(moeTensors) {
		t.Error("DetectMoEFromTensors should return true for MoE tensors")
	}

	chexpsTensors := []ggufTensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_chexps.weight"},
	}

	if !detectMoEFromTensors(chexpsTensors) {
		t.Error("DetectMoEFromTensors should return true for chexps tensors")
	}

	gateUpTensors := []ggufTensorInfo{
		{Name: "blk.0.ffn_gate_up_exps.weight"},
	}

	if !detectMoEFromTensors(gateUpTensors) {
		t.Error("DetectMoEFromTensors should return true for gate_up_exps tensors")
	}

	denseTensors := []ggufTensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up.weight"},
		{Name: "blk.0.ffn_down.weight"},
	}

	if detectMoEFromTensors(denseTensors) {
		t.Error("DetectMoEFromTensors should return false for dense tensors")
	}
}

func TestDetectSharedExpertsFromTensors(t *testing.T) {
	sharedTensors := []ggufTensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_shared_up.weight"},
	}

	if !detectSharedExpertsFromTensors(sharedTensors) {
		t.Error("DetectSharedExpertsFromTensors should return true when 'shared' is in tensor name")
	}

	shexpTensors := []ggufTensorInfo{
		{Name: "blk.0.shexp_gate.weight"},
	}

	if !detectSharedExpertsFromTensors(shexpTensors) {
		t.Error("DetectSharedExpertsFromTensors should return true when 'shexp' is in tensor name")
	}

	noSharedTensors := []ggufTensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_exps.weight"},
	}

	if detectSharedExpertsFromTensors(noSharedTensors) {
		t.Error("DetectSharedExpertsFromTensors should return false when no shared tensors")
	}
}
