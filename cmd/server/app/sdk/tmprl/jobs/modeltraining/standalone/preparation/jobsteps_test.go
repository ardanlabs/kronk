package prepare_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	prepare "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/standalone/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/mocks"
)

type jobStepCompleter func(context.Context, CompletionRequest) (Completion, error)

func (f jobStepCompleter) Complete(ctx context.Context, request CompletionRequest) (Completion, error) {
	return f(ctx, request)
}

func executePreparationStep(t *testing.T, b *prepare.Business, name string, input PreparationInput, execution tmprl.Execution) (PreparationResult, error) {
	t.Helper()
	steps := map[string]StepHandler{"extract": extract.Step{}, "generate": generate.Step{}, "verify": verify.Step{}}
	incoming := Artifacts{Preparation: &PreparationResult{DatasetID: input.Request.DatasetID}}
	output, err := b.ExecuteJobStep(t.Context(), steps[name], input, execution, incoming, nil)
	if err != nil {
		return PreparationResult{}, err
	}
	if name != "verify" && output.Artifacts != incoming {
		t.Fatalf("got artifacts %+v, want preserved %+v", output.Artifacts, incoming)
	}
	if output.Status != "" {
		t.Fatalf("got terminal status %q, want continuation", output.Status)
	}
	if output.Artifacts.Preparation == nil {
		return PreparationResult{}, nil
	}
	return *output.Artifacts.Preparation, nil
}

