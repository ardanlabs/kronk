package gguf

import "testing"

func TestCategorizeWeights(t *testing.T) {
	// Simulate a small 2-layer MoE model.
	tensors := []TensorInfo{
		{Name: "token_embd.weight", GGMLType: 1, Dims: []int64{4096, 32000}, Bytes: GGMLTensorSize(1, []int64{4096, 32000})},
		{Name: "blk.0.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: GGMLTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.0.ffn_up_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: GGMLTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.0.ffn_down_exps.weight", GGMLType: 8, Dims: []int64{1024, 4096, 8}, Bytes: GGMLTensorSize(8, []int64{1024, 4096, 8})},
		{Name: "blk.0.ffn_gate_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: GGMLTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.1.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: GGMLTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.1.ffn_up_exps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: GGMLTensorSize(8, []int64{4096, 1024, 8})},
	}

	wb := CategorizeWeights(tensors, 2)

	expertBlk0 := GGMLTensorSize(8, []int64{4096, 1024, 8})*2 + GGMLTensorSize(8, []int64{1024, 4096, 8})
	expertBlk1 := GGMLTensorSize(8, []int64{4096, 1024, 8})
	wantExpertTotal := expertBlk0 + expertBlk1

	wantAlwaysActive := GGMLTensorSize(1, []int64{4096, 32000}) + GGMLTensorSize(1, []int64{4096, 4096})*2

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
	tensors := []TensorInfo{
		{Name: "token_embd.weight", GGMLType: 1, Dims: []int64{4096, 32000}, Bytes: GGMLTensorSize(1, []int64{4096, 32000})},
		{Name: "blk.0.attn_q.weight", GGMLType: 1, Dims: []int64{4096, 4096}, Bytes: GGMLTensorSize(1, []int64{4096, 4096})},
		{Name: "blk.0.ffn_up_chexps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: GGMLTensorSize(8, []int64{4096, 1024, 8})},
		{Name: "blk.0.ffn_down_chexps.weight", GGMLType: 8, Dims: []int64{1024, 4096, 8}, Bytes: GGMLTensorSize(8, []int64{1024, 4096, 8})},
		{Name: "blk.0.ffn_gate_chexps.weight", GGMLType: 8, Dims: []int64{4096, 1024, 8}, Bytes: GGMLTensorSize(8, []int64{4096, 1024, 8})},
	}

	wb := CategorizeWeights(tensors, 1)

	wantExpert := GGMLTensorSize(8, []int64{4096, 1024, 8})*2 + GGMLTensorSize(8, []int64{1024, 4096, 8})
	wantActive := GGMLTensorSize(1, []int64{4096, 32000}) + GGMLTensorSize(1, []int64{4096, 4096})

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
	moeTensors := []TensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_exps.weight"},
	}

	if !DetectMoEFromTensors(moeTensors) {
		t.Error("DetectMoEFromTensors should return true for MoE tensors")
	}

	chexpsTensors := []TensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_chexps.weight"},
	}

	if !DetectMoEFromTensors(chexpsTensors) {
		t.Error("DetectMoEFromTensors should return true for chexps tensors")
	}

	gateUpTensors := []TensorInfo{
		{Name: "blk.0.ffn_gate_up_exps.weight"},
	}

	if !DetectMoEFromTensors(gateUpTensors) {
		t.Error("DetectMoEFromTensors should return true for gate_up_exps tensors")
	}

	denseTensors := []TensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up.weight"},
		{Name: "blk.0.ffn_down.weight"},
	}

	if DetectMoEFromTensors(denseTensors) {
		t.Error("DetectMoEFromTensors should return false for dense tensors")
	}
}

func TestDetectSharedExpertsFromTensors(t *testing.T) {
	sharedTensors := []TensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_shared_up.weight"},
	}

	if !DetectSharedExpertsFromTensors(sharedTensors) {
		t.Error("DetectSharedExpertsFromTensors should return true when 'shared' is in tensor name")
	}

	shexpTensors := []TensorInfo{
		{Name: "blk.0.shexp_gate.weight"},
	}

	if !DetectSharedExpertsFromTensors(shexpTensors) {
		t.Error("DetectSharedExpertsFromTensors should return true when 'shexp' is in tensor name")
	}

	noSharedTensors := []TensorInfo{
		{Name: "blk.0.attn_q.weight"},
		{Name: "blk.0.ffn_up_exps.weight"},
	}

	if DetectSharedExpertsFromTensors(noSharedTensors) {
		t.Error("DetectSharedExpertsFromTensors should return false when no shared tensors")
	}
}
