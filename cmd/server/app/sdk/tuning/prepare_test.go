package tuning_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tuning"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/mocks"
	"go.temporal.io/sdk/temporal"
)

type completerFunc func(context.Context, CompletionRequest) (Completion, error)

func (f completerFunc) Complete(ctx context.Context, r CompletionRequest) (Completion, error) {
	return f(ctx, r)
}

func livePreparation(t *testing.T, completer ModelCompleter, configure ...func(*Config)) (*Business, func()) {
	t.Helper()
	address := os.Getenv("TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set TEMPORAL_ADDRESS to test preparation against real Temporal")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: os.Getenv("TEMPORAL_NAMESPACE")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	root := t.TempDir()
	cfg := Config{TaskQueue: "moxie-preparation-test-" + rand.Text(), DatasetRoot: root, Completer: completer, StageTimeout: 30 * time.Second}
	for _, option := range configure {
		option(&cfg)
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
	stop := sync.OnceFunc(w.Stop)
	t.Cleanup(stop)
	return b, stop
}

const preparationSource = "A goroutine is an independently executing function in Go."

func TestTemporalDatasetPreviewAndFullPreparation(t *testing.T) {
	var transient atomic.Bool
	var reject atomic.Bool
	b, stop := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
		if !transient.Swap(true) {
			return Completion{}, errors.New("temporary model outage")
		}
		if len(r.Messages) != 2 || !strings.Contains(r.Messages[1].Content, preparationSource) {
			return Completion{}, errors.New("source missing from model request")
		}
		if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
			if reject.Load() {
				return Completion{Content: `{"record_1":"grounding"}`}, nil
			}
			return Completion{Content: `{"record_1":"pass"}`, PromptTokens: 13, CompletionTokens: 2}, nil
		}
		return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":1}]}`, PromptTokens: 17, CompletionTokens: 5}, nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataset, err := b.ImportDataset(ctx, ImportInput{Name: "goroutines.md"}, strings.NewReader(preparationSource))
	if err != nil {
		t.Fatal(err)
	}
	input := PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer, PreviewRecords: 1}
	preview, err := b.StartPreview(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.WaitPreparation(ctx, preview.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || result.Result == nil || result.Result.DatasetID != dataset.ID || result.Result.RecordCount != 1 || len(result.Result.Preview) != 1 {
		t.Fatalf("preview: %+v", result)
	}
	if _, err := b.Wait(ctx, preview.Execution); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("training wait accepted preparation execution: %v", err)
	}
	row := result.Result.Preview[0]
	if row.Prompt != "What is a goroutine?" || row.Target != preparationSource || row.Evidence != preparationSource {
		t.Fatalf("source grounding: %+v", row)
	}
	if result.Result.PromptTokens != 30 || result.Result.CompletionTokens != 7 || len(result.Result.Warnings) != 0 {
		t.Fatalf("preview metrics: %+v", result.Result)
	}
	fullInput := PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalTextGeneration, ReviewedRecords: []Record{{Prompt: "Explain Go goroutines.", Target: preparationSource, Evidence: preparationSource}}}
	full, err := b.StartPreparation(ctx, fullInput)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := b.WaitPreparation(ctx, full.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Status != StatusCompleted || prepared.Result.RecordCount != 1 || prepared.Result.TrainingReady || len(prepared.Result.Warnings) == 0 || len(prepared.Result.Preview) != 0 {
		t.Fatalf("full dataset readiness: %+v", prepared)
	}
	reader, err := b.OpenPreparationRecords(ctx, full.Execution)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var record Record
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.Prompt != fullInput.ReviewedRecords[0].Prompt || record.Target != preparationSource || record.Evidence != preparationSource {
		t.Fatalf("persisted record: %+v", record)
	}
	if err := decoder.Decode(&record); !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected extra rows: %v", err)
	}
	// Completion is not approval: a rejected preview must expose its warning
	// to callers rather than silently becoming training-ready.
	reject.Store(true)
	warned, err := b.StartPreview(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	warned, err = b.WaitPreparation(ctx, warned.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if warned.Status != StatusCompleted || warned.Result == nil || len(warned.Result.Warnings) != 1 || warned.Result.TrainingReady {
		t.Fatalf("rejected preview lost its review requirement: %+v", warned)
	}
	stop()
	stored, err := b.GetPreparation(ctx, full.Execution)
	if err != nil || !reflect.DeepEqual(stored, prepared) {
		t.Fatalf("durable result without worker: %+v %v", stored, err)
	}
}

func TestTemporalPreparationCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	b, _ := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return Completion{}, ctx.Err()
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	dataset, err := b.ImportDataset(ctx, ImportInput{Name: "source.txt"}, strings.NewReader(preparationSource))
	if err != nil {
		t.Fatal(err)
	}
	run, err := b.StartPreview(ctx, PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer, PreviewRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := b.StopPreparation(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WaitPreparation(ctx, run.Execution); !temporal.IsCanceledError(err) {
		t.Fatalf("cancellation: %v", err)
	}
	stored, err := b.GetPreparation(ctx, run.Execution)
	if err != nil || stored.Status != StatusStopped || stored.Result != nil || stored.Request.DatasetID != dataset.ID {
		t.Fatalf("cancelled state: %+v %v", stored, err)
	}
}

func TestTemporalFullGenerationCheckpointAndReadiness(t *testing.T) {
	var outage atomic.Bool
	var evaluationOutage atomic.Bool
	var firstBatch atomic.Int32
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, []byte("controlled model"), 0o600); err != nil {
		t.Fatal(err)
	}
	trainer := &controlledTrainer{base: 17, adapted: 18, validationRows: 10}
	b, _ := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
		prompt := r.Messages[1].Content
		var output any
		switch {
		case strings.Contains(prompt, "[RECORD 1]"):
			outcomes := make(map[string]string)
			for i := range strings.Count(prompt, "[RECORD ") {
				outcomes[fmt.Sprintf("record_%d", i+1)] = "pass"
			}
			output = outcomes
			data, err := json.Marshal(output)
			return Completion{Content: string(data), PromptTokens: 13, CompletionTokens: 2}, err
		case strings.Contains(prompt, "Locked target:"):
			_, target, _ := strings.Cut(prompt, "Locked target: ")
			var index int
			if _, err := fmt.Sscanf(target, "Device %d", &index); err != nil {
				return Completion{}, err
			}
			if index == 5 && !evaluationOutage.Swap(true) {
				return Completion{}, errors.New("model outage after committed evaluation questions")
			}
			question := fmt.Sprintf("Which calibration code belongs to device %02d?", index)
			if strings.Contains(prompt, "Do not repeat: "+question) {
				question = fmt.Sprintf("For device %02d, what is its calibration code?", index)
			}
			output = map[string]string{"record_1": question}
		default:
			_, source, _ := strings.Cut(prompt, "SOURCE:\n")
			if strings.HasPrefix(source, "[PASSAGE 1]\nDevice 05") && !outage.Swap(true) {
				return Completion{}, errors.New("model outage after the first durable batch")
			}
			repeated := false
			if strings.HasPrefix(source, "[PASSAGE 1]\nDevice 00") {
				repeated = firstBatch.Add(1) > 1
			}
			var rows []map[string]any
			for i, section := range strings.Split(source, "[PASSAGE ")[1:] {
				_, text, _ := strings.Cut(section, "]\n")
				var index int
				if _, err := fmt.Sscanf(text, "Device %d", &index); err != nil {
					return Completion{}, err
				}
				question := fmt.Sprintf("What calibration code applies to device %02d?", index)
				if repeated {
					question = fmt.Sprintf("What is device %02d's regenerated calibration question?", index)
				}
				rows = append(rows, map[string]any{"prompt": question, "evidence_id": i + 1, "target_id": 1})
			}
			output = map[string]any{"records": rows}
		}
		data, err := json.Marshal(output)
		return Completion{Content: string(data), PromptTokens: 17, CompletionTokens: 5}, err
	}), func(cfg *Config) { cfg.Models = map[string]string{"gemma": model}; cfg.Trainer = trainer })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var source strings.Builder
	for i := range 20 {
		fmt.Fprintf(&source, "Device %02d has calibration code C%02d.\n\n", i, i)
	}
	dataset, err := b.ImportDataset(ctx, ImportInput{Name: "devices.md"}, strings.NewReader(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	run, err := b.StartPreparation(ctx, PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer})
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.WaitPreparation(ctx, run.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || !result.Result.TrainingReady || result.Result.RecordCount != 50 || len(result.Result.Preview) != 0 || len(result.Result.Warnings) != 0 {
		t.Fatalf("full preparation: %+v", result)
	}
	if result.Result.PromptTokens != 1020 || result.Result.CompletionTokens != 238 {
		t.Fatalf("durable token totals: %+v", result.Result)
	}
	reader, err := b.OpenPreparationRecords(ctx, run.Execution)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	train, validation, test := 0, 0, 0
	for {
		var row Record
		err := decoder.Decode(&row)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var index int
		if _, err := fmt.Sscanf(row.Target, "Device %d", &index); err != nil {
			t.Fatal(err)
		}
		if row.Target != fmt.Sprintf("Device %02d has calibration code C%02d.", index, index) || row.Evidence != row.Target || row.SourceDocument != dataset.ID || row.FactID == "" {
			t.Fatalf("lost grounding/provenance: %+v", row)
		}
		switch {
		case row.ValidationPrompt:
			validation++
			if row.Prompt != fmt.Sprintf("Which calibration code belongs to device %02d?", index) {
				t.Fatalf("committed validation question changed: %+v", row)
			}
		case row.IndependentPrompt:
			test++
		default:
			train++
			if row.Prompt != fmt.Sprintf("What calibration code applies to device %02d?", index) {
				t.Fatalf("committed training record regenerated: %+v", row)
			}
		}
	}
	if train != 20 || validation != 10 || test != 20 {
		t.Fatalf("split flags: %d/%d/%d", train, validation, test)
	}
	training, err := b.StartTraining(ctx, TrainingRequest{Preparation: run.Execution, Model: "gemma"})
	if err != nil {
		t.Fatal(err)
	}
	trained, err := b.Wait(ctx, training.Execution)
	if err != nil || trained.Status != StatusCompleted || trained.Artifacts.Adapter == "" {
		t.Fatalf("preparation-to-training bridge: %+v %v", trained, err)
	}
	preview, err := b.StartPreview(ctx, PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer, PreviewRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.WaitPreparation(ctx, preview.Execution); err != nil {
		t.Fatal(err)
	}
	wantKinds := map[Execution]PreparationKind{run.Execution: PreparationFull, preview.Execution: PreparationPreview}
	// Visibility indexing is eventually consistent; do not assume an empty namespace.
	for {
		found := make(map[Execution]bool)
		var token []byte
		for {
			runs, next, err := b.PreparationHistory(ctx, 1, token)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) > 1 {
				t.Fatalf("preparation history exceeded page size: %d", len(runs))
			}
			for _, e := range runs {
				if e == training.Execution {
					t.Fatal("preparation history included training")
				}
				stored, err := b.GetPreparation(ctx, e)
				if err != nil {
					t.Fatal(err)
				}
				if kind, ok := wantKinds[e]; ok {
					if stored.Execution != e || stored.Kind != kind || stored.Status != StatusCompleted || stored.Request.DatasetID != dataset.ID || stored.Result == nil {
						t.Fatalf("preparation history details: %+v", stored)
					}
					found[e] = true
				}
			}
			if len(found) == len(wantKinds) || len(next) == 0 {
				break
			}
			token = next
		}
		if len(found) == len(wantKinds) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("preview or full preparation missing from history")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestTemporalRejectsUngroundedAndInvalidModelRecords(t *testing.T) {
	for _, mode := range []string{"bad-target", "rejected-review"} {
		t.Run(mode, func(t *testing.T) {
			b, _ := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
				if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
					return Completion{Content: `{"record_1":"grounding"}`}, nil
				}
				return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":99}]}`}, nil
			}))
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			dataset, err := b.ImportDataset(ctx, ImportInput{Name: "source.md"}, strings.NewReader(preparationSource))
			if err != nil {
				t.Fatal(err)
			}
			input := PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer}
			if mode == "rejected-review" {
				input.ReviewedRecords = []Record{{Prompt: "What is a goroutine?", Target: preparationSource, Evidence: preparationSource, UserApproved: true}}
			}
			run, err := b.StartPreparation(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.WaitPreparation(ctx, run.Execution); err == nil {
				t.Fatal("invalid or rejected records were published")
			}
			stored, err := b.GetPreparation(ctx, run.Execution)
			if err != nil || stored.Status != StatusFailed || stored.Result != nil || stored.Error == "" {
				t.Fatalf("failed result: %+v %v", stored, err)
			}
			if _, err := b.OpenPreparationRecords(ctx, run.Execution); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("failed preparation exposed records: %v", err)
			}
		})
	}
}