// All calls use the public Business boundary with an ordinary context: no
// Temporal server, native model, trainer, or activity context is involved.
func TestJobPreparationStepsRecovery(t *testing.T) {
	root := t.TempDir()
	datasets, err := imports.New(imports.Config{DatasetRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for i := range 20 {
		fmt.Fprintf(&text, "Device %02d has calibration code C%02d.\n\n", i, i)
	}
	imported, err := datasets.ImportDataset(t.Context(), imports.ImportInput{Name: "devices.md"}, strings.NewReader(text.String()))
	if err != nil {
		t.Fatal(err)
	}
	generated, evaluated, verified := 0, 0, 0
	genOutage, evalOutage, verifyOutage := false, false, false
	outage := errors.New("provider unavailable")
	completer := jobStepCompleter(func(ctx context.Context, r CompletionRequest) (Completion, error) {
		prompt := r.Messages[1].Content
		var output any
		switch {
		case strings.Contains(prompt, "[RECORD 1]"):
			if verified == 1 && !verifyOutage {
				verifyOutage = true
				return Completion{}, outage
			}
			verified++
			outcomes := make(map[string]string)
			for i := range strings.Count(prompt, "[RECORD ") {
				outcomes[fmt.Sprintf("record_%d", i+1)] = "pass"
			}
			output = outcomes
		case strings.Contains(prompt, "Locked target:"):
			if evaluated == 5 && !evalOutage {
				evalOutage = true
				return Completion{}, outage
			}
			_, target, _ := strings.Cut(prompt, "Locked target: ")
			var index int
			if _, err := fmt.Sscanf(target, "Device %d", &index); err != nil {
				return Completion{}, err
			}
			question := fmt.Sprintf("Which calibration code belongs to device %02d?", index)
			if strings.Contains(prompt, "Do not repeat: "+question) {
				question = fmt.Sprintf("For device %02d, what is its calibration code?", index)
			}
			output = map[string]string{"record_1": question}
			evaluated++
		default:
			if generated == 1 && !genOutage {
				genOutage = true
				return Completion{}, outage
			}
			_, source, _ := strings.Cut(prompt, "SOURCE:\n")
			var rows []map[string]any
			for i, section := range strings.Split(source, "[PASSAGE ")[1:] {
				_, passage, _ := strings.Cut(section, "]\n")
				var index int
				if _, err := fmt.Sscanf(passage, "Device %d", &index); err != nil {
					return Completion{}, err
				}
				rows = append(rows, map[string]any{"prompt": fmt.Sprintf("What calibration code applies to device %02d?", index), "evidence_id": i + 1, "target_id": 1})
			}
			output = map[string]any{"records": rows}
			generated++
		}
		data, err := json.Marshal(output)
		return Completion{Content: string(data), PromptTokens: 1, CompletionTokens: 1}, err
	})
	b, err := prepare.New(&mocks.Client{}, prepare.Config{Datasets: datasets, Completer: completer})
	if err != nil {
		t.Fatal(err)
	}
	e := tmprl.Execution{WorkflowID: "job", RunID: "run"}
	input := PreparationInput{Kind: PreparationFull, Request: PreparationRequest{DatasetID: imported.ID, Goal: GoalQuestionAnswer, Model: "fake"}}
	run := func(step string) (PreparationResult, error) {
		return executePreparationStep(t, b, step, input, e)
	}
	if _, err := run("generate"); err == nil {
		t.Fatal("generate accepted before extract")
	}
	for range 2 {
		if _, err := run("extract"); err != nil {
			t.Fatal(err)
		}
	}
	if generated+evaluated+verified != 0 {
		t.Fatal("extract called completer")
	}
	if _, err := run("verify"); err == nil {
		t.Fatal("verify accepted before generate")
	}
	for range 2 {
		if _, err := run("generate"); !errors.Is(err, outage) {
			t.Fatalf("got %v, want outage", err)
		}
	}
	if _, err := run("generate"); err != nil {
		t.Fatal(err)
	}
	if generated != 4 || evaluated != 30 || verified != 0 {
		t.Fatalf("got calls %d/%d/%d, want 4/30/0", generated, evaluated, verified)
	}
	if _, err := run("generate"); err != nil {
		t.Fatal(err)
	}
	if generated != 4 || evaluated != 30 {
		t.Fatal("generation replay repeated committed calls")
	}
	workspace := filepath.Join(root, "preparations", e.RunID)
	for _, name := range []string{"dataset.jsonl", "result.json"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unverified artifact %s: %v", name, err)
		}
	}
	if _, err := run("verify"); !errors.Is(err, outage) {
		t.Fatalf("got %v, want verification outage", err)
	}
	result, err := run("verify")
	if err != nil {
		t.Fatal(err)
	}
	if !result.TrainingReady || result.RecordCount != 50 || verified != 34 {
		t.Fatalf("got result %+v, calls %d, want ready/50/34", result, verified)
	}
	if _, err := run("verify"); err != nil {
		t.Fatal(err)
	}
	if verified != 34 {
		t.Fatal("verify replay repeated committed calls")
	}
	reader, err := b.OpenRecords(PreparationRun{Execution: e, Status: tmprl.StatusCompleted, Result: &result})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	train, validation, heldout := 0, 0, 0
	for {
		var r training.Record
		if err := decoder.Decode(&r); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if r.SourceDocument != imported.ID || r.FactID == "" {
			t.Fatalf("lost provenance: %+v", r)
		}
		switch {
		case r.IndependentPrompt:
			heldout++
			if r.UserApproved {
				t.Fatal("held-out row user approved")
			}
		case r.ValidationPrompt:
			validation++
		default:
			train++
		}
	}
	if train != 20 || validation != 10 || heldout != 20 {
		t.Fatalf("got splits %d/%d/%d, want 20/10/20", train, validation, heldout)
	}
	changed := input
	changed.Request.Instructions = "different configuration"
	if _, err := executePreparationStep(t, b, "verify", changed, e); err == nil {
		t.Fatal("accepted mismatched replay")
	}
}

func TestJobPreparationVerificationRejects(t *testing.T) {
	root := t.TempDir()
	datasets, err := imports.New(imports.Config{DatasetRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := datasets.ImportDataset(t.Context(), imports.ImportInput{Name: "source.md"}, strings.NewReader("A goroutine is an independently executing function in Go."))
	if err != nil {
		t.Fatal(err)
	}
	b, err := prepare.New(&mocks.Client{}, prepare.Config{Datasets: datasets, Completer: jobStepCompleter(func(ctx context.Context, r CompletionRequest) (Completion, error) {
		if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
			return Completion{Content: `{"record_1":"grounding"}`}, nil
		}
		return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":1}]}`}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	input := PreparationInput{Kind: PreparationFull, Request: PreparationRequest{DatasetID: imported.ID, Model: "fake", Goal: GoalQuestionAnswer}}
	e := tmprl.Execution{WorkflowID: "job", RunID: "run"}
	for _, step := range []string{"extract", "generate"} {
		if _, err := executePreparationStep(t, b, step, input, e); err != nil {
			t.Fatal(err)
		}
	}
	result, err := executePreparationStep(t, b, "verify", input, e)
	if err == nil || result.TrainingReady {
		t.Fatalf("got %+v, %v, want rejected verification", result, err)
	}
	for _, name := range []string{"dataset.jsonl", "dataset-report.json", "result.json"} {
		if _, err := os.Stat(filepath.Join(root, "preparations", e.RunID, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected artifact %s: %v", name, err)
		}
	}
	if _, err := executePreparationStep(t, b, "extract", input, tmprl.Execution{WorkflowID: "job", RunID: "../escape"}); err == nil {
		t.Fatal("accepted path traversal")
	}
}

func TestEngineBuildStepReplayAndLegacyCheckpoint(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			const text = "A goroutine is an independently executing function in Go."
			datasets, err := imports.New(imports.Config{DatasetRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			imported, err := datasets.ImportDataset(t.Context(), imports.ImportInput{Name: "source.md"}, strings.NewReader(text))
			if err != nil {
				t.Fatal(err)
			}
			source, err := datasets.DatasetSource(t.Context(), imported.ID)
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(datasets.Root())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			workspace, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer workspace.Close()
			input := PreparationInput{Kind: PreparationFull, Request: PreparationRequest{DatasetID: imported.ID, Model: "fake", Goal: GoalQuestionAnswer}}
			if legacy {
				identity, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(append(identity, []byte(text)...))
				// This is the original on-disk v1 shape: its existing record
				// was already verified and must not call either provider again.
				checkpoint := map[string]any{"fingerprint": "tuning-preparation-v1/" + hex.EncodeToString(hash[:]), "next": 1, "evaluation_next": 0, "records": []training.Record{{Prompt: "What is a goroutine?", Target: text, Evidence: text, FactID: training.FactID(text, text), SourceDocument: imported.ID}}}
				if err := tmprl.WriteJSON(workspace, "checkpoint.json", checkpoint); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			engine := prepare.Engine{Completer: jobStepCompleter(func(ctx context.Context, r CompletionRequest) (Completion, error) {
				calls++
				if legacy {
					return Completion{}, errors.New("legacy committed work repeated")
				}
				if strings.Contains(r.Messages[1].Content, "[RECORD 1]") {
					return Completion{Content: `{"record_1":"pass"}`}, nil
				}
				return Completion{Content: `{"records":[{"prompt":"What is a goroutine?","evidence_id":1,"target_id":1}]}`}, nil
			})}
			for range 2 {
				result, err := engine.Build(t.Context(), input, source, root, workspace, nil)
				if err != nil {
					t.Fatal(err)
				}
				if result.RecordCount != 1 || result.TrainingReady {
					t.Fatalf("got %+v, want one non-ready record", result)
				}
			}
			want := 2
			if legacy {
				want = 0
			}
			if calls != want {
				t.Fatalf("got provider calls %d, want %d", calls, want)
			}
		})
	}
}
