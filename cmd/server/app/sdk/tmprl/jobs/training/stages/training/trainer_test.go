package training_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestGoMLXTrainerRequiresPool(t *testing.T) {
	trainer, err := training.NewGoMLXTrainer(nil)
	if trainer != nil || !errors.Is(err, training.ErrInvalidArgument) {
		t.Fatalf("NewGoMLXTrainer: got %v, %v, want nil, ErrInvalidArgument", trainer, err)
	}
}

// The adapter owns a concrete private provider; even failed catalog resolution
// consults native llama context defaults. This test therefore requires explicit
// KRONK_TMPRL_NATIVE_SMOKE=1 and KRONK_TMPRL_SMOKE_LIBS pointing to compatible
// installed llama.cpp libraries. It loads libraries, never a model, and downloads
// or trains nothing. Run with GITHUB_WORKSPACE set to the repository root.
// It proves fail-closed delegation, not successful translation or target privacy.
func TestGoMLXTrainerMissingModel(t *testing.T) {
	initNativeTrainer(t)
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rm, err := resman.New(resman.Config{Snapshot: resman.Snapshot{RAMBytes: 1 << 30}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := pool.New(pool.Config{
		Log: applog.DiscardLogger, Models: mdls, Resman: rm,
		StartupDevices: &devices.Devices{},
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupTrainerPool(t, p)
	trainer, err := training.NewGoMLXTrainer(p)
	if err != nil {
		t.Fatal(err)
	}
	var api training.Trainer = trainer
	for _, operation := range []string{"Validate", "Train", "Predict base", "Predict adapter"} {
		t.Run(operation, func(t *testing.T) {
			record := training.Record{Prompt: "Question?", Target: "held-out answer", Evidence: "private evidence", FactID: "fact", IndependentPrompt: true}
			job := training.TrainingJob{
				ModelID: "missing/model", Model: filepath.Join(t.TempDir(), "absent.gguf"),
				Workdir: t.TempDir(), Training: training.Training{Seed: 53, MaximumEpochs: 1},
				Dataset: training.TrainingDataset{Test: []training.Record{record}},
			}
			records := []training.Record{record}
			var err error
			switch operation {
			case "Validate":
				err = api.Validate(t.Context(), job)
			case "Train":
				var candidate training.TrainingCandidate
				candidate, err = api.Train(t.Context(), job, func(tmprl.Progress) {
					t.Error("progress published before acquiring missing model")
				})
				if candidate != (training.TrainingCandidate{}) {
					t.Errorf("Train: got %+v, want zero candidate", candidate)
				}
			default:
				adapter := ""
				if operation == "Predict adapter" {
					adapter = filepath.Join(job.Workdir, "absent-adapter.gguf")
				}
				var answers []string
				answers, err = api.Predict(t.Context(), job, adapter, records)
				if answers != nil {
					t.Errorf("Predict: got %q, want nil answers", answers)
				}
			}
			if !errors.Is(err, models.ErrModelNotFound) {
				t.Errorf("%s: got %v, want wrapped ErrModelNotFound", operation, err)
			}
			if !reflect.DeepEqual(records, []training.Record{record}) || !reflect.DeepEqual(job.Dataset.Test, []training.Record{record}) {
				t.Errorf("inputs: got %+v, %+v, want unchanged records", records, job.Dataset)
			}
			entries, err := os.ReadDir(job.Workdir)
			if err != nil || len(entries) != 0 {
				t.Errorf("workdir: got %v, %v, want empty directory, nil", entries, err)
			}
		})
	}
}

// TestGoMLXTrainerNativeSuccess uses the same KRONK_TMPRL_NATIVE_SMOKE=1,
// KRONK_TMPRL_SMOKE_BASE, KRONK_TMPRL_SMOKE_LIBS and KRONK_TMPRL_SMOKE_MODEL
// configuration as the providers smoke test. It requires compatible installed
// libraries and an existing indexed, small single-file GGUF chat model that fits
// CPU RAM with 1024 context tokens. No downloads, training or adapter creation run.
func TestGoMLXTrainerNativeSuccess(t *testing.T) {
	initNativeTrainer(t)
	base, modelID := os.Getenv("KRONK_TMPRL_SMOKE_BASE"), os.Getenv("KRONK_TMPRL_SMOKE_MODEL")
	if base == "" || modelID == "" {
		t.Fatal("KRONK_TMPRL_SMOKE_BASE and KRONK_TMPRL_SMOKE_MODEL are required")
	}
	if _, err := os.Stat(filepath.Join(base, "models", ".index.yaml")); err != nil {
		t.Fatalf("existing model index: %v", err)
	}
	mdls, err := models.NewWithPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	hardware := devices.List()
	rm, err := resman.New(resman.Config{Snapshot: resman.FromDevices(hardware)})
	if err != nil {
		t.Fatal(err)
	}
	p, err := pool.New(pool.Config{
		Log: applog.DiscardLogger, Models: mdls, Resman: rm, StartupDevices: &hardware,
		ModelsInPool: 1,
		ModelConfig: map[string]models.ModelConfig{modelID: {
			PtrContextWindow: new(1024), PtrNGpuLayers: new(-1), PtrNSeqMax: new(1),
			PtrIncrementalCache: new(false), PtrInsecureLogging: new(false),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupTrainerPool(t, p)
	cfg, err := p.ResolvedKronkConfig(modelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModelFiles) != 1 {
		t.Fatalf("ModelFiles: got %v, want one GGUF", cfg.ModelFiles)
	}
	trainer, err := training.NewGoMLXTrainer(p)
	if err != nil {
		t.Fatal(err)
	}
	job := training.TrainingJob{
		ModelID: modelID, Model: cfg.ModelFiles[0], Workdir: t.TempDir(),
		Training: training.Training{MaximumEpochs: 2, BatchSize: 3, LearningRate: 0.0002, Seed: 53, LoRARank: 4, LoRAAlpha: 8},
		Dataset: training.TrainingDataset{
			Train: []training.Record{{Prompt: "Say hello.", Target: "Hello.", Evidence: "Training evidence: Hello.", FactID: "train", SourceDocument: "train.pdf", SourceChunkID: "t-1", UserApproved: true}},
			Validation: []training.Record{
				{Prompt: "Say goodbye.", Target: "Goodbye.", Evidence: "Validation evidence: Goodbye.", FactID: "validation-1", SourceDocument: "validation.docx", SourceChunkID: "v-1", ValidationPrompt: true},
				{Prompt: "Name a color.", Target: "Blue.", Evidence: "Another validation fact: Blue.", FactID: "validation-2", SourceDocument: "colors.pdf", SourceChunkID: "v-2", ValidationPrompt: true},
			},
			Test: []training.Record{{Prompt: "Say OK.", Target: "Protected target.", Evidence: "Private test evidence.", FactID: "test", SourceDocument: "test.pdf", SourceChunkID: "p-1", IndependentPrompt: true}},
		},
	}
	wantJob := job
	wantJob.Dataset = training.TrainingDataset{Train: slices.Clone(job.Dataset.Train), Validation: slices.Clone(job.Dataset.Validation), Test: slices.Clone(job.Dataset.Test)}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	if err := trainer.Validate(ctx, job); err != nil {
		t.Fatalf("Validate: got %v, want nil", err)
	}
	records := []training.Record{
		{Prompt: "Reply with only the word hello.", Target: "Secret greeting target.", Evidence: "Private greeting evidence.", FactID: "greeting", SourceDocument: "greeting.pdf", IndependentPrompt: true},
		{Prompt: "Reply with only the number 2.", Target: "Secret number target.", Evidence: "Private number evidence.", FactID: "number", SourceChunkID: "n-2", IndependentPrompt: true},
		// This Gemma question previously exhausted the 192-token answer cap.
		{Prompt: "What is the cost for mooring?", Target: "Private mooring target.", Evidence: "Private mooring evidence.", FactID: "mooring", IndependentPrompt: true},
	}
	wantRecords := slices.Clone(records)
	answers, err := trainer.Predict(ctx, job, "", records)
	if err != nil {
		t.Fatalf("Predict base: got %v, want nil", err)
	}
	if len(answers) != len(records) {
		t.Errorf("Predict base: got %d answers, want %d", len(answers), len(records))
	}
	for i, answer := range answers {
		if strings.TrimSpace(answer) == "" {
			t.Errorf("answer %d: got %q, want nonempty content", i, answer)
		}
	}
	if !reflect.DeepEqual(job, wantJob) || !reflect.DeepEqual(records, wantRecords) {
		t.Errorf("inputs: got %+v, %+v, want %+v, %+v", job, records, wantJob, wantRecords)
	}
	entries, err := os.ReadDir(job.Workdir)
	if err != nil || len(entries) != 0 {
		t.Errorf("workdir: got %v, %v, want empty directory, nil", entries, err)
	}
	t.Logf("native stage adapter: Validate succeeded; base Predict returned %q", answers)
}

func initNativeTrainer(t *testing.T) {
	t.Helper()
	switch value := os.Getenv("KRONK_TMPRL_NATIVE_SMOKE"); value {
	case "":
		t.Skip("set KRONK_TMPRL_NATIVE_SMOKE=1 and KRONK_TMPRL_SMOKE_LIBS for native context defaults")
	case "1":
	default:
		t.Fatalf("KRONK_TMPRL_NATIVE_SMOKE: got %q, want 1 or unset", value)
	}
	libPath := os.Getenv("KRONK_TMPRL_SMOKE_LIBS")
	if libPath == "" {
		t.Fatal("KRONK_TMPRL_SMOKE_LIBS is required")
	}
	if err := kronk.Init(kronk.WithLibPath(libPath)); err != nil {
		t.Fatalf("native init: %v", err)
	}
}

func cleanupTrainerPool(t *testing.T, p *pool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := p.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: got %v, want nil", err)
		}
	})
}