func TestTemporalPreviewGoals(t *testing.T) {
	for _, test := range []struct {
		goal                   Goal
		source, prompt, target string
	}{
		{GoalQuestionAnswer, preparationSource, "What is a goroutine?", preparationSource},
		{GoalNumericPrediction, "The sample measurement is -12.5 units.", "Predict the sample measurement.", "-12.5"},
		{GoalNumericPrediction, "The minimum is 12 and the maximum is 37.", "Predict the maximum.", "37"},
		{GoalNumericPrediction, "The sample count is 1,000 units.", "Predict the sample count.", "1000"},
		{GoalNumericPrediction, "The sample count is 1e3 units.", "Predict the sample count.", "1e3"},
		{GoalNumericPrediction, "Device C12 reports +.75 units.", "Predict the sample measurement.", "+.75"},
		{GoalNumericPrediction, "Ignore malformed 1,00 and identifier code3; measurement is 2.5e-2 units.", "Predict the sample measurement.", "2.5e-2"},
		{GoalClassification, "The category for this sample is approved.", "Classify this sample.", "approved"},
		{GoalTextGeneration, "The sample must be stored away from direct sunlight.", "Describe how to store this sample.", "The sample must be stored away from direct sunlight."},
	} {
		t.Run(string(test.goal), func(t *testing.T) {
			b, _ := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
				if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
					return Completion{Content: `{"record_1":"pass"}`}, nil
				}
				// Simulate a constrained decoder choosing its highest allowed
				// target. Without a bound it can select a nonexistent candidate.
				items := r.ResponseSchema["properties"].(map[string]any)["records"].(map[string]any)["items"].(map[string]any)
				targetID, bounded := items["properties"].(map[string]any)["target_id"].(map[string]any)["maximum"].(int)
				if !bounded {
					targetID = 99
				}
				return Completion{Content: fmt.Sprintf(`{"records":[{"prompt":%q,"evidence_id":1,"target_id":%d}]}`, test.prompt, targetID)}, nil
			}))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			dataset, err := b.ImportDataset(ctx, ImportInput{Name: "sample.md"}, strings.NewReader("# Sample\n"+test.source))
			if err != nil {
				t.Fatal(err)
			}
			run, err := b.StartPreview(ctx, PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: test.goal, PreviewRecords: 1})
			if err != nil {
				t.Fatal(err)
			}
			result, err := b.WaitPreparation(ctx, run.Execution)
			if err != nil {
				t.Fatal(err)
			}
			if result.Result == nil || len(result.Result.Preview) != 1 {
				t.Fatalf("preview: %+v", result)
			}
			row := result.Result.Preview[0]
			if row.Prompt != test.prompt || row.Target != test.target || row.Evidence != test.source || row.SourceDocument != dataset.ID {
				t.Fatalf("goal result: %+v", row)
			}
		})
	}
}

