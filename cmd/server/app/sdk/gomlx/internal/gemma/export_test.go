package gemma

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gguf"
)

func TestExportBestAdapterWritesLlamaCompatibleGGUF(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	modelPath := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, modelPath, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(modelPath, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{
		Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true},
	}
	result, err := runtime.TrainDataset(t.Context(), PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{
		Epochs: 2, LearningRate: 1e-3, Seed: 42,
		AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "adapter.gguf")
	if err := runtime.ExportBestAdapter(result, output); err != nil {
		t.Fatal(err)
	}

	reader, err := gguf.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if value, _ := reader.String("general.type"); value != "adapter" {
		t.Fatalf("general.type = %q", value)
	}
	if value, _ := reader.String("general.architecture"); value != "gemma3" {
		t.Fatalf("general.architecture = %q", value)
	}
	if value, _ := reader.String("adapter.type"); value != "lora" {
		t.Fatalf("adapter.type = %q", value)
	}
	if value, _ := reader.Float32("adapter.lora.alpha"); value != 4 {
		t.Fatalf("adapter.lora.alpha = %g", value)
	}
	want := map[string][]uint64{
		"blk.0.attn_q.weight.lora_a":      {8, 2},
		"blk.0.attn_q.weight.lora_b":      {2, 8},
		"blk.0.attn_k.weight.lora_a":      {8, 2},
		"blk.0.attn_k.weight.lora_b":      {2, 4},
		"blk.0.attn_v.weight.lora_a":      {8, 2},
		"blk.0.attn_v.weight.lora_b":      {2, 4},
		"blk.0.attn_output.weight.lora_a": {8, 2},
		"blk.0.attn_output.weight.lora_b": {2, 8},
		"blk.0.ffn_gate.weight.lora_a":    {8, 2},
		"blk.0.ffn_gate.weight.lora_b":    {2, 16},
		"blk.0.ffn_up.weight.lora_a":      {8, 2},
		"blk.0.ffn_up.weight.lora_b":      {2, 16},
		"blk.0.ffn_down.weight.lora_a":    {16, 2},
		"blk.0.ffn_down.weight.lora_b":    {2, 8},
	}
	for _, tensor := range reader.Tensors() {
		dimensions, exists := want[tensor.Name]
		if !exists || !slices.Equal(tensor.Dimensions, dimensions) || tensor.Type != 0 {
			t.Fatalf("unexpected tensor = %+v", tensor)
		}
		delete(want, tensor.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tensors = %v", want)
	}
}

func TestExportBestAdapterRejectsUnselectedCheckpoint(t *testing.T) {
	runtime := &Runtime{base: &BaseModel{}, adapterConfig: &AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1}}
	err := runtime.ExportBestAdapter(TrainingResult{
		BestEpoch: 1, BestValidationLoss: 1, BestCheckpoint: "unknown",
		Epochs: []EpochMetrics{{Epoch: 1, ValidationLoss: 2, Checkpoint: "other"}},
	}, filepath.Join(t.TempDir(), "adapter.gguf"))
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
}
