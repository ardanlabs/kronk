package gemma

import (
	"cmp"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
)

func TestTrainDatasetRunsDeterministicEpochsAndPreservesBase(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")

	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if _, err := runtime.Run([]int32{1, 4, 5}); err != nil {
		t.Fatal(err)
	}
	baseBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return !strings.Contains(variable.Path(), "/lora/") && !strings.HasPrefix(variable.Path(), "/#")
	})
	adapterBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})
	dataset := PreparedDataset{
		Training: []TrainingRecord{
			{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
			{Tokens: []int32{1, 7, 8, 9}, Targets: []int32{7, 8, 9, 10}, LossMask: []bool{false, false, true, true}},
		},
		Validation: []TrainingRecord{
			{Tokens: []int32{1, 11, 12}, Targets: []int32{11, 12, 13}, LossMask: []bool{false, true, true}},
			{Tokens: []int32{1, 14}, Targets: []int32{14, 15}, LossMask: []bool{false, true}},
		},
	}
	var progress []TrainingProgress
	checkpointRoot := filepath.Join(t.TempDir(), "checkpoints")
	result, err := runtime.TrainDataset(t.Context(), dataset, TrainingConfig{
		Epochs: 2, LearningRate: 1e-3, WeightDecay: 0.01, BatchSize: 2,
		Seed: 42, AdapterCheckpointDirectory: checkpointRoot,
	}, func(update TrainingProgress) {
		progress = append(progress, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Epochs) != 2 {
		t.Fatalf("epoch metrics = %d, want 2", len(result.Epochs))
	}
	for index, epoch := range result.Epochs {
		if epoch.Epoch != index+1 || math.IsNaN(epoch.TrainingLoss) || math.IsInf(epoch.TrainingLoss, 0) || math.IsNaN(epoch.ValidationLoss) || math.IsInf(epoch.ValidationLoss, 0) {
			t.Fatalf("epoch metrics = %+v", epoch)
		}
	}
	wantBest := result.Epochs[0]
	for _, epoch := range result.Epochs[1:] {
		if epoch.ValidationLoss < wantBest.ValidationLoss {
			wantBest = epoch
		}
	}
	if result.BestEpoch != wantBest.Epoch || result.BestValidationLoss != wantBest.ValidationLoss || result.BestCheckpoint != wantBest.Checkpoint {
		t.Fatalf("best checkpoint = %+v, want %+v", result, wantBest)
	}
	entries, err := os.ReadDir(checkpointRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Join(checkpointRoot, entries[0].Name()) != result.BestCheckpoint {
		t.Fatalf("retained checkpoints = %v, want only %q", entries, result.BestCheckpoint)
	}
	if len(progress) != 2 || progress[1] != (TrainingProgress{Epoch: 2, CompletedSteps: 2, TotalSteps: 2}) {
		t.Fatalf("progress = %+v", progress)
	}
	baseAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		_, exists := baseBefore[variable.Path()]
		return exists
	})
	if !equalVariableSnapshots(baseBefore, baseAfter) {
		t.Fatal("training changed frozen base weights")
	}
	adapterAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		_, exists := adapterBefore[variable.Path()]
		return exists
	})
	if equalVariableSnapshots(adapterBefore, adapterAfter) {
		t.Fatal("training did not change LoRA weights")
	}
}