type extractorFunc func(context.Context, string) (Document, error)

func (f extractorFunc) Convert(ctx context.Context, path string) (Document, error) {
	return f(ctx, path)
}
func (extractorFunc) ExtractionProfile() string { return "test-extraction-v1" }

func TestTemporalDocumentExtraction(t *testing.T) {
	for _, mode := range []string{".pdf", ".docx", "replacement", "invalid", "missing"} {
		t.Run(mode, func(t *testing.T) {
			var conversions atomic.Int32
			var outage atomic.Bool
			var snapshot atomic.Pointer[string]
			var originalSource atomic.Pointer[string]
			var root string
			extractor := extractorFunc(func(ctx context.Context, path string) (Document, error) {
				conversions.Add(1)
				if mode == "replacement" {
					if err := os.WriteFile(*originalSource.Load(), []byte("replaced source"), 0o600); err != nil {
						return Document{}, err
					}
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "document fixture" {
					return Document{}, fmt.Errorf("wrong extracted source: %q %v", data, err)
				}
				snapshot.Store(new(path))
				if mode == "invalid" {
					return Document{}, nil
				}
				return Document{Markdown: preparationSource, StructuredJSON: json.RawMessage(`{"texts":[{"text":"Extracted source"}]}`)}, nil
			})
			b, _ := livePreparation(t, completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
				if mode != "replacement" && !outage.Swap(true) {
					return Completion{}, errors.New("outage after extraction")
				}
				if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
					return Completion{Content: `{"record_1":"pass"}`}, nil
				}
				return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":1}]}`}, nil
			}), func(cfg *Config) {
				root = cfg.DatasetRoot
				if mode != "missing" {
					cfg.Extractor = extractor
				}
			})
			ext := ".pdf"
			if mode == ".docx" {
				ext = mode
			}
			dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "source" + ext}, strings.NewReader("document fixture"))
			if err != nil {
				t.Fatal(err)
			}
			originalSource.Store(new(filepath.Join(root, "imports", dataset.ID, "source"+ext)))
			run, err := b.StartPreparation(t.Context(), PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer})
			if err != nil {
				t.Fatal(err)
			}
			result, err := b.WaitPreparation(t.Context(), run.Execution)
			if mode == "invalid" || mode == "missing" {
				if err == nil {
					t.Fatal("invalid extraction published records")
				}
				stored, err := b.GetPreparation(t.Context(), run.Execution)
				if err != nil || stored.Status != StatusFailed || stored.Result != nil {
					t.Fatalf("failed extraction: %+v %v", stored, err)
				}
				return
			}
			if err != nil || result.Result == nil || result.Result.RecordCount != 1 || conversions.Load() != 1 {
				t.Fatalf("extraction/retry: %+v conversions=%d %v", result, conversions.Load(), err)
			}
			reader, err := b.OpenPreparationRecords(t.Context(), run.Execution)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var record Record
			if err := json.NewDecoder(reader).Decode(&record); err != nil {
				t.Fatal(err)
			}
			if record.Target != preparationSource || record.Evidence != preparationSource {
				t.Fatalf("extracted grounding lost: %+v", record)
			}
			if _, err := os.Stat(*snapshot.Load()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("private source snapshot leaked: %v", err)
			}
		})
	}
}

