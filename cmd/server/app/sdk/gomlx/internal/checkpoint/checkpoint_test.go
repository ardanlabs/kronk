package checkpoint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
)

func TestSaveAdapterWritesOnlyLoRAVariables(t *testing.T) {
	store := model.NewStore()
	defer store.Finalize()
	store.VariableWithValue("/base", tensors.FromValue([]float32{1, 2})).SetTrainable(false)
	store.VariableWithValue("/layer/query/lora/a", tensors.FromValue([][]float32{{1, 2}}))
	store.VariableWithValue("/layer/query/lora/b", tensors.FromValue([][]float32{{3}, {4}}))

	directory := filepath.Join(t.TempDir(), "adapter")
	if err := SaveAdapter(directory, store); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]int{"/layer/query/lora/a": {1, 2}, "/layer/query/lora/b": {2, 1}}
	if err := Validate(directory, expected); err != nil {
		t.Fatal(err)
	}
	if err := SaveAdapter(directory, store); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error = %v", err)
	}
}

func TestSaveAdapterRequiresLoRAVariables(t *testing.T) {
	store := model.NewStore()
	defer store.Finalize()
	store.VariableWithValue("/base", tensors.FromValue([]float32{1, 2})).SetTrainable(false)
	err := SaveAdapter(filepath.Join(t.TempDir(), "adapter"), store)
	if err == nil || !strings.Contains(err.Error(), "no LoRA") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCheckpointMetadataAndPayload(t *testing.T) {
	directory := t.TempDir()
	writeCheckpoint(t, directory, []variable{
		{ParameterName: "var:/a", Dimensions: []int{2, 3}, DType: "Float32", Pos: 0, Length: 24},
		{ParameterName: "var:/b", Dimensions: []int{1}, DType: "Float32", Pos: 24, Length: 4},
	}, 28)
	if err := Validate(directory, map[string][]int{"/a": {2, 3}, "/b": {1}}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCheckpointRejectsShapeAndTruncatedPayload(t *testing.T) {
	directory := t.TempDir()
	writeCheckpoint(t, directory, []variable{
		{ParameterName: "var:/a", Dimensions: []int{2, 3}, DType: "Float32", Pos: 0, Length: 24},
	}, 20)
	if err := Validate(directory, map[string][]int{"/a": {3, 2}}); err == nil || !strings.Contains(err.Error(), "want Float32") {
		t.Fatalf("shape error = %v", err)
	}
	if err := Validate(directory, map[string][]int{"/a": {2, 3}}); err == nil || !strings.Contains(err.Error(), "payload has") {
		t.Fatalf("payload error = %v", err)
	}
}

func writeCheckpoint(t *testing.T, directory string, variables []variable, payloadSize int64) {
	t.Helper()
	base := "checkpoint-n0000000-test"
	data, err := json.Marshal(metadata{Variables: variables, BinFormat: "uncompressed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, base+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, base+".bin"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(payloadSize); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
