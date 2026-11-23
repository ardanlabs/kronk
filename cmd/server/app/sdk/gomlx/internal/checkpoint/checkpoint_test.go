package checkpoint

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestTrainingCommitRestoreAndCorruption(t *testing.T) {
	store := model.NewStore()
	defer store.Finalize()
	store.VariableWithValue("/base", []float32{123}).SetTrainable(false)
	store.VariableWithValue("/layer/lora/a", []float32{1, 2})
	store.VariableWithValue("/AdamOptimizer/layer/lora/a_1st_moment", []float32{0.1, 0.2}).SetTrainable(false)
	store.VariableWithValue("/global_step", int64(7)).SetTrainable(false)
	store.VariableWithValue("/#rngState", []uint64{42, 43}).SetTrainable(false)
	excluded := map[string]bool{"/base": true}
	directory := t.TempDir()
	if _, _, err := ReadTraining(directory); !errors.Is(err, ErrNoTrainingCheckpoint) {
		t.Fatal(err)
	}
	first, err := SaveTraining(directory, store, excluded, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(filepath.Join(directory, first))
	if err != nil {
		t.Fatal(err)
	}
	state, variables, err := ReadTraining(directory)
	if err != nil || string(state) != "first" {
		t.Fatalf("read: %q %v", state, err)
	}
	for _, variable := range variables {
		if variable.Path == "/base" {
			t.Fatal("serialized frozen base weights")
		}
	}
	fresh := model.NewStore()
	defer fresh.Finalize()
	fresh.VariableWithValue("/base", []float32{456}).SetTrainable(false)
	if err := RestoreTraining(fresh, excluded, variables); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh.GetVariable("/base").MustValue().Value(), []float32{456}) {
		t.Fatal("restored base weights")
	}
	for _, variable := range variables {
		if !reflect.DeepEqual(fresh.GetVariable(variable.Path).MustValue().Value(), store.GetVariable(variable.Path).MustValue().Value()) {
			t.Fatalf("value differs: %s", variable.Path)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "training-orphan.chk"), []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".latest-orphan"), []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadTraining(directory); err != nil {
		t.Fatalf("orphan interrupted commit affected latest: %v", err)
	}
	second, err := SaveTraining(directory, store, excluded, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(filepath.Join(directory, first))
	if err != nil || !bytes.Equal(firstBytes, unchanged) || second == first {
		t.Fatal("generation not immutable")
	}
	secondPath := filepath.Join(directory, second)
	good, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(directory, "training-latest.json")
	pointer, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadTraining(directory); err == nil || errors.Is(err, ErrNoTrainingCheckpoint) {
		t.Fatalf("missing manifest silently reset committed work: %v", err)
	}
	if err := os.WriteFile(manifest, pointer, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, corruption := range []string{"truncated", "modified", "missing"} {
		t.Run(corruption, func(t *testing.T) {
			data := bytes.Clone(good)
			switch corruption {
			case "truncated":
				data = data[:len(data)/2]
			case "modified":
				data[len(data)/2] ^= 1
			case "missing":
				if err := os.Remove(secondPath); err != nil {
					t.Fatal(err)
				}
			}
			if corruption != "missing" {
				if err := os.WriteFile(secondPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := ReadTraining(directory); err == nil || errors.Is(err, ErrNoTrainingCheckpoint) {
				t.Fatalf("corrupt commit must fail closed, got %v", err)
			}
			if err := os.WriteFile(secondPath, good, 0o600); err != nil {
				t.Fatal(err)
			}
		})
	}
	variables[0].Path = "/base"
	if err := RestoreTraining(fresh, excluded, variables); err == nil {
		t.Fatal("accepted frozen variable")
	}
}

func TestTrainingRestoreRejectsDuplicateAndInvalidTensor(t *testing.T) {
	store := model.NewStore()
	defer store.Finalize()
	value := tensors.FromValue([]float32{1})
	defer value.FinalizeAll()
	var buffer bytes.Buffer
	if err := value.GobSerialize(gob.NewEncoder(&buffer)); err != nil {
		t.Fatal(err)
	}
	for _, variables := range [][]TrainingVariable{
		{{Path: "/a", Tensor: []byte("invalid")}},
		{{Path: "/a", Tensor: buffer.Bytes()}, {Path: "/a", Tensor: buffer.Bytes()}},
	} {
		if err := RestoreTraining(store, nil, variables); err == nil {
			t.Fatal("accepted invalid state")
		}
		if store.NumVariables() != 0 {
			t.Fatal("invalid restore modified store")
		}
	}
}
