package preview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/preview"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

type completer func(context.Context, providers.CompletionRequest) (providers.Completion, error)

func (c completer) Complete(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
	return c(ctx, request)
}

var passages = []string{
	"Amber harbor opens at dawn.", "Birch ferry leaves at noon.",
	"Cedar library closes at dusk.", "Dahlia market opens on Monday.",
	"Elm station closes on Friday.", "Fir museum opens on Tuesday.",
	"Grove clinic closes on Saturday.",
}

func inputFor(t *testing.T, count int, source []string) preparation.StepContext {
	t.Helper()
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Put(t.Context(), uploads.Input{Name: "source.txt"}, strings.NewReader(strings.Join(source, "\n\n")))
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
	open = func(context.Context) (io.ReadCloser, error) {
		return nil, errors.New("preview must not reopen the source")
	}
	return preparation.StepContext{
		Workspace: workspace,
		Preparation: &preparation.PreparationContext{
			Input: preparation.PreparationInput{Request: preparation.PreparationRequest{
				DatasetID: file.ID, Model: "catalog/qwen", Goal: preparation.GoalQuestionAnswer,
				Instructions: "Answer briefly", PreviewRecords: count,
			}, Kind: preparation.PreparationFull},
			Source: file, OpenSource: open, Completer: responses(count, "Where is example"),
		},
	}
}

func responses(count int, prompt string) completer {
	return func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		if request.Model != "catalog/qwen" || len(request.Messages) < 2 || !strings.Contains(request.Messages[1].Content, "Answer briefly") {
			return providers.Completion{}, errors.New("preparation model or instructions were not forwarded")
		}
		if request.ResponseSchema != nil {
			var records []map[string]any
			for i := range count {
				records = append(records, map[string]any{"prompt": fmt.Sprintf("%s %d?", prompt, i), "evidence_id": count - i, "target_id": 1})
			}
			data, err := json.Marshal(map[string]any{"records": records})
			return providers.Completion{Content: string(data), PromptTokens: 11, CompletionTokens: 3}, err
		}
		if !request.EnableThinking || request.MaxTokens != 4096 {
			return providers.Completion{}, errors.New("preview verification did not opt into reasoned completion")
		}
		outcomes := make(map[string]string)
		for i := range count {
			outcomes[fmt.Sprintf("record_%d", i+1)] = "pass"
		}
		outcomes["record_1"] = "grounding"
		data, err := json.Marshal(outcomes)
		return providers.Completion{Content: string(data), PromptTokens: 7, CompletionTokens: 2}, err
	}
}

func TestPassages(t *testing.T) {
	for _, tc := range []struct {
		name        string
		markdown    string
		goal        preparation.Goal
		count       int
		want        []string
		warnings    []string
		wantError   bool
		wantInvalid bool
	}{
		{
			name: "even sample", markdown: strings.Join(passages, "\n\n"),
			goal: preparation.GoalQuestionAnswer, count: 3,
			want: []string{"Amber harbor opens at dawn.", "Cedar library closes at dusk.", "Elm station closes on Friday."},
		},
		{
			name: "bounded by source", markdown: "The ferry leaves at noon.\n\nThe market opens at dawn.",
			goal: preparation.GoalTextGeneration, count: 5,
			want: []string{"The ferry leaves at noon.", "The market opens at dawn."},
		},
		{
			name: "filter before sampling", markdown: strings.Join([]string{
				"The harbor opens at dawn.", "The ferry carries 12 passengers.",
				strings.Repeat("x", 401), "The market opens on Monday.",
				"The museum has 24 exhibits.", "The library has 36 shelves.",
			}, "\n\n"),
			goal: preparation.GoalNumericPrediction, count: 2,
			want:     []string{"The ferry carries 12 passengers.", "The museum has 24 exhibits."},
			warnings: []string{"3 source passages excluded"},
		},
		{name: "zero count", markdown: passages[0], goal: preparation.GoalQuestionAnswer, count: 0, wantError: true, wantInvalid: true},
		{name: "excess count", markdown: passages[0], goal: preparation.GoalQuestionAnswer, count: 6, wantError: true, wantInvalid: true},
		{name: "no numeric evidence", markdown: passages[0], goal: preparation.GoalNumericPrediction, count: 1, wantError: true},
		{name: "empty source", goal: preparation.GoalQuestionAnswer, count: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings, err := preview.Passages(tc.markdown, tc.goal, tc.count)
			if (err != nil) != tc.wantError || errors.Is(err, preparation.ErrInvalidArgument) != tc.wantInvalid {
				t.Fatalf("Passages error = %v; want error %t, invalid argument %t", err, tc.wantError, tc.wantInvalid)
			}
			if !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(warnings, tc.warnings) {
				t.Fatalf("Passages = %q, warnings %q; want %q, warnings %q", got, warnings, tc.want, tc.warnings)
			}
		})
	}
}

