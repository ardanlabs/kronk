package gemma

import (
	"bytes"
	"cmp"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/checkpoint"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
)

func TestTrainDatasetPausePreservesOptimizerAndStopCancels(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	newRuntime := func(t *testing.T) *Runtime {
		t.Helper()
		runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}
	dataset := PreparedDataset{Training: []TrainingRecord{
		{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
		{Tokens: []int32{1, 7, 8, 9}, Targets: []int32{7, 8, 9, 10}, LossMask: []bool{false, false, true, true}},
	}}
	config := TrainingConfig{Epochs: 2, LearningRate: 1e-2, BatchSize: 1, Seed: 42, AdapterCheckpointDirectory: t.TempDir()}
	baseline := newRuntime(t)
	defer baseline.Close()
	want, err := baseline.TrainDataset(t.Context(), dataset, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter := filepath.Join(t.TempDir(), "uninterrupted.gguf")
	if err := baseline.ExportBestAdapter(want, adapter); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(adapter)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"resume", "stop"} {
		t.Run(action, func(t *testing.T) {
			runtime := newRuntime(t)
			defer runtime.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			paused, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var completed atomic.Int32
			cfg := config
			cfg.AdapterCheckpointDirectory = t.TempDir()
			var once atomic.Bool
			cfg.PausePoint = func(ctx context.Context) error {
				if completed.Load() == 1 && once.CompareAndSwap(false, true) {
					close(paused)
					select {
					case <-resume:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return ctx.Err()
			}
			var got TrainingResult
			var trainErr error
			go func() {
				defer close(done)
				got, trainErr = runtime.TrainDataset(ctx, dataset, cfg, func(p TrainingProgress) { completed.Store(int32(p.CompletedSteps)) })
			}()
			defer func() { cancel(); <-done }()
			select {
			case <-paused:
			case <-done:
				t.Fatalf("training finished instead of pausing between batches: %v", trainErr)
			case <-ctx.Done():
				t.Fatal("training did not reach a pause point")
			}
			select {
			case <-done:
				t.Fatal("training completed while paused")
			case <-time.After(100 * time.Millisecond):
			}
			if completed.Load() != 1 {
				t.Fatal("optimizer advanced while paused")
			}
			if action == "stop" {
				cancel()
				<-done
				if !errors.Is(trainErr, context.Canceled) || completed.Load() != 1 || got.BestEpoch != 0 {
					t.Fatalf("Stop advanced or completed training: %+v %v", got, trainErr)
				}
				return
			}
			close(resume)
			<-done
			if trainErr != nil || len(got.Epochs) != len(want.Epochs) || completed.Load() != 4 {
				t.Fatalf("Resume lost training work: %+v, completed=%d, err=%v", got, completed.Load(), trainErr)
			}
			for index, metrics := range got.Epochs {
				if metrics.TrainingLoss != want.Epochs[index].TrainingLoss || metrics.ValidationLoss != want.Epochs[index].ValidationLoss {
					t.Fatalf("pausing changed epoch %d losses: %+v, want %+v", index+1, metrics, want.Epochs[index])
				}
			}
			output := filepath.Join(t.TempDir(), "resumed.gguf")
			if err := runtime.ExportBestAdapter(got, output); err != nil {
				t.Fatal(err)
			}
			gotBytes, err := os.ReadFile(output)
			if err != nil || !bytes.Equal(gotBytes, wantBytes) {
				t.Fatalf("resumed adapter differs from uninterrupted training: %v", err)
			}
		})
	}
}

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

func TestTrainingRestartExactlyMatchesUninterrupted(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "gemma.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	dataset := PreparedDataset{Training: []TrainingRecord{
		{Tokens: []int32{1, 4, 5}, Targets: []int32{4, 5, 6}, LossMask: []bool{false, true, true}},
		{Tokens: []int32{1, 7}, Targets: []int32{7, 8}, LossMask: []bool{false, true}},
		{Tokens: []int32{1, 9, 10, 11}, Targets: []int32{9, 10, 11, 12}, LossMask: []bool{false, false, true, true}},
	}}
	dataset.Validation = dataset.Training[:1]
	newRuntime := func(t *testing.T) *Runtime {
		t.Helper()
		runtime, err := NewLoRARuntime(path, AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(runtime.Close)
		return runtime
	}
	config := TrainingConfig{Epochs: 4, LearningRate: 0.01, WeightDecay: 0.03, BatchSize: 2, Seed: 42,
		EarlyStoppingPatience: 2, MinimumValidationImprovement: 100, RetainedCheckpointEpochs: []int{1}, AdapterCheckpointDirectory: t.TempDir()}
	baseline := newRuntime(t)
	want, err := baseline.TrainDataset(t.Context(), dataset, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput := filepath.Join(t.TempDir(), "baseline.gguf")
	if err := baseline.ExportBestAdapter(want, wantOutput); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(wantOutput)
	if err != nil {
		t.Fatal(err)
	}
	wantVariables := trainingVariableBytes(t, baseline)
	// Initial, partial batch, final batch before validation, completed epoch,
	// next epoch, early-stop commit. Every safe boundary is restartable.
	for stopAt := 1; stopAt <= 10; stopAt++ {
		t.Run(fmt.Sprint(stopAt), func(t *testing.T) {
			cfg := config
			cfg.ResumeDirectory, cfg.AdapterCheckpointDirectory = t.TempDir(), t.TempDir()
			calls := 0
			stop := errors.New("graceful stop")
			cfg.SafePoint = func(_ context.Context, relative string) error {
				if !filepath.IsLocal(relative) {
					t.Fatalf("not relative: %q", relative)
				}
				if _, err := os.Stat(filepath.Join(cfg.ResumeDirectory, relative)); err != nil {
					t.Fatal(err)
				}
				if _, _, err := checkpoint.ReadTraining(cfg.ResumeDirectory); err != nil {
					t.Fatal(err)
				}
				calls++
				if calls == stopAt {
					return stop
				}
				return nil
			}
			first := newRuntime(t)
			_, err := first.TrainDataset(t.Context(), dataset, cfg, nil)
			if !errors.Is(err, stop) {
				t.Fatalf("did not stop at boundary %d: %v (calls=%d)", stopAt, err, calls)
			}
			first.Close() // All optimizer/graph state is gone, as on server restart.
			cfg.SafePoint = nil
			restarted := newRuntime(t)
			got, err := restarted.TrainDataset(t.Context(), dataset, cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.BestEpoch != want.BestEpoch || got.StoppedEarly != want.StoppedEarly || len(got.Epochs) != len(want.Epochs) {
				t.Fatalf("result differs: %+v vs %+v", got, want)
			}
			for i := range got.Epochs {
				if got.Epochs[i].TrainingLoss != want.Epochs[i].TrainingLoss || got.Epochs[i].ValidationLoss != want.Epochs[i].ValidationLoss {
					t.Fatalf("epoch metrics differ: %+v vs %+v", got.Epochs, want.Epochs)
				}
			}
			if !reflect.DeepEqual(trainingVariableBytes(t, restarted), wantVariables) {
				t.Fatal("continued optimizer/adapter output differs")
			}
			output := filepath.Join(t.TempDir(), "resumed.gguf")
			if err := restarted.ExportBestAdapter(got, output); err != nil {
				t.Fatal(err)
			}
			gotBytes, err := os.ReadFile(output)
			if err != nil || !bytes.Equal(gotBytes, wantBytes) {
				t.Fatalf("export differs: %v", err)
			}
		})
	}
}

func trainingVariableBytes(t *testing.T, runtime *Runtime) map[string][]byte {
	t.Helper()
	values := make(map[string][]byte)
	for variable := range runtime.base.store.IterVariables() {
		if runtime.base.frozen[variable.Path()] || strings.HasPrefix(variable.Path(), "/metrics/") {
			continue
		}
		var buffer bytes.Buffer
		if err := variable.MustValue().GobSerialize(gob.NewEncoder(&buffer)); err != nil {
			t.Fatal(err)
		}
		values[variable.Path()] = buffer.Bytes()
	}
	return values
}

func TestTrainingRestoreRejectsIncompatibleIdentity(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	for _, change := range []string{"model", "adapter", "optimizer", "dataset", "seed", "epochs"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "base.gguf")
			writeTestModel(t, path, testConfig(), "gemma3")
			adapter := AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407}
			dataset := PreparedDataset{Training: []TrainingRecord{{Tokens: []int32{1, 4}, Targets: []int32{4, 5}, LossMask: []bool{false, true}}}}
			stop := errors.New("stop")
			config := TrainingConfig{Epochs: 2, LearningRate: 0.01, BatchSize: 1,
				AdapterCheckpointDirectory: t.TempDir(), ResumeDirectory: t.TempDir(),
				SafePoint: func(context.Context, string) error { return stop }}
			first, err := NewLoRARuntime(path, adapter)
			if err != nil {
				t.Fatal(err)
			}
			_, err = first.TrainDataset(t.Context(), dataset, config, nil)
			first.Close()
			if !errors.Is(err, stop) {
				t.Fatal(err)
			}
			switch change {
			case "model":
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := file.Write([]byte("changed model identity"))
				if err := errors.Join(writeErr, file.Close()); err != nil {
					t.Fatal(err)
				}
			case "adapter":
				adapter.Alpha++
			case "optimizer":
				config.LearningRate *= 2
			case "dataset":
				dataset.Training[0].Targets[1]++
			case "seed":
				config.Seed++
			case "epochs":
				config.Epochs++
			}
			config.SafePoint = nil
			second, err := NewLoRARuntime(path, adapter)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			if _, err := second.TrainDataset(t.Context(), dataset, config, nil); err == nil || !strings.Contains(err.Error(), "identity differs") {
				t.Fatalf("accepted incompatible %s: %v", change, err)
			}
		})
	}
}

