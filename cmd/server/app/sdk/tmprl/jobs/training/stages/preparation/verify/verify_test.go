package verify_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

type completer struct {
	content string
	err     error
	request *providers.CompletionRequest
	respond func(context.Context, providers.CompletionRequest) (providers.Completion, error)
}

func (c completer) Complete(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
	if c.request != nil {
		*c.request = request
	}
	if c.respond != nil {
		return c.respond(ctx, request)
	}
	return providers.Completion{Content: c.content, PromptTokens: 9}, c.err
}

func TestRun(t *testing.T) {
	records := []training.Record{{Prompt: "Where?", Target: "here", Evidence: "It is here."}, {Prompt: "When?", Target: "now", Evidence: "It is now."}, {Prompt: "Who?", Target: "Ada", Evidence: "Ada works."}, {Prompt: "What?", Target: "boat", Evidence: "A boat."}}
	for _, tc := range []struct {
		name, response string
		warnings       []string
		invalid        bool
	}{
		{name: "all pass", response: `{"record_1":"pass","record_2":"pass","record_3":"pass","record_4":"pass"}`},
		{name: "rejections", response: `{"record_4":"user_requirements","record_2":"grounding","record_1":"pass","record_3":"task_quality"}`, warnings: []string{"record 2: grounding", "record 3: task_quality", "record 4: user_requirements"}},
		{name: "missing", response: `{"record_1":"pass"}`, invalid: true},
		{name: "wrong keys", response: `{"record_1":"pass","record_2":"pass","record_3":"pass","other":"pass"}`, invalid: true},
		{name: "unknown outcome", response: `{"record_1":"yes","record_2":"pass","record_3":"pass","record_4":"pass"}`, invalid: true},
		{name: "malformed", response: `{`, invalid: true},
		{name: "trailing", response: `{} {}`, invalid: true},
		{name: "extra", response: `{"record_1":"pass","record_2":"pass","record_3":"pass","record_4":"pass","record_5":"pass"}`, invalid: true},
		{name: "number", response: `{"record_1":1,"record_2":"pass","record_3":"pass","record_4":"pass"}`, invalid: true},
		{name: "null outcome", response: `{"record_1":null,"record_2":"pass","record_3":"pass","record_4":"pass"}`, invalid: true},
		{name: "empty outcome", response: `{"record_1":"","record_2":"pass","record_3":"pass","record_4":"pass"}`, invalid: true},
		{name: "empty", invalid: true},
		{name: "null", response: `null`, invalid: true},
		{name: "array", response: `[]`, invalid: true},
		{name: "fenced", response: "```json\n{}\n```", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warnings, usage, err := verify.Run(t.Context(), completer{content: tc.response}, preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer}, "Source topic", records)
			if (err != nil) != tc.invalid {
				t.Fatalf("error = %v", err)
			}
			if !tc.invalid && !reflect.DeepEqual(warnings, tc.warnings) {
				t.Fatalf("warnings = %v, want %v", warnings, tc.warnings)
			}
			if usage.PromptTokens != 9 {
				t.Fatalf("lost usage: %+v", usage)
			}
		})
	}
	want := errors.New("completion failed")
	if _, _, err := verify.Run(t.Context(), completer{err: want}, preparation.PreparationRequest{}, "Source topic", records); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRequest(t *testing.T) {
	var request providers.CompletionRequest
	records := []training.Record{{Prompt: "When does the ferry leave?", Target: "Noon", Evidence: "The ferry leaves at noon."}}
	_, _, err := verify.Run(t.Context(), completer{content: `{"record_1":"pass"}`, request: &request}, preparation.PreparationRequest{Model: "catalog/qwen", Goal: preparation.GoalQuestionAnswer, Instructions: "Ask about harbor operations."}, "Harbor operations. The ferry was repaired after a storm.", records)
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "catalog/qwen" || !request.EnableThinking || request.ResponseSchema != nil || request.MaxTokens != 4096 || request.Seed != 3407 {
		t.Fatalf("incorrect verification request: %+v", request)
	}
	if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
		t.Fatalf("incorrect messages: %+v", request.Messages)
	}
	for _, text := range []string{records[0].Prompt, records[0].Target, records[0].Evidence, "question_answer", "Ask about harbor operations."} {
		if !strings.Contains(request.Messages[1].Content, text) {
			t.Fatalf("request omitted %q", text)
		}
	}
	contextBlock, recordBlock, ok := strings.Cut(request.Messages[1].Content, "[END DOCUMENT CONTEXT]\n")
	if !ok || contextBlock != "[DOCUMENT CONTEXT: TOPIC ONLY]\nHarbor operations. The ferry was repaired after a storm.\n" || strings.Contains(recordBlock, "storm") || !strings.Contains(recordBlock, "EVIDENCE: The ferry leaves at noon.") {
		t.Fatalf("document context was not kept separate from evidence: %s", request.Messages[1].Content)
	}
	if !strings.Contains(request.Messages[0].Content, "exactly these keys: record_1.") || !strings.Contains(request.Messages[0].Content, "only the record's own EVIDENCE may establish those") {
		t.Fatalf("missing output or evidence contract: %s", request.Messages[0].Content)
	}
}