// Without validation records, an epoch's training loss selects the checkpoint.
func TestTrainDatasetWithoutValidationSelectsByTrainingLoss(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	dataset := PreparedDataset{Training: []TrainingRecord{
		{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
		{Tokens: []int32{1, 7, 8, 9}, Targets: []int32{7, 8, 9, 10}, LossMask: []bool{false, false, true, true}},
	}}
	result, err := runtime.TrainDataset(t.Context(), dataset, TrainingConfig{
		Epochs: 3, LearningRate: 1e-2, BatchSize: 2, Seed: 42, AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantBest := result.Epochs[0]
	for _, epoch := range result.Epochs {
		if epoch.ValidationLoss != epoch.TrainingLoss || math.IsNaN(epoch.TrainingLoss) {
			t.Fatalf("epoch %d selection loss %g, want its training loss %g", epoch.Epoch, epoch.ValidationLoss, epoch.TrainingLoss)
		}
		if epoch.TrainingLoss < wantBest.TrainingLoss {
			wantBest = epoch
		}
	}
	if result.BestEpoch != wantBest.Epoch || validateSelectedCheckpoint(result) != nil {
		t.Fatalf("selected epoch %d, want %d with an exportable checkpoint: %+v", result.BestEpoch, wantBest.Epoch, result)
	}
}

func TestTrainDatasetValidationLossIsEpochLocal(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", cmp.Or(os.Getenv("GOMLX_BACKEND"), "go"))
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	// Distinct embedding directions make the synthetic checkpoint losses
	// change measurably, unlike the shared fixture's nearly parallel vectors.
	embedding := make([]float32, runtime.base.config.VocabSize*runtime.base.config.EmbeddingDimension)
	for index := range embedding {
		embedding[index] = float32(math.Sin(float64(index*17 + 3)))
	}
	for path, values := range map[string][]float32{
		"/token_embed/embeddings": embedding,
		"/output/dense/weights":   transpose(embedding, runtime.base.config.VocabSize, runtime.base.config.EmbeddingDimension),
	} {
		variable := runtime.base.store.GetVariable(path)
		if err := variable.SetValue(tensors.FromFlatDataAndDimensions(values, variable.Shape().Dimensions...)); err != nil {
			t.Fatal(err)
		}
	}
	baseBefore := snapshotVariables(t, runtime.base.store, func(*model.Variable) bool { return true })
	dataset := PreparedDataset{
		Training: []TrainingRecord{
			{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
			{Tokens: []int32{1, 7, 8, 9}, Targets: []int32{7, 8, 9, 10}, LossMask: []bool{false, false, true, true}},
		},
		Validation: []TrainingRecord{
			{Tokens: []int32{1, 11, 12}, Targets: []int32{11, 12, 13}, LossMask: []bool{false, true, true}},
			{Tokens: []int32{1, 14}, Targets: []int32{14, 15}, LossMask: []bool{false, true}},
		},
	}
	var expected []float64
	config := TrainingConfig{
		Epochs: 3, LearningRate: 0.1, BatchSize: 1, Seed: 42,
		AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
		PersistCheckpoint: func(result TrainingResult) error {
			if len(result.Epochs) == len(expected) {
				return nil
			}
			weights := func(variable *model.Variable) bool {
				_, base := baseBefore[variable.Path()]
				return base || strings.Contains(variable.Path(), "/lora/")
			}
			before := snapshotVariables(t, runtime.base.store, weights)
			var total float64
			var active int
			for _, record := range dataset.Validation {
				_, err := runtime.run(record.Tokens, func(values []float32) {
					for position, use := range record.LossMask {
						if !use {
							continue
						}
						logits := values[position*runtime.base.config.VocabSize : (position+1)*runtime.base.config.VocabSize]
						maximum := float64(slices.Max(logits))
						var sum float64
						for _, value := range logits {
							sum += math.Exp(float64(value) - maximum)
						}
						total += maximum + math.Log(sum) - float64(logits[record.Targets[position]])
						active++
					}
				})
				if err != nil {
					return err
				}
			}
			if !equalVariableSnapshots(before, snapshotVariables(t, runtime.base.store, weights)) {
				t.Fatal("measuring validation loss changed base or LoRA weights")
			}
			expected = append(expected, total/float64(active))
			return nil
		},
	}
	result, err := runtime.TrainDataset(t.Context(), dataset, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Epochs) != config.Epochs || len(expected) != config.Epochs {
		t.Fatalf("completed %d epochs and measured %d, want %d", len(result.Epochs), len(expected), config.Epochs)
	}
	best := 0
	for index, epoch := range result.Epochs {
		t.Logf("epoch %d validation loss: reported=%g independent=%g", epoch.Epoch, epoch.ValidationLoss, expected[index])
		// The independent host reduction is F64, while the batched graph uses F32.
		if math.IsNaN(expected[index]) || math.IsInf(expected[index], 0) || math.Abs(epoch.ValidationLoss-expected[index]) > 1e-5*math.Max(1, math.Abs(expected[index])) {
			t.Errorf("epoch %d validation loss = %g, want current-checkpoint loss %g", epoch.Epoch, epoch.ValidationLoss, expected[index])
		}
		if expected[index] < expected[best] {
			best = index
		}
	}
	if result.BestEpoch != best+1 || math.Abs(result.BestValidationLoss-expected[best]) > 1e-5*math.Max(1, math.Abs(expected[best])) || result.BestCheckpoint != result.Epochs[best].Checkpoint {
		t.Fatalf("selected checkpoint = %+v, want epoch %d with independent loss %g", result, best+1, expected[best])
	}
	baseAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		_, exists := baseBefore[variable.Path()]
		return exists
	})
	if !equalVariableSnapshots(baseBefore, baseAfter) {
		t.Fatal("training changed frozen base weights")
	}
}

func TestTrainDatasetRetainsRequestedCheckpoints(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	dataset := PreparedDataset{
		Training: []TrainingRecord{
			{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
		},
		Validation: []TrainingRecord{
			{Tokens: []int32{1, 7, 8}, Targets: []int32{7, 8, 9}, LossMask: []bool{false, true, true}},
		},
	}
	checkpointRoot := filepath.Join(t.TempDir(), "checkpoints")
	result, err := runtime.TrainDataset(t.Context(), dataset, TrainingConfig{
		Epochs: 2, LearningRate: 1e-3, Seed: 42,
		AdapterCheckpointDirectory: checkpointRoot, RetainedCheckpointEpochs: []int{1, 2},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(checkpointRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || result.Epochs[0].Checkpoint == "" || result.Epochs[1].Checkpoint == "" {
		t.Fatalf("retained checkpoints = %v, metrics = %+v", entries, result.Epochs)
	}
}

func TestEarlyStopperWaitsForConsecutiveValidationMisses(t *testing.T) {
	stopper := earlyStopper{patience: 3, minimumImprovement: .01}
	observations := []struct {
		loss float64
		stop bool
	}{
		{1, false},
		{.995, false},
		{.991, false},
		{.989, false},
		{.985, false},
		{.981, false},
		{.98, true},
	}
	for index, observation := range observations {
		if stop := stopper.observe(observation.loss); stop != observation.stop {
			t.Fatalf("observation %d stopped=%t, want %t", index+1, stop, observation.stop)
		}
	}
	noStopping := earlyStopper{}
	noStopping.observe(1)
	if noStopping.observe(1) {
		t.Fatal("zero patience enabled early stopping")
	}
	equalLoss := earlyStopper{patience: 2}
	first := equalLoss.observe(1)
	second := equalLoss.observe(1)
	third := equalLoss.observe(1)
	if first || second || !third {
		t.Fatal("unchanged validation loss did not trigger early stopping")
	}
}

func TestTrainDatasetRejectsNegativeEarlyStoppingPatience(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{Tokens: []int32{1}, Targets: []int32{2}, LossMask: []bool{true}}
	_, err = runtime.TrainDataset(t.Context(), PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{
		Epochs: 1, EarlyStoppingPatience: -1, LearningRate: 1e-3,
		AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "patience") {
		t.Fatalf("error = %v, want invalid patience", err)
	}
}

func TestTrainDatasetRejectsInvalidMinimumValidationImprovement(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{Tokens: []int32{1}, Targets: []int32{2}, LossMask: []bool{true}}
	_, err = runtime.TrainDataset(t.Context(), PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{
		Epochs: 1, MinimumValidationImprovement: math.NaN(), LearningRate: 1e-3,
		AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "minimum validation improvement") {
		t.Fatalf("error = %v, want invalid minimum validation improvement", err)
	}
}

func TestTrainDatasetRejectsCanceledContextBeforeUpdating(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	dataset := PreparedDataset{
		Training:   []TrainingRecord{{Tokens: []int32{1}, Targets: []int32{2}, LossMask: []bool{true}}},
		Validation: []TrainingRecord{{Tokens: []int32{1}, Targets: []int32{2}, LossMask: []bool{true}}},
	}
	_, err = runtime.TrainDataset(ctx, dataset, TrainingConfig{
		Epochs: 1, LearningRate: 1e-3, AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want canceled", err)
	}
}

func TestTrainDatasetReturnsCompletedEpochWhenCanceled(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}}
	ctx, cancel := context.WithCancel(t.Context())
	persisted := TrainingResult{}
	result, err := runtime.TrainDataset(ctx, PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{
		Epochs: 2, LearningRate: 1e-3, AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
		PersistCheckpoint: func(result TrainingResult) error {
			persisted = result
			cancel()
			return nil
		},
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TrainDataset() error = %v, want cancellation", err)
	}
	if len(result.Epochs) != 1 || result.BestEpoch != 1 || result.BestCheckpoint == "" {
		t.Fatalf("partial result = %+v, want one resumable epoch", result)
	}
	if persisted.BestCheckpoint != result.BestCheckpoint {
		t.Fatalf("persisted result = %+v, returned result = %+v", persisted, result)
	}
	if info, statErr := os.Stat(result.BestCheckpoint); statErr != nil || !info.IsDir() {
		t.Fatalf("completed checkpoint is unavailable: %v", statErr)
	}
}

func TestTrainDatasetDoesNotDeleteCheckpointBeforePersistingPrunedState(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}}
	var previous string
	_, err = runtime.TrainDataset(t.Context(), PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{
		Epochs: 2, LearningRate: 1e-3, AdapterCheckpointDirectory: filepath.Join(t.TempDir(), "checkpoints"),
		PersistCheckpoint: func(result TrainingResult) error {
			if previous != "" {
				if _, statErr := os.Stat(previous); statErr != nil {
					t.Fatalf("checkpoint was deleted before replacement state was persisted: %v", statErr)
				}
			}
			previous = result.BestCheckpoint
			return nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidationDoesNotUpdateAdapter(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	record := TrainingRecord{
		Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true},
	}
	if _, err := runtime.TrainStep(record.Tokens, record.Targets, record.LossMask, 1e-3); err != nil {
		t.Fatal(err)
	}
	adapterBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})
	if _, err := runtime.evaluate([]TrainingRecord{record}); err != nil {
		t.Fatal(err)
	}
	adapterAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})
	if !equalVariableSnapshots(adapterBefore, adapterAfter) {
		t.Fatal("validation changed LoRA weights")
	}
}

func TestTrainDatasetRejectsExistingCheckpointBeforeUpdating(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if _, err := runtime.Run([]int32{1, 4, 5}); err != nil {
		t.Fatal(err)
	}
	adapterBefore := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})
	checkpointRoot := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(filepath.Join(checkpointRoot, "epoch-01"), 0o700); err != nil {
		t.Fatal(err)
	}
	record := TrainingRecord{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}}
	_, err = runtime.TrainDataset(t.Context(), PreparedDataset{
		Training: []TrainingRecord{record}, Validation: []TrainingRecord{record},
	}, TrainingConfig{Epochs: 1, LearningRate: 1e-3, AdapterCheckpointDirectory: checkpointRoot}, nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v", err)
	}
	adapterAfter := snapshotVariables(t, runtime.base.store, func(variable *model.Variable) bool {
		return strings.Contains(variable.Path(), "/lora/")
	})
	if !equalVariableSnapshots(adapterBefore, adapterAfter) {
		t.Fatal("failed checkpoint preflight changed LoRA weights")
	}
}