func TestTrainingRestartAfterUncommittedEpochExport(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	path := filepath.Join(t.TempDir(), "base.gguf")
	writeTestModel(t, path, testConfig(), "gemma3")
	adapter := AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407}
	dataset := PreparedDataset{Training: []TrainingRecord{{Tokens: []int32{1, 4}, Targets: []int32{4, 5}, LossMask: []bool{false, true}}}}
	interrupted := errors.New("crash after inference export but before epoch commit")
	config := TrainingConfig{Epochs: 2, LearningRate: 0.01, BatchSize: 1,
		AdapterCheckpointDirectory: t.TempDir(), ResumeDirectory: t.TempDir(),
		PersistCheckpoint: func(TrainingResult) error { return interrupted }}
	first, err := NewLoRARuntime(path, adapter)
	if err != nil {
		t.Fatal(err)
	}
	_, err = first.TrainDataset(t.Context(), dataset, config, nil)
	first.Close()
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.AdapterCheckpointDirectory, "epoch-01")); err != nil {
		t.Fatal(err)
	}
	config.PersistCheckpoint = nil
	second, err := NewLoRARuntime(path, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	updates := 0
	result, err := second.TrainDataset(t.Context(), dataset, config, func(TrainingProgress) { updates++ })
	if err != nil {
		t.Fatal(err)
	}
	if updates != 1 || len(result.Epochs) != 2 {
		t.Fatalf("replayed committed optimizer batch or lost epochs: updates=%d result=%+v", updates, result)
	}
	second.Close()
	third, err := NewLoRARuntime(path, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	completed, err := third.TrainDataset(t.Context(), dataset, config, func(TrainingProgress) { t.Fatal("replayed completed run") })
	if err != nil || !reflect.DeepEqual(completed, result) {
		t.Fatalf("completed run did not round-trip: %+v %v", completed, err)
	}
}

func TestTrainingRestoreFailsClosedOnCorruption(t *testing.T) {
	t.Setenv("GOMLX_BACKEND", "go")
	for _, target := range []string{"generation", "best-export"} {
		t.Run(target, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "base.gguf")
			writeTestModel(t, path, testConfig(), "gemma3")
			adapter := AdapterConfig{Rank: 2, Alpha: 4, AdapterScale: 1, Seed: 3407}
			dataset := PreparedDataset{Training: []TrainingRecord{{Tokens: []int32{1, 4}, Targets: []int32{4, 5}, LossMask: []bool{false, true}}}}
			config := TrainingConfig{Epochs: 2, LearningRate: 0.01, BatchSize: 1,
				AdapterCheckpointDirectory: t.TempDir(), ResumeDirectory: t.TempDir()}
			calls := 0
			generation := ""
			config.SafePoint = func(_ context.Context, relative string) error {
				calls++
				if calls == 3 {
					generation = filepath.Join(config.ResumeDirectory, relative)
					return context.Canceled
				}
				return nil
			}
			first, err := NewLoRARuntime(path, adapter)
			if err != nil {
				t.Fatal(err)
			}
			_, err = first.TrainDataset(t.Context(), dataset, config, nil)
			first.Close()
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			corrupt := generation
			if target == "best-export" {
				files, err := filepath.Glob(filepath.Join(config.AdapterCheckpointDirectory, "epoch-01", "*.bin"))
				if err != nil || len(files) != 1 {
					t.Fatalf("checkpoint payloads: %v %v", files, err)
				}
				corrupt = files[0]
			}
			data, err := os.ReadFile(corrupt)
			if err != nil {
				t.Fatal(err)
			}
			data[len(data)/2] ^= 1
			if err := os.WriteFile(corrupt, data, 0o600); err != nil {
				t.Fatal(err)
			}
			config.SafePoint = func(context.Context, string) error { t.Fatal("acknowledged corrupt state"); return nil }
			second, err := NewLoRARuntime(path, adapter)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			if _, err := second.TrainDataset(t.Context(), dataset, config, nil); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("corrupt %s did not fail closed: %v", target, err)
			}
		})
	}
}