func TestStepRecovery(t *testing.T) {
	for _, failure := range []string{"provider", "context budget", "truncated", "malformed", "rejected", "canceled", "pause"} {
		t.Run(failure, func(t *testing.T) {
			store, err := uploads.New(uploads.Config{Root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			var source strings.Builder
			for i := range 20 {
				fmt.Fprintf(&source, "Device %02d has calibration code C%02d.\n\n", i, i)
			}
			file, err := store.Put(t.Context(), uploads.Input{Name: "devices.md"}, strings.NewReader(source.String()))
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { workspace.Close() })
			open := func(ctx context.Context) (io.ReadCloser, error) { return store.Open(ctx, file.ID) }
			if _, err := extract.Run(t.Context(), nil, file, open, workspace); err != nil {
				t.Fatal(err)
			}
			var records []training.Record
			for i := range 50 {
				index := i % 20
				r := training.Record{
					Prompt: fmt.Sprintf("What is the calibration code for device %02d in question %02d?", index, i),
					Target: fmt.Sprintf("C%02d", index), Evidence: fmt.Sprintf("Device %02d has calibration code C%02d.", index, index),
					SourceDocument: file.ID, UserApproved: i < 3, ValidationPrompt: i >= 20 && i < 30, IndependentPrompt: i >= 30,
				}
				r.FactID = training.FactID(r.Evidence, r.Target)
				records = append(records, r)
			}
			input := preparation.StepContext{
				Workspace: workspace,
				Review:    &preparation.Review{Records: slices.Clone(records[:3])}, ReviewID: "accepted-review",
				Preparation: &preparation.PreparationContext{
					Input: preparation.PreparationInput{Kind: preparation.PreparationFull, Request: preparation.PreparationRequest{
						DatasetID: file.ID, Model: "catalog/qwen", Goal: preparation.GoalQuestionAnswer, Instructions: "Ask about calibration codes.",
					}},
					Source: file, OpenSource: func(context.Context) (io.ReadCloser, error) {
						t.Fatal("Verify must not reopen or convert the source")
						return nil, os.ErrNotExist
					},
				},
			}
			state, _, err := checkpoint.Load(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			state.Generated, state.Records = true, slices.Clone(records)
			state.ReviewID, state.ReviewedCount = input.ReviewID, 3
			state.Result.PromptTokens, state.Result.CompletionTokens = 100, 200
			if err := tmprl.WriteJSON(workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			var savedBeforeFailure []byte
			offset, batches := 0, 0
			failureOffset := 3
			if failure == "rejected" {
				failureOffset = 0 // Rejected approved examples must still stop publication.
			}
			failed := false
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			wantErr := errors.New("controlled completion failure")
			switch failure {
			case "context budget":
				wantErr = errors.New("insufficient context budget")
			case "truncated":
				wantErr = providers.ErrCompletionTruncated
			case "canceled":
				wantErr = context.Canceled
			}
			if failure == "pause" {
				input.SafePoint = func(_ context.Context, path string) error {
					if offset != failureOffset || failed {
						return nil
					}
					failed = true
					var err error
					savedBeforeFailure, err = input.Workspace.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					return wantErr
				}
			}
			input.Preparation.Completer = completer{respond: func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				end := min(offset+5, 20)
				switch {
				case offset < 3:
					end = 3
				case offset >= 20:
					end = offset + 1
				}
				contextBlock, recordBlock, ok := strings.Cut(request.Messages[1].Content, "[END DOCUMENT CONTEXT]\n")
				if !ok || contextBlock != "[DOCUMENT CONTEXT: TOPIC ONLY]\n\n" || strings.Count(recordBlock, "[RECORD ") != end-offset {
					t.Fatalf("incorrect saved context or recovery batch at %d: %+v", offset, request)
				}
				outcomes := make(map[string]string)
				for i, r := range records[offset:end] {
					block := fmt.Sprintf("[RECORD %d]\nPROMPT: %s\nTARGET: %s\nEVIDENCE: %s\n", i+1, r.Prompt, r.Target, r.Evidence)
					if !strings.Contains(recordBlock, block) {
						t.Fatalf("changed or repeated record at %d: %s", offset+i, recordBlock)
					}
					outcomes[fmt.Sprintf("record_%d", i+1)] = "pass"
				}
				if offset == failureOffset && !failed {
					failed = true
					var err error
					savedBeforeFailure, err = input.Workspace.ReadFile(checkpoint.File)
					if err != nil {
						t.Fatal(err)
					}
					switch failure {
					case "malformed":
						return providers.Completion{Content: "not JSON", PromptTokens: 999}, nil
					case "rejected":
						return providers.Completion{Content: `{"record_1":"pass","record_2":"grounding","record_3":"pass"}`, PromptTokens: 999}, nil
					case "canceled":
						cancel()
						return providers.Completion{PromptTokens: 999}, ctx.Err()
					default:
						return providers.Completion{PromptTokens: 999}, wantErr
					}
				}
				offset = end
				batches++
				data, err := json.Marshal(outcomes)
				return providers.Completion{Content: string(data), PromptTokens: 9, CompletionTokens: 17}, err
			}}
			got, err := (verify.Step{}).Execute(ctx, input)
			if err == nil || got.Artifacts.Preparation != nil || !failed || offset != failureOffset {
				t.Fatalf("failed verification published or lost progress: %+v, %v, offset=%d", got, err, offset)
			}
			if failure != "malformed" && failure != "rejected" && !errors.Is(err, wantErr) {
				t.Fatalf("lost completion error: %v", err)
			}
			after, err := workspace.ReadFile(checkpoint.File)
			if err != nil || string(after) != string(savedBeforeFailure) {
				t.Fatalf("failed batch changed checkpoint: %v", err)
			}
			if err := json.Unmarshal(after, &state); err != nil {
				t.Fatal(err)
			}
			if state.Verified != failureOffset || state.Result.PromptTokens != 100+batches*9 || state.Result.CompletionTokens != 200+batches*17 || !slices.Equal(state.Records, records) || state.ReviewID != input.ReviewID || state.ReviewedCount != 3 {
				t.Fatalf("failed batch changed records, accepted review, usage or cursor: %+v", state)
			}
			for _, name := range []string{"dataset.jsonl", "dataset-report.json", "result.json"} {
				if _, err := workspace.ReadFile(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed verification published %s: %v", name, err)
				}
			}
			// Saved context must be available and match; Verify never repairs it.
			if failure == "provider" {
				extraction, err := workspace.ReadFile("extraction.json")
				if err != nil {
					t.Fatal(err)
				}
				for _, bad := range []string{"missing", "stale"} {
					switch bad {
					case "missing":
						err = workspace.Remove("extraction.json")
					case "stale":
						err = workspace.WriteFile("extraction.json", []byte(strings.Replace(string(extraction), file.SHA256, "stale-source", 1)), 0o600)
					}
					if err != nil {
						t.Fatal(err)
					}
					if got, err := (verify.Step{}).Execute(t.Context(), input); err == nil || got.Artifacts.Preparation != nil || offset != 3 {
						t.Fatalf("%s extraction was accepted: %+v, %v", bad, got, err)
					}
					data, err := workspace.ReadFile(checkpoint.File)
					if err != nil || string(data) != string(after) {
						t.Fatalf("%s extraction changed progress: %v", bad, err)
					}
				}
				if err := workspace.WriteFile("extraction.json", extraction, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root := workspace.Name()
			if err := workspace.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { reopened.Close() })
			input.Workspace = reopened
			got, err = (verify.Step{}).Execute(t.Context(), input)
			if err != nil || got.Artifacts.Preparation == nil || !got.Artifacts.Preparation.TrainingReady || got.Artifacts.Preparation.RecordCount != 50 || offset != 50 || batches != 35 {
				t.Fatalf("checkpoint recovery: %+v, %v, offset=%d batches=%d", got, err, offset, batches)
			}
			result := got.Artifacts.Preparation
			if result.PromptTokens != 415 || result.CompletionTokens != 795 {
				t.Fatalf("incorrect durable usage: %+v", result)
			}
			published, err := reopened.ReadFile("dataset.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			gotRecords, err := training.ReadRecords(t.Context(), strings.NewReader(string(published)))
			if err != nil || !slices.Equal(gotRecords, records) {
				t.Fatalf("published records changed: %+v, %v", gotRecords, err)
			}
			// Completed replay, and publication-only recovery, need no source context.
			if err := reopened.Remove("extraction.json"); err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completer{respond: func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("completed verification must not call the model again")
				return providers.Completion{}, wantErr
			}}
			for _, publicationOnly := range []bool{false, true} {
				if publicationOnly {
					if err := reopened.Remove("result.json"); err != nil {
						t.Fatal(err)
					}
				}
				replayed, err := (verify.Step{}).Execute(t.Context(), input)
				if err != nil || !reflect.DeepEqual(replayed, got) {
					t.Fatalf("completed replay (publication only=%t): %+v, %v", publicationOnly, replayed, err)
				}
			}
		})
	}
}