func TestPreview(t *testing.T) {
	for _, tc := range []struct {
		count  int
		source []string
		want   []int
	}{
		{count: 1, source: passages, want: []int{0}},
		{count: 3, source: passages, want: []int{4, 2, 0}},
		{count: 5, source: passages, want: []int{5, 4, 2, 1, 0}},
		{count: 5, source: passages[:2], want: []int{1, 0}},
	} {
		t.Run(fmt.Sprintf("requested-%d-available-%d", tc.count, len(tc.source)), func(t *testing.T) {
			input := inputFor(t, tc.count, tc.source)
			input.Preparation.Completer = completer(func(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				if request.ResponseSchema == nil && !strings.HasPrefix(request.Messages[1].Content, "[DOCUMENT CONTEXT: TOPIC ONLY]\n"+strings.Join(tc.source, "\n\n")+"\n[END DOCUMENT CONTEXT]\n") {
					t.Fatal("preview verification omitted or changed saved document context")
				}
				return responses(len(tc.want), "Where is example")(ctx, request)
			})
			full := preparation.PreparationResult{DatasetID: "finished-dataset", RecordCount: 50, TrainingReady: true}
			input.Artifacts.Preparation = &full
			for _, name := range []string{"steps.json", "result.json", "dataset.jsonl"} {
				if err := input.Workspace.WriteFile(name, []byte("existing full preparation"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var progress []tmprl.Progress
			input.Publish = func(p tmprl.Progress) { progress = append(progress, p) }
			got, err := (preview.Step{}).Execute(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			result := got.Artifacts.Preview
			if result == nil || result.DatasetID != input.Preparation.Source.ID || result.RecordCount != len(tc.want) || result.TrainingReady || got.Status != "" {
				t.Fatalf("incorrect preview result: %+v", got)
			}
			if result.PromptTokens != 18 || result.CompletionTokens != 5 || !reflect.DeepEqual(result.Warnings, []string{"record 1: grounding"}) {
				t.Fatalf("incorrect usage or review findings: %+v", result)
			}
			for i, index := range tc.want {
				r := result.Preview[i]
				if r.Target != tc.source[index] || r.Evidence != tc.source[index] || r.SourceDocument != input.Preparation.Source.ID || r.FactID == "" || r.UserApproved || r.ValidationPrompt || r.IndependentPrompt {
					t.Fatalf("record %d = %+v; want source passage %q", i, r, tc.source[index])
				}
			}
			if !reflect.DeepEqual(got.Artifacts.Preparation, &full) || input.Preparation.Input.Kind != preparation.PreparationFull {
				t.Fatal("preview modified full-preparation artifacts or input")
			}
			for _, name := range []string{"steps.json", "result.json", "dataset.jsonl"} {
				data, err := input.Workspace.ReadFile(name)
				if err != nil || string(data) != "existing full preparation" {
					t.Fatalf("full preparation %s changed: %q, %v", name, data, err)
				}
			}
			if len(progress) == 0 || progress[len(progress)-1].Completed != len(tc.want) || progress[len(progress)-1].Total != len(tc.want) {
				t.Fatalf("missing completion progress: %+v", progress)
			}
			input.Preparation.Completer = completer(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				return providers.Completion{}, errors.New("model unavailable after preview")
			})
			replayed, err := (preview.Step{}).Execute(t.Context(), input)
			if err != nil || !reflect.DeepEqual(replayed, got) {
				t.Fatalf("saved preview = %+v, %v; want %+v", replayed, err, got)
			}
		})
	}
}

func TestPreviewRetry(t *testing.T) {
	for _, name := range []string{"verification failure", "verification context limit", "verification truncated", "verification malformed", "verification canceled", "generation canceled", "generation malformed"} {
		t.Run(name, func(t *testing.T) {
			input := inputFor(t, 3, passages)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("verification unavailable")
			if name == "verification context limit" {
				failure = errors.New("insufficient context budget")
			}
			if name == "verification truncated" {
				failure = providers.ErrCompletionTruncated
			}
			var generated []byte
			input.Preparation.Completer = completer(func(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				generating := request.ResponseSchema != nil
				if !generating {
					var err error
					generated, err = input.Workspace.ReadFile("preview/steps.json")
					if err != nil {
						t.Fatal(err)
					}
				}
				switch {
				case generating && name == "generation malformed":
					return providers.Completion{Content: `{"records":[]}`}, nil
				case !generating && (name == "verification failure" || name == "verification context limit" || name == "verification truncated"):
					return providers.Completion{}, failure
				case !generating && name == "verification malformed":
					return providers.Completion{Content: "```json\n{}\n```"}, nil
				case generating && name == "generation canceled", !generating && name == "verification canceled":
					cancel()
				}
				return responses(3, "Original example")(ctx, request)
			})
			got, err := (preview.Step{}).Execute(ctx, input)
			want := failure
			switch name {
			case "verification canceled", "generation canceled":
				want = context.Canceled
			case "generation malformed", "verification malformed":
				want = nil
			}
			if err == nil || want != nil && !errors.Is(err, want) || got.Artifacts.Preview != nil {
				t.Fatalf("failed preview = %+v, %v; want %v and no published preview", got, err, want)
			}
			if generated != nil {
				after, err := input.Workspace.ReadFile("preview/steps.json")
				if err != nil || string(after) != string(generated) {
					t.Fatalf("failed verification changed saved generation: %v", err)
				}
			}
			reopened, err := os.OpenRoot(input.Workspace.Name())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { reopened.Close() })
			input.Workspace = reopened
			input.Preparation.Completer = responses(3, "Regenerated example")
			retried, err := (preview.Step{}).Execute(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			result := retried.Artifacts.Preview
			if result.PromptTokens != 18 || result.CompletionTokens != 5 || result.TrainingReady {
				t.Fatalf("incorrect retried result: %+v", result)
			}
			prompt := "Original example"
			if strings.HasPrefix(name, "generation") {
				prompt = "Regenerated example"
			}
			for i, r := range result.Preview {
				if r.Prompt != fmt.Sprintf("%s %d?", prompt, i) {
					t.Fatalf("retry did not preserve committed work: %+v", r)
				}
			}
		})
	}
}

func TestPreviewRejectsInput(t *testing.T) {
	for _, name := range []string{"zero count", "excess count", "accepted examples", "missing completer", "missing resources", "missing workspace", "missing extraction", "stale source", "no numeric evidence", "canceled"} {
		t.Run(name, func(t *testing.T) {
			input := inputFor(t, 3, passages)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var want error
			switch name {
			case "zero count":
				input.Preparation.Input.Request.PreviewRecords = 0
				want = preparation.ErrInvalidArgument
			case "excess count":
				input.Preparation.Input.Request.PreviewRecords = 6
				want = preparation.ErrInvalidArgument
			case "accepted examples":
				input.Preparation.Input.Request.ReviewedRecords = []training.Record{{Prompt: "Accepted?"}}
				want = preparation.ErrInvalidArgument
			case "missing completer":
				input.Preparation.Completer = nil
				want = preparation.ErrInvalidArgument
			case "missing resources":
				input.Preparation = nil
				want = preparation.ErrInvalidArgument
			case "missing workspace":
				input.Workspace = nil
				want = preparation.ErrInvalidArgument
			case "missing extraction":
				if err := input.Workspace.Remove("extraction.json"); err != nil {
					t.Fatal(err)
				}
				want = os.ErrNotExist
			case "stale source":
				input.Preparation.Source.SHA256 = "another source"
				want = preparation.ErrInvalidArgument
			case "no numeric evidence":
				input.Preparation.Input.Request.Goal = preparation.GoalNumericPrediction
			case "canceled":
				cancel()
				want = context.Canceled
			}
			got, err := (preview.Step{}).Execute(ctx, input)
			if err == nil || got.Artifacts.Preview != nil || want != nil && !errors.Is(err, want) {
				t.Fatalf("preview = %+v, %v; want rejection (%v)", got, err, want)
			}
		})
	}
}

func TestPreviewRejectsChangedConfiguration(t *testing.T) {
	for _, name := range []string{"model", "instructions", "source"} {
		t.Run(name, func(t *testing.T) {
			input := inputFor(t, 3, passages)
			if _, err := (preview.Step{}).Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "model":
				input.Preparation.Input.Request.Model = "another-model"
			case "instructions":
				input.Preparation.Input.Request.Instructions = "Different instructions"
			case "source":
				other := inputFor(t, 3, []string{"A different source document."})
				input.Preparation.Source = other.Preparation.Source
				input.Preparation.OpenSource = other.Preparation.OpenSource
				input.Preparation.Input.Request.DatasetID = other.Preparation.Source.ID
				data, err := other.Workspace.ReadFile("extraction.json")
				if err != nil {
					t.Fatal(err)
				}
				if err := input.Workspace.WriteFile("extraction.json", data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := (preview.Step{}).Execute(t.Context(), input)
			if err == nil || !strings.Contains(err.Error(), "checkpoint does not match") || got.Artifacts.Preview != nil {
				t.Fatalf("reused preview with changed %s: %+v, %v", name, got, err)
			}
		})
	}
}

func TestValidateReview(t *testing.T) {
	for _, edit := range []string{"prompt only", "target only", "both"} {
		t.Run(edit, func(t *testing.T) {
			saved := preparation.PreparationResult{DatasetID: "harbor", Preview: []training.Record{
				{Prompt: "When does the harbor open?", Target: "At dawn.", Evidence: "  The harbor\n opens at dawn.  ", SourceDocument: "harbor", FactID: "old-harbor"},
				{Prompt: "When does the ferry leave?", Target: "At noon.", Evidence: "The ferry leaves at noon.", SourceDocument: "harbor", FactID: "old-ferry"},
			}}
			records := append([]training.Record(nil), saved.Preview...)
			if edit != "target only" {
				records[0].Prompt = " \nWhat  time does the harbor open?\t"
			}
			if edit != "prompt only" {
				// Human approval permits paraphrases that are not quoted evidence.
				records[1].Target = " \nAt\t midday.  "
			}
			before := append([]training.Record(nil), records...)
			savedBefore := append([]training.Record(nil), saved.Preview...)
			payload, err := json.Marshal(preparation.Review{Records: records})
			if err != nil {
				t.Fatal(err)
			}
			payloadBefore := string(payload)
			got, err := preview.ValidateReview(preparation.PreparationRequest{DatasetID: "harbor", Goal: preparation.GoalQuestionAnswer}, &saved, payload)
			if err != nil {
				t.Fatal(err)
			}
			if string(payload) != payloadBefore || !reflect.DeepEqual(records, before) || !reflect.DeepEqual(saved.Preview, savedBefore) {
				t.Fatal("validation modified the submitted or saved records")
			}
			for i, r := range got.Records {
				want := before[i]
				want.Prompt = strings.TrimSpace(want.Prompt)
				want.Target = training.Normalized(want.Target)
				want.Evidence = training.Normalized(want.Evidence)
				want.FactID = training.FactID(want.Evidence, want.Target)
				want.UserApproved = true
				if r != want {
					t.Fatalf("record %d = %+v; want %+v", i, r, want)
				}
			}
			got.Records[0].Prompt = "changed returned copy"
			if string(payload) != payloadBefore || !reflect.DeepEqual(records, before) || !reflect.DeepEqual(saved.Preview, savedBefore) {
				t.Fatal("returned records alias input")
			}
		})
	}
}

func TestValidateReviewRejectsInvalidSubmissions(t *testing.T) {
	for _, name := range []string{
		"missing preview", "dataset mismatch", "saved source mismatch",
		"empty preview", "large preview", "training ready", "missing records", "extra records",
		"unknown goal", "evidence edit", "evidence whitespace", "evidence swap", "source edit", "reordered records",
		"empty prompt", "short prompt", "long prompt", "empty target", "empty evidence", "question without mark",
		"numeric target", "classification target", "duplicate prompts", "validation flag", "independent flag", "both flags",
	} {
		t.Run(name, func(t *testing.T) {
			saved := preparation.PreparationResult{DatasetID: "harbor", Preview: []training.Record{
				{Prompt: "When does the harbor open?", Target: "Dawn", Evidence: "The harbor opens at dawn.", SourceDocument: "harbor"},
				{Prompt: "When does the ferry leave?", Target: "Noon", Evidence: "The ferry leaves at noon.", SourceDocument: "harbor"},
			}}
			savedInput := &saved
			records := append([]training.Record(nil), saved.Preview...)
			goal := preparation.GoalQuestionAnswer
			switch name {
			case "missing preview":
				savedInput = nil
			case "dataset mismatch":
				saved.DatasetID = "another-dataset"
			case "saved source mismatch":
				saved.Preview[1].SourceDocument = "another-source"
			case "empty preview":
				saved.Preview = nil
			case "large preview":
				saved.Preview = append(saved.Preview, records...)
				saved.Preview = append(saved.Preview, records...)
				records = append([]training.Record(nil), saved.Preview...)
			case "training ready":
				saved.TrainingReady = true
			case "missing records":
				records = records[:1]
			case "extra records":
				records = append(records, records[0])
			case "unknown goal":
				goal = "unknown"
			case "evidence edit":
				records[0].Evidence = "Invented evidence."
			case "evidence whitespace":
				records[0].Evidence += " "
			case "evidence swap":
				records[0].Evidence, records[1].Evidence = records[1].Evidence, records[0].Evidence
			case "source edit":
				records[0].SourceDocument = "another-source"
			case "reordered records":
				records[0], records[1] = records[1], records[0]
			case "empty prompt":
				records[1].Prompt = " \n "
			case "short prompt":
				records[1].Prompt = "?"
			case "long prompt":
				records[1].Prompt = strings.Repeat("a", 2000) + "?"
			case "empty target":
				records[1].Target = " \t "
			case "empty evidence":
				saved.Preview[1].Evidence = " \t "
				records[1].Evidence = saved.Preview[1].Evidence
			case "question without mark":
				records[1].Prompt = "Describe the ferry."
			case "numeric target":
				goal = preparation.GoalNumericPrediction
			case "classification target":
				goal = preparation.GoalClassification
				records[1].Target = "one two three four five six seven eight nine"
			case "duplicate prompts":
				records[1].Prompt = " WHEN does\n the harbor open? "
			case "validation flag":
				records[1].ValidationPrompt = true
			case "independent flag":
				records[1].IndependentPrompt = true
			case "both flags":
				records[1].ValidationPrompt = true
				records[1].IndependentPrompt = true
			}
			before := append([]training.Record(nil), records...)
			savedBefore := append([]training.Record(nil), saved.Preview...)
			payload, err := json.Marshal(preparation.Review{Records: records})
			if err != nil {
				t.Fatal(err)
			}
			payloadBefore := string(payload)
			got, err := preview.ValidateReview(preparation.PreparationRequest{DatasetID: "harbor", Goal: goal}, savedInput, payload)
			if !errors.Is(err, preparation.ErrInvalidArgument) || got.Records != nil {
				t.Fatalf("ValidateReview = %+v, %v; want invalid argument and no records", got, err)
			}
			if string(payload) != payloadBefore || !reflect.DeepEqual(records, before) || !reflect.DeepEqual(saved.Preview, savedBefore) {
				t.Fatal("rejected submission modified input")
			}
		})
	}
}

func TestValidateReviewPayload(t *testing.T) {
	saved := preparation.PreparationResult{DatasetID: "harbor", Preview: []training.Record{
		{Prompt: "When does the harbor open?", Target: "Dawn", Evidence: "The harbor opens at dawn.", SourceDocument: "harbor"},
	}}
	valid := `{"records":[{"prompt":"When does the harbor open?","target":"Dawn","evidence":"The harbor opens at dawn.","source_document":"harbor"}]}`
	for _, test := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "valid", payload: valid},
		{name: "trailing whitespace", payload: valid + " \n\t"},
		{name: "empty", wantErr: "review payload"},
		{name: "whitespace", payload: " \n", wantErr: "review payload"},
		{name: "malformed", payload: "{", wantErr: "review payload"},
		{name: "null", payload: "null", wantErr: "reviewed count"},
		{name: "missing records", payload: "{}", wantErr: "reviewed count"},
		{name: "null records", payload: `{"records":null}`, wantErr: "reviewed count"},
		{name: "array", payload: "[]", wantErr: "review payload"},
		{name: "unknown field", payload: strings.Replace(valid, `{"records":`, `{"unexpected":true,"records":`, 1), wantErr: "unknown field"},
		{name: "unknown record field", payload: strings.Replace(valid, `{"prompt":`, `{"unexpected":true,"prompt":`, 1), wantErr: "unknown field"},
		{name: "second object", payload: valid + " {}", wantErr: "one JSON object"},
		{name: "second null", payload: valid + " null", wantErr: "one JSON object"},
		{name: "trailing garbage", payload: valid + " trailing", wantErr: "one JSON object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := preview.ValidateReview(preparation.PreparationRequest{DatasetID: "harbor", Goal: preparation.GoalQuestionAnswer}, &saved, json.RawMessage(test.payload))
			switch {
			case test.wantErr != "":
				if !errors.Is(err, preparation.ErrInvalidArgument) || !strings.Contains(err.Error(), test.wantErr) || got.Records != nil {
					t.Fatalf("ValidateReview = %+v, %v; want invalid argument containing %q and no records", got, err, test.wantErr)
				}
			default:
				want := saved.Preview[0]
				want.UserApproved = true
				want.FactID = training.FactID(want.Evidence, want.Target)
				if err != nil || !reflect.DeepEqual(got.Records, []training.Record{want}) {
					t.Fatalf("ValidateReview = %+v, %v; want approved record %+v", got, err, want)
				}
			}
		})
	}
}

func TestValidateReviewGoals(t *testing.T) {
	for _, tc := range []struct {
		goal   preparation.Goal
		target string
	}{
		{preparation.GoalQuestionAnswer, "Twelve passengers"},
		{preparation.GoalNumericPrediction, "12"},
		{preparation.GoalClassification, "Passenger ferry"},
		{preparation.GoalTextGeneration, "The ferry can carry a dozen passengers."},
	} {
		t.Run(string(tc.goal), func(t *testing.T) {
			saved := preparation.PreparationResult{DatasetID: "ferry", Preview: []training.Record{
				{Prompt: "How many passengers can the ferry carry?", Target: "12", Evidence: "The ferry carries 12 passengers.", SourceDocument: "ferry"},
			}}
			records := append([]training.Record(nil), saved.Preview...)
			records[0].Target = tc.target
			records[0].FactID = "untrusted-id"
			payload, err := json.Marshal(preparation.Review{Records: records})
			if err != nil {
				t.Fatal(err)
			}
			got, err := preview.ValidateReview(preparation.PreparationRequest{DatasetID: "ferry", Goal: tc.goal}, &saved, payload)
			if err != nil || len(got.Records) != 1 {
				t.Fatalf("ValidateReview = %+v, %v", got, err)
			}
			if !got.Records[0].UserApproved || got.Records[0].FactID != training.FactID(records[0].Evidence, tc.target) {
				t.Fatalf("missing approval or recomputed identity: %+v", got.Records[0])
			}
		})
	}
}
