package tuning_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tuning"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

// TestTemporalRealGemmaTraining exercises the default trainer, not a test
// replacement. Model quality is reported separately from pipeline correctness.
// Run explicitly with MOXIE_TEST_GEMMA_MODEL and TEMPORAL_ADDRESS set.
// Optional MOXIE_TEST_DOCUMENT and MOXIE_TEST_PREPARATION_ENDPOINT exercise
// document import and preparation first. All local outputs live in t.TempDir.
// MOXIE_TEST_PREPARATION_MODEL selects the preparation model (default gemma);
// the trained and evaluated base remains MOXIE_TEST_GEMMA_MODEL.
func TestTemporalRealGemmaTraining(t *testing.T) {
	model := os.Getenv("MOXIE_TEST_GEMMA_MODEL")
	if model == "" {
		t.Skip("set MOXIE_TEST_GEMMA_MODEL to a supported Gemma 3 GGUF")
	}
	address := os.Getenv("TEMPORAL_ADDRESS")
	if address == "" {
		t.Fatal("TEMPORAL_ADDRESS is required for the real-model test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	hashModel := func() [sha256.Size]byte {
		t.Helper()
		file, err := os.Open(model)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			t.Fatal(err)
		}
		return [sha256.Size]byte(hash.Sum(nil))
	}
	before := hashModel()
	t.Cleanup(func() {
		if after := hashModel(); after != before {
			t.Error("training changed the frozen base GGUF")
		}
	})
	c, err := client.Dial(client.Options{HostPort: address, Namespace: os.Getenv("TEMPORAL_NAMESPACE")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	cfg := Config{TaskQueue: "moxie-real-training-" + rand.Text(), DatasetRoot: t.TempDir(), Models: map[string]string{"gemma": model}, LlamaServer: os.Getenv("MOXIE_TEST_LLAMA_SERVER"), StageTimeout: 25 * time.Minute}
	document := os.Getenv("MOXIE_TEST_DOCUMENT")
	if document != "" {
		cfg.Extractor, err = NewDoclingExtractor(os.Getenv("MOXIE_TEST_DOCLING_ENDPOINT"), nil)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Completer, err = NewHTTPCompleter(os.Getenv("MOXIE_TEST_PREPARATION_ENDPOINT"), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := New(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	w, err := b.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	request := TrainingRequest{Model: "gemma", Training: Training{MaximumEpochs: 1, BatchSize: 1, LearningRate: 0.0002, Seed: 3407, LoRARank: 8, LoRAAlpha: 8}}
	if document != "" {
		file, err := os.Open(document)
		if err != nil {
			t.Fatal(err)
		}
		dataset, err := b.ImportDataset(ctx, ImportInput{Name: filepath.Base(document)}, file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		preparationModel := os.Getenv("MOXIE_TEST_PREPARATION_MODEL")
		if preparationModel == "" {
			preparationModel = "gemma"
		}
		input := PreparationRequest{DatasetID: dataset.ID, Model: preparationModel, Goal: GoalQuestionAnswer, PreviewRecords: 3}
		for _, preview := range []bool{true, false} {
			var run PreparationRun
			if preview {
				run, err = b.StartPreview(ctx, input)
			} else {
				input.PreviewRecords = 0
				run, err = b.StartPreparation(ctx, input)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = b.StopPreparation(cleanup, run.Execution)
			})
			prepared, err := b.WaitPreparation(ctx, run.Execution)
			if err != nil {
				t.Fatalf("document %s failed: %v", run.Kind, err)
			}
			if prepared.Status != StatusCompleted || prepared.Result == nil || prepared.Result.RecordCount < 1 {
				t.Fatalf("invalid document preparation: %+v", prepared)
			}
			if len(prepared.Result.Warnings) > 0 {
				t.Fatalf("document %s requires review before continuing: %v", run.Kind, prepared.Result.Warnings)
			}
			t.Logf("document=%s kind=%s records=%d training_ready=%t warnings=%v", filepath.Base(document), run.Kind, prepared.Result.RecordCount, prepared.Result.TrainingReady, prepared.Result.Warnings)
			if !preview {
				if !prepared.Result.TrainingReady {
					t.Fatal("document preparation did not produce a training-ready dataset")
				}
				request.Preparation = prepared.Execution
			}
		}
	} else {
		var source strings.Builder
		for i := range 20 {
			target := fmt.Sprintf("C%02d", i)
			questions := []string{
				fmt.Sprintf("What is device %02d's calibration code? Reply with only the code.", i),
				fmt.Sprintf("Give the calibration code for device %02d, with no explanation.", i),
				fmt.Sprintf("Which calibration code belongs to device %02d? Output only its code.", i),
			}
			for split, prompt := range questions {
				if split == 1 && i >= 10 {
					continue
				}
				record := Record{Prompt: prompt, Target: target, Evidence: fmt.Sprintf("Device %02d has calibration code %s.", i, target), FactID: fmt.Sprintf("device-%02d", i), ValidationPrompt: split == 1, IndependentPrompt: split == 2}
				if err := json.NewEncoder(&source).Encode(record); err != nil {
					t.Fatal(err)
				}
			}
		}
		dataset, err := b.ImportDataset(ctx, ImportInput{Name: "calibration.jsonl"}, strings.NewReader(source.String()))
		if err != nil {
			t.Fatal(err)
		}
		request.DatasetID = dataset.ID
	}
	start := time.Now()
	run, err := b.StartTraining(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("model=%s sha256=%x workflow=%s run=%s", model, before, run.WorkflowID, run.RunID)
	// Never leave a timed-out or failed test's workflow training in the background.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = b.Stop(cleanup, run.Execution)
	})
	result, err := b.Wait(ctx, run.Execution)
	if err != nil {
		t.Fatalf("real training failed: %v", err)
	}
	if result.Status != StatusCompleted && result.Status != StatusNeedsImprovement {
		t.Fatalf("unexpected terminal state: %+v", result)
	}
	report, err := b.GetTrainingEvaluation(ctx, run.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 20 || report.BaseCorrect < 0 || report.BaseCorrect > 20 || report.AdapterCorrect < 0 || report.AdapterCorrect > 20 {
		t.Fatalf("invalid measured evaluation: %+v", report)
	}
	if len(report.Answers) != 20 {
		t.Fatalf("missing real answer evidence: %d", len(report.Answers))
	}
	baseCorrect, adapterCorrect := 0, 0
	for _, answer := range report.Answers {
		t.Logf("question=%q expected=%q base=%q adapter=%q", answer.Prompt, answer.Expected, answer.BaseAnswer, answer.AdapterAnswer)
		expected := strings.ToLower(strings.Join(strings.Fields(answer.Expected), " "))
		if strings.ToLower(strings.Join(strings.Fields(answer.BaseAnswer), " ")) == expected {
			baseCorrect++
		}
		if strings.ToLower(strings.Join(strings.Fields(answer.AdapterAnswer), " ")) == expected {
			adapterCorrect++
		}
	}
	if report.BaseCorrect != baseCorrect || report.AdapterCorrect != adapterCorrect {
		t.Fatalf("reported scores disagree with actual answers: %+v", report)
	}
	wantAccepted := report.AdapterCorrect >= 18 && report.AdapterCorrect >= report.BaseCorrect
	if report.Accepted != wantAccepted || result.Artifacts.Accepted != wantAccepted || (result.Status == StatusCompleted) != wantAccepted {
		t.Fatalf("quality gate inconsistent: %+v %+v", result, report)
	}
	candidate, err := os.Open(result.Artifacts.Checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	var magic [4]byte
	if _, err := io.ReadFull(candidate, magic[:]); err != nil || string(magic[:]) != "GGUF" {
		t.Fatalf("candidate is not GGUF: %q %v", magic, err)
	}
	if wantAccepted {
		reader, err := b.OpenTrainingAdapter(ctx, run.Execution)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		candidateHash, publishedHash := sha256.New(), sha256.New()
		if _, err := candidate.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(candidateHash, candidate); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(publishedHash, reader); err != nil {
			t.Fatal(err)
		}
		if string(candidateHash.Sum(nil)) != string(publishedHash.Sum(nil)) {
			t.Fatal("published adapter differs from evaluated candidate")
		}
	} else {
		if result.Artifacts.Adapter != "" {
			t.Fatal("rejected candidate was published")
		}
		if _, err := b.OpenTrainingAdapter(ctx, run.Execution); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("rejected adapter is downloadable: %v", err)
		}
	}
	t.Logf("elapsed=%s status=%s base=%d/%d adapter=%d/%d accepted=%t", time.Since(start).Round(time.Second), result.Status, report.BaseCorrect, report.Total, report.AdapterCorrect, report.Total, report.Accepted)
	t.Run("quality", func(t *testing.T) {
		if !report.Accepted {
			t.Errorf("model quality failed: base=%d/%d adapter=%d/%d; require at least 90%% accuracy and no regression", report.BaseCorrect, report.Total, report.AdapterCorrect, report.Total)
		}
	})
}

func TestTemporalRealGemmaContextPreflight(t *testing.T) {
	model, address := os.Getenv("MOXIE_TEST_GEMMA_MODEL"), os.Getenv("TEMPORAL_ADDRESS")
	if model == "" || address == "" {
		t.Skip("requires MOXIE_TEST_GEMMA_MODEL and TEMPORAL_ADDRESS")
	}
	header, err := gguf.ReadHeaderBytes(model)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := gguf.ParseMetadata(header)
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := gguf.ParseInt64(metadata, "gemma3.context_length")
	if err != nil || capacity < 1 {
		t.Fatalf("invalid Gemma context metadata: %d %v", capacity, err)
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: os.Getenv("TEMPORAL_NAMESPACE")})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, err := New(c, Config{DatasetRoot: t.TempDir(), TaskQueue: "context-test-" + rand.Text(), Models: map[string]string{"gemma": model}, LlamaServer: os.Getenv("MOXIE_TEST_LLAMA_SERVER")})
	if err != nil {
		t.Fatal(err)
	}
	w, err := b.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	for split, name := range []string{"training", "validation", "protected test"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			source := strings.Replace(trainingFixture(t), fmt.Sprintf("Device 00 question variant %d?", split), strings.Repeat("hello ", int(capacity)+128)+"Question?", 1)
			dataset, err := b.ImportDataset(ctx, ImportInput{Name: "oversized.jsonl"}, strings.NewReader(source))
			if err != nil {
				t.Fatal(err)
			}
			run, err := b.StartTraining(ctx, TrainingRequest{DatasetID: dataset.ID, Model: "gemma"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = b.Wait(ctx, run.Execution)
			application, ok := errors.AsType[*temporal.ApplicationError](err)
			if !ok || !application.NonRetryable() || !strings.Contains(application.Error(), name+" record") {
				t.Fatalf("oversized input was not rejected before optimization: %v", err)
			}
			closed, err := b.Get(ctx, run.Execution)
			if err != nil || closed.Status != StatusFailed || closed.Artifacts != (Artifacts{}) {
				t.Fatalf("preflight committed training artifacts: %+v %v", closed, err)
			}
		})
	}
}