func TestTemporalPreparationStorageRecovery(t *testing.T) {
	address := os.Getenv("TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set TEMPORAL_ADDRESS to test storage recovery")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: os.Getenv("TEMPORAL_NAMESPACE")})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, mode := range []string{"transient", "corrupt", "escaping-workspace"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			root := t.TempDir()
			b, err := New(c, Config{DatasetRoot: root, TaskQueue: "storage-test-" + rand.Text(), Completer: completerFunc(func(ctx context.Context, r CompletionRequest) (Completion, error) {
				if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
					return Completion{Content: `{"record_1":"pass"}`}, nil
				}
				return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":1}]}`}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			dataset, err := b.ImportDataset(ctx, ImportInput{Name: "source.txt"}, strings.NewReader(preparationSource))
			if err != nil {
				t.Fatal(err)
			}
			// Submit before starting the worker so the fault is guaranteed to be
			// present on the activity's first source lookup, not on submission.
			run, err := b.StartPreview(ctx, PreparationRequest{DatasetID: dataset.ID, Model: "preparer", Goal: GoalQuestionAnswer, PreviewRecords: 1})
			if err != nil {
				t.Fatal(err)
			}
			metadata := filepath.Join(root, "imports", dataset.ID, "metadata.json")
			original, err := os.ReadFile(metadata)
			if err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			switch mode {
			case "transient":
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(metadata, 0o700); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(metadata, []byte(`not json`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "escaping-workspace":
				if err := os.Symlink(outside, filepath.Join(root, "preparations")); err != nil {
					t.Fatal(err)
				}
			}
			w, err := b.NewWorker()
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			defer w.Stop()
			if mode == "transient" {
				for {
					description, err := c.DescribeWorkflowExecution(ctx, run.WorkflowID, run.RunID)
					if err != nil {
						t.Fatal(err)
					}
					if len(description.PendingActivities) > 0 && description.PendingActivities[0].Attempt >= 2 {
						break
					}
					if description.WorkflowExecutionInfo.CloseTime != nil {
						t.Fatal("storage error closed the workflow instead of retrying")
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(10 * time.Millisecond):
					}
				}
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(metadata, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := b.WaitPreparation(ctx, run.Execution)
			if mode == "transient" {
				if err != nil || result.Result == nil || result.Result.Preview[0].Target != preparationSource {
					t.Fatalf("storage did not recover: %+v %v", result, err)
				}
				return
			}
			if err == nil {
				t.Fatal("unsafe storage produced a successful preparation")
			}
			if mode == "corrupt" {
				application, ok := errors.AsType[*temporal.ApplicationError](err)
				if !ok || !application.NonRetryable() {
					t.Fatalf("corrupt metadata is retryable: %v", err)
				}
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("preparation escaped its storage root: %v %v", entries, err)
			}
		})
	}
}

func TestPreparationInputValidation(t *testing.T) {
	b, err := New(&mocks.Client{}, Config{DatasetRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, pageSize := range []int{0, 101} {
		if _, _, err := b.PreparationHistory(t.Context(), pageSize, nil); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("accepted invalid preparation history page size %d: %v", pageSize, err)
		}
	}
	for _, change := range []func(*PreparationRequest){
		func(r *PreparationRequest) { r.PreviewRecords = 0 },
		func(r *PreparationRequest) { r.PreviewRecords = 6 },
		func(r *PreparationRequest) { r.Model = " " },
		func(r *PreparationRequest) { r.Goal = "unknown" },
		func(r *PreparationRequest) { r.Instructions = strings.Repeat("x", 2001) },
		func(r *PreparationRequest) { r.ReviewedRecords = []Record{{Prompt: "unexpected seed"}} },
	} {
		input := PreparationRequest{DatasetID: "unused", Model: "preparer", Goal: GoalQuestionAnswer, PreviewRecords: 1}
		change(&input)
		if _, err := b.StartPreview(t.Context(), input); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("accepted invalid preparation input: %+v %v", input, err)
		}
	}
	if _, err := b.StartPreparation(t.Context(), PreparationRequest{Model: "preparer", Goal: GoalQuestionAnswer, ReviewedRecords: []Record{{Prompt: "Question?", Target: "Unsupported answer."}}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("accepted reviewed record without evidence: %v", err)
	}
	var source strings.Builder
	encoder := json.NewEncoder(&source)
	for i := range 50 {
		if err := encoder.Encode(Record{Prompt: fmt.Sprintf("Question %d?", i), Target: "yes", Evidence: fmt.Sprintf("Source %d says yes.", i)}); err != nil {
			t.Fatal(err)
		}
	}
	dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "prepared.jsonl"}, strings.NewReader(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.StartPreview(t.Context(), PreparationRequest{DatasetID: dataset.ID, Model: "model", Goal: GoalQuestionAnswer, PreviewRecords: 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("reprepared JSONL: %v", err)
	}
}
