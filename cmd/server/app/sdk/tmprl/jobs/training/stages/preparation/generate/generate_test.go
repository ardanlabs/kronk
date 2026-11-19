package generate_test

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
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"go.temporal.io/sdk/temporal"
)

type completer struct {
	content string
	err     error
}

func (c completer) Complete(context.Context, providers.CompletionRequest) (providers.Completion, error) {
	return providers.Completion{Content: c.content, PromptTokens: 12, CompletionTokens: 7}, c.err
}

func TestRun(t *testing.T) {
	request := preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer, Model: "controlled"}
	units := []preparation.SourceUnit{
		{Reference: "harbor", Evidence: "Harbor schedule\nThe harbor opens at dawn.", Targets: []string{"The harbor opens at dawn."}},
		{Reference: "ferry", Evidence: "Ferry schedule\nThe ferry leaves at noon and returns at dusk.", Targets: []string{"The ferry returns at dusk.", "The ferry leaves at noon."}},
	}
	valid := `{"decisions":[{"reason":"none","examples":[{"prompt":" When does the ferry leave? ","answer_quote":"The ferry leaves at noon.","target_id":0}],"evidence_id":2},{"reason":"none","examples":[{"prompt":"When does the harbor open?","answer_quote":"The harbor opens at dawn.","target_id":0}],"evidence_id":1}]}`
	outcome, usage, err := generate.Run(t.Context(), completer{content: valid}, request, units, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Decisions) != 2 || outcome.Decisions[0].Reference != "harbor" || outcome.Decisions[1].Reference != "ferry" || len(outcome.Decisions[0].Records) != 1 || len(outcome.Decisions[1].Records) != 1 || !outcome.Decisions[0].Complete || !outcome.Decisions[1].Complete {
		t.Fatalf("incorrect source decisions: %+v", outcome)
	}
	records := []training.Record{outcome.Decisions[0].Records[0], outcome.Decisions[1].Records[0]}
	if records[1].Prompt != "When does the ferry leave?" || records[1].Target != "The ferry leaves at noon." || records[1].Evidence != units[1].Evidence || records[1].FactID != training.FactID(units[1].Evidence, "The ferry leaves at noon.") || records[0].Target != "The harbor opens at dawn." || records[0].FactID == records[1].FactID {
		t.Fatalf("incorrect grounded records: %+v", records)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 {
		t.Fatalf("lost usage: %+v", usage)
	}
	for name, response := range map[string]string{
		"malformed":              `{`,
		"trailing":               valid + `{}`,
		"wrong count":            `{"decisions":[]}`,
		"repeated evidence":      strings.Replace(valid, `"evidence_id":1`, `"evidence_id":2`, 1),
		"unknown field":          strings.Replace(valid, `"decisions":`, `"extra":true,"decisions":`, 1),
		"bad evidence":           strings.Replace(valid, `"evidence_id":2`, `"evidence_id":0`, 1),
		"target from other unit": strings.Replace(valid, `"evidence_id":2`, `"evidence_id":1`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got, _, err := generate.Run(t.Context(), completer{content: response}, request, units, nil, 0, ""); err == nil || len(got.Decisions) != 0 {
				t.Fatalf("accepted invalid response: %+v, %v", got, err)
			}
		})
	}
	if got, _, err := generate.Run(t.Context(), completer{content: valid}, request, units, records, 0, ""); err != nil || len(got.Decisions) != 2 || len(got.Decisions[0].Records) != 0 || len(got.Decisions[1].Records) != 0 || !got.Decisions[0].Complete || !got.Decisions[1].Complete || got.Decisions[0].Reason != "" || got.Decisions[1].Reason != "" {
		t.Fatalf("exact retained examples must resolve without adding duplicates: %+v, %v", got, err)
	}
	if got, _, err := generate.Run(t.Context(), completer{content: strings.Replace(valid, "When does the ferry leave?", "Describe the ferry", 1)}, request, units, nil, 0, ""); err != nil || len(got.Decisions[0].Records) != 1 || len(got.Decisions[1].Records) != 0 || got.Decisions[1].Complete || !strings.Contains(got.Decisions[1].Reason, "question mark") {
		t.Fatalf("invalid question discarded the valid peer: %+v, %v", got, err)
	}
	want := errors.New("provider unavailable")
	if _, usage, err := generate.Run(t.Context(), completer{err: want}, request, units, nil, 0, ""); !errors.Is(err, want) || usage.PromptTokens != 12 {
		t.Fatalf("provider failure/usage: %+v, %v", usage, err)
	}
}

func TestRunAbstentions(t *testing.T) {
	units := []preparation.SourceUnit{
		{Reference: "question", Evidence: "Do I care about the legacy I'm leaving behind?", Targets: []string{"Do I care about the legacy I'm leaving behind?"}},
		{Reference: "katrina", Evidence: "Skill develops when we produce, not consume. — Katrina Owen", Targets: []string{"Skill develops when we produce, not consume."}},
	}
	request := preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer}
	valid := `{"decisions":[{"evidence_id":1,"examples":[],"reason":"no_supported_example"},{"evidence_id":2,"examples":[{"prompt":"What activity develops skill?","answer_quote":"Skill develops when we produce, not consume.","target_id":0}],"reason":"none"}]}`
	outcome, _, err := generate.Run(t.Context(), completer{content: valid}, request, units, nil, 0, "")
	if err != nil || len(outcome.Decisions) != 2 {
		t.Fatalf("mixed decisions: %+v, %v", outcome, err)
	}
	if outcome.Decisions[0].Reference != "question" || len(outcome.Decisions[0].Records) != 0 || !outcome.Decisions[0].Complete || outcome.Decisions[0].Reason != "model abstained: no_supported_example" || len(outcome.Decisions[1].Records) != 1 || outcome.Decisions[1].Records[0].Evidence != units[1].Evidence || outcome.Decisions[1].Records[0].Target != "Skill develops when we produce, not consume." {
		t.Fatalf("mixed decisions changed provenance: %+v", outcome)
	}
	for name, response := range map[string]string{
		"missing examples":     strings.Replace(valid, `"examples":[],`, "", 1),
		"null examples":        strings.Replace(valid, `"examples":[]`, `"examples":null`, 1),
		"missing prompt":       strings.Replace(valid, `"prompt":"What activity develops skill?",`, "", 1),
		"null prompt":          strings.Replace(valid, `"prompt":"What activity develops skill?"`, `"prompt":null`, 1),
		"missing target":       strings.Replace(valid, `,"target_id":0`, "", 1),
		"null target":          strings.Replace(valid, `"target_id":0`, `"target_id":null`, 1),
		"missing quote":        strings.Replace(valid, `"answer_quote":"Skill develops when we produce, not consume.",`, "", 1),
		"null quote":           strings.Replace(valid, `"answer_quote":"Skill develops when we produce, not consume."`, `"answer_quote":null`, 1),
		"invalid reason":       strings.Replace(valid, `"no_supported_example"`, `"none"`, 1),
		"unsupported coverage": strings.Replace(valid, `"no_supported_example"`, `"already_covered"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			got, _, err := generate.Run(t.Context(), completer{content: response}, request, units, nil, 0, "")
			if name == "invalid reason" {
				if err != nil || len(got.Decisions) != 2 || got.Decisions[0].Complete || got.Decisions[0].Reason != "an empty passage result requires reason=no_supported_example; " || len(got.Decisions[0].Records) != 0 || !got.Decisions[1].Complete || len(got.Decisions[1].Records) != 1 {
					t.Fatalf("source reason conflict discarded the valid peer or became an omission: %+v, %v", got, err)
				}
				return
			}
			if err == nil || len(got.Decisions) != 0 {
				t.Fatalf("accepted malformed exclusion or returned partial output: %+v, %v", got, err)
			}
		})
	}
}

type completionFunc func(context.Context, providers.CompletionRequest) (providers.Completion, error)

func (f completionFunc) Complete(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
	return f(ctx, request)
}

func TestRunTargetIDSchema(t *testing.T) {
	unit := preparation.SourceUnit{Reference: "fleet", Evidence: "The fleet has 11 red boats and 22 blue boats.", Targets: []string{"11", "22"}}
	for _, test := range []struct {
		goal preparation.Goal
		ids  []any
	}{
		{preparation.GoalQuestionAnswer, []any{0}},
		{preparation.GoalTextGeneration, []any{0}},
		{preparation.GoalNumericPrediction, []any{1, 2}},
		{preparation.GoalClassification, []any{1, 2}},
	} {
		t.Run(string(test.goal), func(t *testing.T) {
			var schema map[string]any
			model := completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				schema = request.ResponseSchema
				if schema == nil {
					t.Fatal("generation must constrain model output, not just describe the schema in the prompt")
				}
				return providers.Completion{Content: `{"decisions":[{"evidence_id":1,"examples":[],"reason":"no_supported_example"}]}`}, nil
			})
			if _, _, err := generate.Run(t.Context(), model, preparation.PreparationRequest{Goal: test.goal}, []preparation.SourceUnit{unit}, nil, 0, ""); err != nil {
				t.Fatal(err)
			}
			decision := schema["properties"].(map[string]any)["decisions"].(map[string]any)["items"].(map[string]any)
			example := decision["properties"].(map[string]any)["examples"].(map[string]any)["items"].(map[string]any)
			target := example["properties"].(map[string]any)["target_id"].(map[string]any)
			gotIDs, err := json.Marshal(target["enum"])
			if err != nil {
				t.Fatal(err)
			}
			wantIDs, err := json.Marshal(test.ids)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotIDs) != string(wantIDs) {
				t.Fatalf("target IDs allowed by schema: %v, want %v", target["enum"], test.ids)
			}
		})
	}
}

// spanDecision builds the current wire format directly, not a legacy response.
func spanDecision(id int, prompt, quote string) map[string]any {
	return map[string]any{"evidence_id": id, "reason": "none", "examples": []map[string]any{{"prompt": prompt, "answer_quote": quote, "target_id": 0}}}
}

func TestRunExampleCountsAndExactSpans(t *testing.T) {
	evidence := "Harbor opens at dawn; the code is fmt.Println(\"Hello, \\\"reader\\\"!\").\nThe café files are in C:\\library\\returns."
	unit := preparation.SourceUnit{Reference: "schedule", Evidence: evidence, Targets: []string{evidence}}
	quotes := []string{"dawn", `fmt.Println("Hello, \"reader\"!")`, `café files are in C:\library\returns`}
	for _, goal := range []preparation.Goal{preparation.GoalQuestionAnswer, preparation.GoalTextGeneration} {
		for _, count := range []int{0, 1, 3, 4} {
			t.Run(fmt.Sprintf("%s/%d", goal, count), func(t *testing.T) {
				examples := []map[string]any{}
				for i := range count {
					examples = append(examples, map[string]any{"prompt": fmt.Sprintf("What is schedule fact %d?", i), "answer_quote": quotes[i%3], "target_id": 0})
				}
				reason := "none"
				if count == 0 {
					reason = "no_supported_example"
				}
				data, err := json.Marshal(map[string]any{"decisions": []any{map[string]any{"evidence_id": 1, "examples": examples, "reason": reason}}})
				if err != nil {
					t.Fatal(err)
				}
				out, _, err := generate.Run(t.Context(), completer{content: string(data)}, preparation.PreparationRequest{Goal: goal}, []preparation.SourceUnit{unit}, nil, 0, "")
				if count == 4 {
					if err == nil || len(out.Decisions) != 0 {
						t.Fatalf("accepted four examples: %+v, %v", out, err)
					}
					return
				}
				if err != nil || len(out.Decisions) != 1 || !out.Decisions[0].Complete || len(out.Decisions[0].Records) != count {
					t.Fatalf("count %d: %+v, %v", count, out, err)
				}
				for i, r := range out.Decisions[0].Records {
					if r.Target != quotes[i] || r.Evidence != unit.Evidence || r.FactID != training.FactID(unit.Evidence, quotes[i]) {
						t.Fatalf("answer span was expanded or changed: %+v", r)
					}
				}
			})
		}
	}
}

func TestRunRejectsFabricatedOrRepeatedSpans(t *testing.T) {
	unit := preparation.SourceUnit{Reference: "schedule", Evidence: "Harbor opens at dawn. Ferry leaves at noon. Secret evidence-only label.", Targets: []string{"Harbor opens at dawn.", "Ferry leaves at noon."}}
	for _, quote := range []string{"at midnight", "dawn. Ferry", "Secret evidence-only label", "DAWN", "dawn", "dawn.*", "Harbor opens at dawn?", "Harboropens at dawn."} {
		t.Run(quote, func(t *testing.T) {
			row := spanDecision(1, "When does the harbor open?", "dawn")
			row["examples"] = append(row["examples"].([]map[string]any), map[string]any{"prompt": "What is the other schedule fact?", "answer_quote": quote, "target_id": 0})
			data, err := json.Marshal(map[string]any{"decisions": []any{row}})
			if err != nil {
				t.Fatal(err)
			}
			out, _, err := generate.Run(t.Context(), completer{content: string(data)}, preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer}, []preparation.SourceUnit{unit}, nil, 0, "")
			if err != nil || len(out.Decisions) != 1 || out.Decisions[0].Complete || len(out.Decisions[0].Records) != 1 || out.Decisions[0].Records[0].Target != "dawn" || out.Decisions[0].Reason == "" {
				t.Fatalf("invalid answer lost valid peer or resolved passage: %+v, %v", out, err)
			}
		})
	}
}

func TestRunIndexedTargets(t *testing.T) {
	for _, goal := range []preparation.Goal{preparation.GoalNumericPrediction, preparation.GoalClassification} {
		unit := preparation.SourceUnit{Reference: "source", Evidence: "There are 12 red boats.", Targets: []string{"12", "red"}}
		id := 1
		if goal == preparation.GoalClassification {
			id = 2
		}
		for _, invalid := range []string{"", "zero", "out of range", "nonempty quote"} {
			t.Run(fmt.Sprintf("%s/%s", goal, invalid), func(t *testing.T) {
				targetID, quote := id, ""
				switch invalid {
				case "zero":
					targetID = 0
				case "out of range":
					targetID = 3
				case "nonempty quote":
					quote = unit.Targets[id-1]
				}
				response := fmt.Sprintf(`{"decisions":[{"evidence_id":1,"examples":[{"prompt":"Describe the boats","answer_quote":%q,"target_id":%d}],"reason":"none"}]}`, quote, targetID)
				out, _, err := generate.Run(t.Context(), completer{content: response}, preparation.PreparationRequest{Goal: goal}, []preparation.SourceUnit{unit}, nil, 0, "")
				if err != nil || len(out.Decisions) != 1 {
					t.Fatalf("indexed target: %+v, %v", out, err)
				}
				decision := out.Decisions[0]
				if invalid == "" {
					if !decision.Complete || len(decision.Records) != 1 || decision.Records[0].Target != unit.Targets[id-1] {
						t.Fatalf("lost indexed target: %+v", decision)
					}
				} else if decision.Complete || len(decision.Records) != 0 {
					t.Fatalf("accepted invalid indexed target: %+v", decision)
				}
			})
		}
	}
}

func TestRunSavedCandidateLimit(t *testing.T) {
	unit := preparation.SourceUnit{Reference: "schedule", Evidence: "dawn noon dusk midnight", Targets: []string{"dawn noon dusk midnight"}}
	var saved []training.Record
	for i, quote := range []string{"dawn", "noon", "dusk"} {
		r := training.Record{Prompt: fmt.Sprintf("Saved question %d?", i), Target: quote, Evidence: unit.Evidence, FactID: training.FactID(unit.Evidence, quote)}
		saved = append(saved, r)
	}
	for _, response := range []string{
		`{"decisions":[{"evidence_id":1,"examples":[],"reason":"none"}]}`,
		`{"decisions":[{"evidence_id":1,"examples":[{"prompt":"What is the fourth time?","answer_quote":"midnight","target_id":0}],"reason":"none"}]}`,
		`{"decisions":[{"evidence_id":1,"examples":[],"reason":"no_supported_example"}]}`,
	} {
		out, _, err := generate.Run(t.Context(), completer{content: response}, preparation.PreparationRequest{Goal: preparation.GoalQuestionAnswer}, []preparation.SourceUnit{unit}, saved, 1, "repair")
		switch {
		case strings.Contains(response, `"reason":"none"`) && strings.Contains(response, `"examples":[]`):
			if err != nil || len(out.Decisions) != 1 || !out.Decisions[0].Complete || len(out.Decisions[0].Records) != 0 || out.Decisions[0].Reason != "" {
				t.Fatalf("saved-only passage must resolve without an omission or replayed records: %+v, %v", out, err)
			}
		case strings.Contains(response, `"reason":"no_supported_example"`):
			if err != nil || len(out.Decisions) != 1 || out.Decisions[0].Complete || len(out.Decisions[0].Records) != 0 || out.Decisions[0].Reason != "a passage with new or saved examples requires reason=none; " {
				t.Fatalf("unsupported exclusion of saved work must remain a source conflict: %+v, %v", out, err)
			}
		default:
			if err != nil || len(out.Decisions) != 1 || out.Decisions[0].Complete || len(out.Decisions[0].Records) != 0 || !strings.Contains(out.Decisions[0].Reason, "at most three") {
				t.Fatalf("fourth candidate must remain a source conflict without discarding peers: %+v, %v", out, err)
			}
		}
	}
}

// evaluationInput models the lighthouse failure boundary: three accepted
// examples, seventeen generated examples, and four saved evaluation questions.
func evaluationInput(t *testing.T) (preparation.StepContext, checkpoint.State) {
	t.Helper()
	workspace, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspace.Close() })
	input := preparation.StepContext{Workspace: workspace, Preparation: &preparation.PreparationContext{
		Input:  preparation.PreparationInput{Kind: preparation.PreparationFull, Request: preparation.PreparationRequest{Model: "controlled", Goal: preparation.GoalQuestionAnswer}},
		Source: uploads.File{ID: "source"}, OpenSource: func(context.Context) (io.ReadCloser, error) {
			t.Fatal("recovery must not reopen the source")
			return nil, nil
		},
	}}
	state, _, err := checkpoint.Load(t.Context(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		evidence := fmt.Sprintf("Harbor %d opens at dawn.", i)
		r := training.Record{Prompt: fmt.Sprintf("When does harbor %d open?", i), Target: evidence, Evidence: evidence, SourceDocument: "source", UserApproved: i < 3}
		r.FactID = training.FactID(r.Evidence, r.Target)
		state.Records = append(state.Records, r)
		if i >= 3 {
			state.Units = append(state.Units, preparation.SourceUnit{Reference: fmt.Sprintf("harbor-%d", i), Evidence: evidence, Targets: []string{evidence}})
		}
	}
	input.Review = &preparation.Review{Records: slices.Clone(state.Records[:3])}
	input.ReviewID = "accepted-review"
	state.ReviewID, state.ReviewedCount = input.ReviewID, 3
	for i := range 4 {
		r := state.Records[i]
		r.Prompt = fmt.Sprintf("At what time does harbor %d open?", i)
		r.UserApproved, r.ValidationPrompt = false, true
		state.Records = append(state.Records, r)
	}
	state.Next, state.EvaluationNext = 17, 4
	state.Result.PromptTokens, state.Result.CompletionTokens = 101, 47
	if err := tmprl.WriteJSON(workspace, checkpoint.File, state); err != nil {
		t.Fatal(err)
	}
	return input, state
}

// evaluationCounts returns the validation and test questions among records.
func evaluationCounts(records []training.Record) (validation, test int) {
	for _, r := range records {
		switch {
		case r.ValidationPrompt:
			validation++
		case r.IndependentPrompt:
			test++
		}
	}
	return validation, test
}

func savedState(t *testing.T, input preparation.StepContext) checkpoint.State {
	t.Helper()
	data, err := input.Workspace.ReadFile(checkpoint.File)
	if err != nil {
		t.Fatal(err)
	}
	var state checkpoint.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func reopen(t *testing.T, input *preparation.StepContext) {
	t.Helper()
	name := input.Workspace.Name()
	input.Workspace.Close()
	workspace, err := os.OpenRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspace.Close() })
	input.Workspace = workspace
}

// generationInput reproduces the saved PDF failure boundary without a model:
// three approved examples, twenty-five committed generated examples, then a
// five-passage batch followed by one more pending passage.
func generationInput(t *testing.T) (preparation.StepContext, checkpoint.State) {
	t.Helper()
	input, state := evaluationInput(t)
	state.Records = state.Records[:20]
	for i := 20; i < 28; i++ {
		evidence := fmt.Sprintf("Harbor %d opens at dawn.", i)
		r := training.Record{Prompt: fmt.Sprintf("When does harbor %d open?", i), Target: evidence, Evidence: evidence, SourceDocument: "source"}
		r.FactID = training.FactID(r.Evidence, r.Target)
		state.Records = append(state.Records, r)
		state.Units = append(state.Units, preparation.SourceUnit{Reference: fmt.Sprintf("harbor-%d", i), Evidence: evidence, Targets: []string{evidence}})
	}
	for i := 28; i < 34; i++ {
		evidence := fmt.Sprintf("Harbor %d opens at dawn.", i)
		state.Units = append(state.Units, preparation.SourceUnit{Reference: fmt.Sprintf("harbor-%d", i), Evidence: evidence, Targets: []string{evidence}})
	}
	state.Next, state.EvaluationNext = 25, 0
	if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
		t.Fatal(err)
	}
	return input, state
}

func TestGenerationSafePointContinuation(t *testing.T) {
	for _, phase := range []string{"generation", "evaluation"} {
		t.Run(phase, func(t *testing.T) {
			input, initial := generationInput(t)
			// Training records plus 8 validation and 16 test questions.
			wantCount := 58
			if phase == "evaluation" {
				input, initial = evaluationInput(t)
				wantCount = 44
			}
			paused := errors.New("controlled safe-point suspension")
			input.SafePoint = func(_ context.Context, path string) error {
				if path != checkpoint.File {
					t.Fatalf("unexpected checkpoint: %s", path)
				}
				saved := savedState(t, input)
				if phase == "generation" && saved.Next == 30 || phase == "evaluation" && saved.EvaluationNext == 6 {
					return paused
				}
				return nil
			}
			evaluations := initial.EvaluationNext
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				if !strings.Contains(request.Messages[1].Content, "[EVIDENCE 1]") {
					evaluations++
					return providers.Completion{Content: fmt.Sprintf(`{"record_1":"Which hour applies in evaluation question %d?"}`, evaluations)}, nil
				}
				var decisions []any
				for _, unit := range initial.Units {
					if strings.Contains(request.Messages[1].Content, "\n"+unit.Evidence+"\n") {
						decisions = append(decisions, spanDecision(len(decisions)+1, "What is the opening schedule for "+unit.Reference+"?", unit.Evidence))
					}
				}
				data, err := json.Marshal(map[string]any{"decisions": decisions})
				return providers.Completion{Content: string(data)}, err
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, paused) {
				t.Fatalf("did not suspend at saved progress: %v", err)
			}
			saved := savedState(t, input)
			if saved.Generated || saved.ReviewID != initial.ReviewID || !slices.Equal(saved.Records[:len(initial.Records)], initial.Records) || saved.GenerationAttempts != 0 {
				t.Fatalf("pause damaged review, records or request budget: %+v", saved)
			}
			reopen(t, &input)
			input.SafePoint = nil
			if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			completed := savedState(t, input)
			if validation, test := evaluationCounts(completed.Records); !completed.Generated || len(completed.Records) != wantCount || validation != 8 || test != 16 || !slices.Equal(completed.Records[:len(saved.Records)], saved.Records) || completed.ReviewID != initial.ReviewID {
				t.Fatalf("Continue repeated or lost committed progress: %+v", completed)
			}
		})
	}
}

func TestRequestedRecordsAcrossPasses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		target     int
		want       int
		reject     bool
		pause      bool
		noProgress bool
	}{
		{name: "fifty from six passages", target: 50, want: 50},
		{name: "replace rejected records", target: 50, want: 50, reject: true},
		{name: "resume additional pass", target: 50, want: 50, pause: true},
		{name: "trim final batch", target: 7, want: 7},
		{name: "unlimited unchanged", want: 6},
		{name: "no progress saves available records", target: 50, want: 6, noProgress: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := evaluationInput(t)
			input.Preparation.Input.Request.MaxQuestions = tc.target
			input.Review, input.ReviewID = nil, ""
			if err := input.Workspace.Remove(checkpoint.File); err != nil {
				t.Fatal(err)
			}
			state, _, err := checkpoint.Load(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			for i := range 6 {
				evidence := fmt.Sprintf("Facility %d opens at dawn.", i)
				state.Units = append(state.Units, preparation.SourceUnit{Reference: fmt.Sprintf("facility-%d", i), Evidence: evidence, Targets: []string{evidence}})
			}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			attempts := make([]int, 6)
			rejected := false
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				prompt := request.Messages[1].Content
				if request.EnableThinking {
					verdicts := make(map[string]string)
					for _, block := range strings.Split(prompt, "[RECORD ")[1:] {
						var index int
						if _, err := fmt.Sscanf(block, "%d]", &index); err != nil {
							t.Fatal(err)
						}
						verdict := "pass"
						if tc.reject && strings.Contains(block, "PROMPT: When does facility 0 open (question 1)?") {
							verdict, rejected = "task_quality", true
						}
						verdicts[fmt.Sprintf("record_%d", index)] = verdict
					}
					data, err := json.Marshal(verdicts)
					return providers.Completion{Content: string(data)}, err
				}
				var decisions []any
				for i, unit := range state.Units {
					if !strings.Contains(prompt, "\n"+unit.Evidence+"\n") {
						continue
					}
					attempts[i]++
					number := attempts[i]
					if tc.noProgress {
						number = 1
					}
					decisions = append(decisions, spanDecision(len(decisions)+1, fmt.Sprintf("When does facility %d open (question %d)?", i, number), "dawn"))
				}
				data, err := json.Marshal(map[string]any{"decisions": decisions})
				return providers.Completion{Content: string(data)}, err
			})
			step := generate.Step{Verify: verify.Batch}
			if tc.pause {
				paused := errors.New("pause during additional generation")
				input.SafePoint = func(context.Context, string) error {
					saved := savedState(t, input)
					if saved.GenerationRound == 2 && saved.Next == 5 && saved.GenerationAttempts == 0 {
						return paused
					}
					return nil
				}
				if _, err := step.Execute(t.Context(), input); !errors.Is(err, paused) {
					t.Fatalf("additional pass did not pause: %v", err)
				}
				reopen(t, &input)
				input.SafePoint = nil
			}
			if _, err := step.Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			result, err := (verify.Step{}).Execute(t.Context(), input)
			if err != nil || result.Artifacts.Preparation == nil || result.Artifacts.Preparation.RecordCount != tc.want {
				t.Fatalf("requested %d: published %+v, %v; want %d", tc.target, result.Artifacts.Preparation, err, tc.want)
			}
			file, err := input.Workspace.Open("dataset.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			records, err := training.ReadRecords(t.Context(), file)
			if err != nil || len(records) != tc.want {
				t.Fatalf("saved records = %d, %v; want %d", len(records), err, tc.want)
			}
			for _, record := range records {
				if record.Target != "dawn" || tc.reject && record.Prompt == "When does facility 0 open (question 1)?" {
					t.Fatalf("unsupported or rejected record published: %+v", record)
				}
			}
			if tc.reject && !rejected {
				t.Fatal("test did not exercise verification rejection")
			}
			if tc.noProgress && !strings.Contains(strings.Join(result.Artifacts.Preparation.Warnings, " "), "three passes") {
				t.Fatal("short preparation omitted no-progress warning")
			}
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("completed generation repeated on replay")
				return providers.Completion{}, nil
			})
			if _, err := step.Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerificationFeedbackRepairsRecordsAfterResume(t *testing.T) {
	for _, category := range []string{"grounding", "task_quality", "user_requirements"} {
		t.Run(category, func(t *testing.T) {
			input, _ := evaluationInput(t)
			input.Preparation.Input.Request.MaxQuestions = 1
			input.Review, input.ReviewID = nil, ""
			if err := input.Workspace.Remove(checkpoint.File); err != nil {
				t.Fatal(err)
			}
			state, _, err := checkpoint.Load(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			const evidence = "Interface guidance\nDon't use an interface:\nFor the sake of using an interface."
			const bad = "What is the reason given for using an interface in the text?"
			const good = "Which reason for using an interface should be avoided?"
			const target = "For the sake of using an interface."
			state.Units = []preparation.SourceUnit{{Reference: "interface", Evidence: evidence, Targets: []string{target}}}
			state.Result.Exclusions = []preparation.SourceExclusion{{Reference: "other", Reason: `verification rejected example "unrelated question": grounding`}}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				prompt := request.Messages[1].Content
				if request.EnableThinking {
					outcome := category
					if strings.Contains(prompt, "PROMPT: "+good) {
						outcome = "pass"
					}
					return providers.Completion{Content: fmt.Sprintf(`{"record_1":%q}`, outcome)}, nil
				}
				question := bad
				if strings.Contains(prompt, "verification rejected example") && strings.Contains(prompt, bad) && strings.Contains(prompt, ": "+category) {
					question = good
				}
				if strings.Contains(prompt, "unrelated question") {
					t.Fatal("another passage's verification feedback leaked into this repair")
				}
				data, err := json.Marshal(map[string]any{"decisions": []any{spanDecision(1, question, target)}})
				return providers.Completion{Content: string(data)}, err
			})
			paused := errors.New("pause after verification rejection")
			input.SafePoint = func(context.Context, string) error {
				if len(savedState(t, input).Result.Exclusions) > 1 {
					return paused
				}
				return nil
			}
			step := generate.Step{Verify: verify.Batch}
			if _, err := step.Execute(t.Context(), input); !errors.Is(err, paused) {
				t.Fatalf("did not pause with a durable rejection: %v", err)
			}
			reopen(t, &input)
			input.SafePoint = nil
			if _, err := step.Execute(t.Context(), input); err != nil {
				t.Fatalf("repair did not produce a verified replacement: %v", err)
			}
			result, err := (verify.Step{}).Execute(t.Context(), input)
			if err != nil || result.Artifacts.Preparation == nil || result.Artifacts.Preparation.RecordCount != 1 {
				t.Fatalf("replacement was not saved: %+v, %v", result, err)
			}
			file, err := input.Workspace.Open("dataset.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			records, err := training.ReadRecords(t.Context(), file)
			if err != nil || len(records) != 1 || records[0].Prompt != good || records[0].Target != target || records[0].Evidence != evidence {
				t.Fatalf("saved incorrect replacement: %+v, %v", records, err)
			}
		})
	}
}

func TestDocumentQuestionCap(t *testing.T) {
	// The limit covers training records and the 24 evaluation questions that 20
	// or more training records need. Below 44 there is no room for both, so the
	// run saves training records only. The document has 34 passages. A dataset
	// that already holds 52 training records needs its 24 questions from this
	// run, rewording its earlier records. Verification rejections are replaced.
	for _, test := range []struct {
		name          string
		limit         int
		want          int
		evaluation    int
		base          int
		reject        bool
		rejectVariant bool
	}{
		{name: "3", limit: 3, want: 3}, {name: "20", limit: 20, want: 20}, {name: "21", limit: 21, want: 21}, {name: "43", limit: 43, want: 34},
		{name: "44", limit: 44, want: 44, evaluation: 24}, {name: "50", limit: 50, want: 50, evaluation: 24}, {name: "60", limit: 60, want: 58, evaluation: 24}, {name: "0", want: 58, evaluation: 24},
		{name: "dataset", limit: 30, want: 30, evaluation: 24, base: 52},
		{name: "rejected", limit: 20, want: 20, reject: true},
		{name: "rejected variant preserves evaluation", limit: 50, want: 50, evaluation: 24, rejectVariant: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, state := generationInput(t)
			input.Preparation.Input.Request.MaxQuestions = test.limit
			baseFacts := make(map[string]bool)
			for i := range test.base {
				evidence := fmt.Sprintf("Pier %d closes at dusk.", i)
				r := training.Record{Prompt: fmt.Sprintf("When does pier %d close?", i), Target: evidence, Evidence: evidence, SourceDocument: "earlier"}
				r.FactID = training.FactID(r.Evidence, r.Target)
				input.Preparation.DatasetRecords = append(input.Preparation.DatasetRecords, r)
				baseFacts[r.FactID] = true
			}
			step := generate.Step{}
			rejected := func(r training.Record) bool {
				if test.rejectVariant {
					return strings.Contains(r.Prompt, " rejected variant?")
				}
				return strings.HasSuffix(r.Prompt, " harbor-5?") || strings.HasSuffix(r.Prompt, " harbor-12?")
			}
			if test.reject || test.rejectVariant {
				step.Verify = func(_ context.Context, input preparation.StepContext, state *checkpoint.State) error {
					kept := state.Records[:state.Verified]
					for _, r := range state.Records[state.Verified:] {
						if !rejected(r) {
							kept = append(kept, r)
						}
					}
					state.Records, state.Verified = kept, len(kept)
					return tmprl.WriteJSON(input.Workspace, checkpoint.File, *state)
				}
			}
			if err := input.Workspace.Remove(checkpoint.File); err != nil {
				t.Fatal(err)
			}
			identity, _, err := checkpoint.Load(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			state.Fingerprint = identity.Fingerprint
			state.Records = state.Records[:3]
			state.Next = 0
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			evaluations := 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				if !strings.Contains(request.Messages[1].Content, "[EVIDENCE 1]") {
					evaluations++
					return providers.Completion{Content: fmt.Sprintf(`{"record_1":"What is the opening time for evaluation question %d?"}`, evaluations)}, nil
				}
				var decisions []any
				for _, unit := range state.Units {
					if !strings.Contains(request.Messages[1].Content, "\n"+unit.Evidence+"\n") {
						continue
					}
					examples := []any{map[string]any{"prompt": "What is the opening schedule? " + unit.Reference + "?", "answer_quote": unit.Evidence, "target_id": 0}}
					if test.rejectVariant && unit.Reference == "harbor-3" {
						examples = append(examples, map[string]any{"prompt": "What is the opening schedule? rejected variant?", "answer_quote": unit.Evidence, "target_id": 0})
					}
					decisions = append(decisions, map[string]any{"evidence_id": len(decisions) + 1, "reason": "none", "examples": examples})
				}
				data, err := json.Marshal(map[string]any{"decisions": decisions})
				return providers.Completion{Content: string(data)}, err
			})
			if _, err := step.Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			got := savedState(t, input)
			validation, independent := evaluationCounts(got.Records)
			if len(got.Records) != test.want || validation+independent != test.evaluation || !got.Generated || !slices.Equal(got.Records[:3], input.Review.Records) {
				t.Fatalf("cap %d: got %d records with %d evaluation questions, generated=%v; want %d with %d and review preserved", test.limit, len(got.Records), validation+independent, got.Generated, test.want, test.evaluation)
			}
			if evaluations != test.evaluation {
				t.Fatalf("cap %d: %d evaluation requests, want %d", test.limit, evaluations, test.evaluation)
			}
			fromBase := 0
			for _, r := range got.Records {
				if r.ValidationPrompt || r.IndependentPrompt {
					if baseFacts[r.FactID] {
						fromBase++
					}
					if r.SourceDocument != "source" {
						t.Fatalf("evaluation question not owned by this run's document: %+v", r)
					}
				}
				if test.reject && rejected(r) {
					t.Fatalf("rejected record was saved: %q", r.Prompt)
				}
			}
			// Six run records cover 6 validation and 6 test questions; the rest
			// reword the dataset's earlier records.
			if test.base > 0 && fromBase != 12 || test.base == 0 && fromBase != 0 {
				t.Fatalf("%d evaluation questions reword earlier dataset records", fromBase)
			}
			if test.reject && got.Verified != len(got.Records) {
				t.Fatalf("verified %d of %d records", got.Verified, len(got.Records))
			}
			reopen(t, &input)
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("completed capped generation called the provider on replay")
				return providers.Completion{}, nil
			})
			if _, err := step.Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			if replay := savedState(t, input); !reflect.DeepEqual(replay, got) {
				t.Fatal("replay changed capped results")
			}
		})
	}
}

func TestFiveQuestionCapWithReviewedPassage(t *testing.T) {
	input, state := evaluationInput(t)
	input.Preparation.Input.Request.MaxQuestions = 5
	if err := input.Workspace.Remove(checkpoint.File); err != nil {
		t.Fatal(err)
	}
	identity, _, err := checkpoint.Load(t.Context(), input, true)
	if err != nil {
		t.Fatal(err)
	}
	state.Fingerprint = identity.Fingerprint
	unit := preparation.SourceUnit{Reference: "schedule", Evidence: "Harbor opens at dawn; ferry leaves at noon; café closes at dusk; gate locks at midnight.", Targets: []string{"Harbor opens at dawn; ferry leaves at noon; café closes at dusk; gate locks at midnight."}}
	state.Records = state.Records[:3]
	state.Records[0].Evidence, state.Records[0].Target = unit.Evidence, "dawn"
	state.Records[0].FactID = training.FactID(unit.Evidence, "dawn")
	state.Units = append([]preparation.SourceUnit{unit}, state.Units...)
	state.Next, state.EvaluationNext = 0, 0
	input.Review.Records = slices.Clone(state.Records)
	if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
		t.Fatal(err)
	}
	calls := 0
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		calls++
		if calls > 1 {
			t.Fatal("generation retried after enough valid questions filled the cap")
		}
		if strings.Contains(request.Messages[1].Content, "[EVIDENCE 3]") {
			t.Fatal("requested more passages than the two remaining questions")
		}
		return providers.Completion{Content: `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does the ferry leave?","answer_quote":"noon","target_id":0},{"prompt":"When does the café close?","answer_quote":"dusk","target_id":0},{"prompt":"When does the gate lock?","answer_quote":"midnight","target_id":0}],"reason":"none"},{"evidence_id":2,"examples":[{"prompt":"What is the next harbor schedule?","answer_quote":"Harbor 3 opens at dawn.","target_id":0}],"reason":"none"}]}`}, nil
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	got := savedState(t, input)
	if !got.Generated || len(got.Records) != 5 || !slices.Equal(got.Records[:3], input.Review.Records) || got.Records[3].Target != "noon" || got.Records[4].Target != "dusk" {
		t.Fatalf("lost reviewed or valid generated examples: %+v", got)
	}
	if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationReviewedPassageCapacity(t *testing.T) {
	for _, mode := range []string{"two new answers", "approved only", "approved counts toward cap"} {
		t.Run(mode, func(t *testing.T) {
			input, state := evaluationInput(t)
			unit := preparation.SourceUnit{Reference: "schedule", Evidence: "Harbor opens at dawn; ferry leaves at noon; café closes at dusk; gate locks at midnight.", Targets: []string{"Harbor opens at dawn; ferry leaves at noon; café closes at dusk; gate locks at midnight."}}
			approved := training.Record{Prompt: "When does the harbor open?", Target: "dawn", Evidence: unit.Evidence, FactID: training.FactID(unit.Evidence, "dawn"), SourceDocument: "source", UserApproved: true}
			state.Units, state.Records = []preparation.SourceUnit{unit}, nil
			state.Next, state.EvaluationNext, state.ReviewedCount, state.ReviewID = 0, 0, 0, ""
			input.Review = &preparation.Review{Records: []training.Record{approved}}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			interrupted := errors.New("interrupted reviewed-passage request")
			calls := 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				prompt := request.Messages[1].Content
				saved := `[SAVED EXAMPLE] prompt="When does the harbor open?" answer="dawn"`
				if !strings.Contains(prompt, unit.Evidence+"\n"+saved+"\n") || strings.Index(prompt, saved) > strings.Index(prompt, "ANSWER SOURCES:") {
					t.Fatalf("approved example must be inside its retained evidence block before answer sources: %s", prompt)
				}
				if calls == 2 || mode == "approved only" {
					return providers.Completion{}, interrupted
				}
				response := `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does the ferry leave?","answer_quote":"noon","target_id":0},{"prompt":"When does the café close?","answer_quote":"invented","target_id":0}],"reason":"none"}]}`
				if mode == "approved counts toward cap" {
					response = `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does the ferry leave?","answer_quote":"noon","target_id":0},{"prompt":"When does the café close?","answer_quote":"dusk","target_id":0},{"prompt":"When does the gate lock?","answer_quote":"midnight","target_id":0}],"reason":"none"}]}`
				}
				return providers.Completion{Content: response}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, interrupted) {
				t.Fatalf("expected durable interruption: %v", err)
			}
			reopen(t, &input)
			pending := savedState(t, input)
			if len(pending.Units) != 1 || !slices.Equal(pending.Records, []training.Record{approved}) || pending.Next != 0 || len(pending.GenerationDecisions) != 1 {
				t.Fatalf("review or pending passage lost: %+v", pending)
			}
			if mode == "two new answers" && (len(pending.GenerationDecisions[0].Records) != 1 || pending.GenerationDecisions[0].Records[0].Target != "noon" || pending.GenerationDecisions[0].Complete) {
				t.Fatalf("partial candidate was not preserved beside approved example: %+v", pending.GenerationDecisions)
			}
			if mode == "approved counts toward cap" && (len(pending.GenerationDecisions[0].Records) != 2 || !strings.Contains(pending.GenerationDecisions[0].Reason, "at most three")) {
				t.Fatalf("approved peer did not consume capacity: %+v", pending)
			}
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				if mode == "two new answers" && !strings.Contains(request.Messages[1].Content, `[SAVED EXAMPLE] prompt="When does the ferry leave?" answer="noon"`) {
					t.Fatal("reloaded repair lost staged answer")
				}
				examples := []map[string]any{}
				if mode == "approved counts toward cap" {
					examples = append(examples, map[string]any{"prompt": "When does the ferry leave?", "answer_quote": "noon", "target_id": 0})
				}
				if mode != "approved only" {
					examples = append(examples, map[string]any{"prompt": "When does the café close?", "answer_quote": "dusk", "target_id": 0})
				}
				data, err := json.Marshal(map[string]any{"decisions": []any{map[string]any{"evidence_id": 1, "examples": examples, "reason": "none"}}})
				return providers.Completion{Content: string(data)}, err
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			reopen(t, &input)
			completed := savedState(t, input)
			count := 3
			if mode == "approved only" {
				count = 1
			}
			if len(completed.Records) != count || completed.Records[0] != approved || !completed.Generated || completed.Next != 1 || len(completed.Result.Exclusions) != 0 {
				t.Fatalf("reviewed-passage completion changed approval, count, or omissions: %+v", completed)
			}
			if count == 3 && (completed.Records[1].Target != "noon" || completed.Records[2].Target != "dusk" || completed.Records[1].UserApproved || completed.Records[2].UserApproved) {
				t.Fatalf("repair lost distinct new answers: %+v", completed.Records)
			}
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("completed reviewed passage was regenerated on replay")
				return providers.Completion{}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			if replay := savedState(t, input); !reflect.DeepEqual(replay, completed) {
				t.Fatal("replay changed completed reviewed passage")
			}
		})
	}
}

func TestGenerationReportsOmissions(t *testing.T) {
	input, original := generationInput(t)
	stop := errors.New("stop before evaluations")
	calls := 0
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		if strings.Contains(request.Messages[1].Content, "Locked target:") {
			return providers.Completion{}, stop
		}
		count := 5
		if calls == 1 {
			count = 1
		}
		calls++
		rows := []map[string]any{}
		for i := range count {
			rows = append(rows, map[string]any{"evidence_id": i + 1, "examples": []any{}, "reason": "no_supported_example"})
		}
		data, err := json.Marshal(map[string]any{"decisions": rows})
		return providers.Completion{Content: string(data)}, err
	})
	for range 2 {
		if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 2 {
			t.Fatalf("omissions did not resolve or were regenerated: calls=%d err=%v", calls, err)
		}
		reopen(t, &input)
		state := savedState(t, input)
		if state.Next != len(original.Units) || !slices.Equal(state.Records, original.Records) || len(state.Result.Exclusions) != 6 || state.GenerationAttempts != 0 || len(state.GenerationDecisions) != 0 {
			t.Fatalf("omissions changed records or were not committed: %+v", state)
		}
		for i, exclusion := range state.Result.Exclusions {
			if exclusion.Reference != original.Units[original.Next+i].Reference || exclusion.Reason != "model abstained: no_supported_example" {
				t.Fatalf("omission report lost source or model-selected reason: %+v", exclusion)
			}
		}
	}
}

func TestGenerationEmptyOutputFailure(t *testing.T) {
	for _, tc := range []struct {
		name, response, failure string
	}{
		{"malformed", "```json\n{}\n```", "preparation-request-budget-exhausted"},
		{"unsupported answer", `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does the harbor open?","answer_quote":"midnight","target_id":0}],"reason":"none"}]}`, "preparation-request-budget-exhausted"},
		{"abstention", `{"decisions":[{"evidence_id":1,"examples":[],"reason":"no_supported_example"}]}`, "preparation-no-examples"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, state := evaluationInput(t)
			input.Review, input.ReviewID = nil, ""
			state = checkpoint.State{Version: state.Version, Fingerprint: state.Fingerprint, Units: state.Units[:1], Result: state.Result}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completer{content: tc.response}
			for range 2 {
				_, err := (generate.Step{}).Execute(t.Context(), input)
				failure, ok := errors.AsType[*temporal.ApplicationError](err)
				if !ok || failure.Type() != tc.failure || !failure.NonRetryable() {
					t.Fatalf("failure = %v; want nonretryable %s", err, tc.failure)
				}
				reopen(t, &input)
				if got := savedState(t, input); len(got.Records) != 0 || got.Generated || len(got.Result.Exclusions) != 1 {
					t.Fatalf("invalid output was saved as examples or lost its diagnostic: %+v", got)
				}
			}
		})
	}
}

func TestGenerationPartialCandidateRecovery(t *testing.T) {
	for _, finish := range []string{"new candidate", "saved only"} {
		t.Run(finish, func(t *testing.T) {
			input, original := generationInput(t)
			unit := &original.Units[original.Next]
			unit.Evidence = "Harbor 28 opens at dawn and closes at dusk."
			unit.Targets = []string{unit.Evidence}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, original); err != nil {
				t.Fatal(err)
			}
			interrupted := errors.New("repair transport interrupted")
			calls := 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				if calls == 2 {
					if !strings.Contains(request.Messages[1].Content, `answer="dawn"`) || strings.Contains(request.Messages[1].Content, "[EVIDENCE 2]") {
						t.Fatalf("repair omitted saved answer or included completed peers: %+v", request)
					}
					return providers.Completion{}, interrupted
				}
				rows := []map[string]any{spanDecision(1, "When does harbor 28 open?", "dawn")}
				rows[0]["examples"] = append(rows[0]["examples"].([]map[string]any), map[string]any{"prompt": "When does harbor 28 close?", "answer_quote": "midnight", "target_id": 0})
				for i := 1; i < 5; i++ {
					rows = append(rows, spanDecision(i+1, fmt.Sprintf("When does harbor %d open?", 28+i), original.Units[original.Next+i].Targets[0]))
				}
				data, err := json.Marshal(map[string]any{"decisions": rows})
				return providers.Completion{Content: string(data), PromptTokens: 11, CompletionTokens: 4}, err
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, interrupted) || calls != 2 {
				t.Fatalf("partial recovery did not reach interruption: calls=%d err=%v", calls, err)
			}
			reopen(t, &input)
			pending := savedState(t, input)
			if pending.Next != original.Next || !slices.Equal(pending.Records, original.Records) || len(pending.GenerationDecisions) != 5 || pending.GenerationDecisions[0].Complete || len(pending.GenerationDecisions[0].Records) != 1 || pending.GenerationDecisions[0].Records[0].Target != "dawn" {
				t.Fatalf("lost partial candidate or committed too soon: %+v", pending)
			}
			for _, peer := range pending.GenerationDecisions[1:] {
				if !peer.Complete || len(peer.Records) != 1 {
					t.Fatalf("lost valid passage peer: %+v", peer)
				}
			}
			stop := errors.New("next batch")
			calls = 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				if calls == 2 {
					return providers.Completion{}, stop
				}
				if request.Seed != 3409 || !strings.Contains(request.Messages[1].Content, `answer="dawn"`) || strings.Contains(request.Messages[1].Content, "[EVIDENCE 2]") {
					t.Fatalf("reloaded repair lost saved candidate: %+v", request)
				}
				response := `{"decisions":[{"evidence_id":1,"examples":[],"reason":"none"}]}`
				if finish == "new candidate" {
					response = `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does harbor 28 close?","answer_quote":"dusk","target_id":0}],"reason":"none"}]}`
				}
				return providers.Completion{Content: response, PromptTokens: 5, CompletionTokens: 2}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 2 {
				t.Fatalf("repair failed to commit batch: calls=%d err=%v", calls, err)
			}
			reopen(t, &input)
			state := savedState(t, input)
			want := slices.Clone(original.Records)
			for i, decision := range pending.GenerationDecisions {
				want = append(want, decision.Records...)
				if i == 0 && finish == "new candidate" {
					r := training.Record{Prompt: "When does harbor 28 close?", Target: "dusk", Evidence: unit.Evidence, SourceDocument: "source", FactID: training.FactID(unit.Evidence, "dusk")}
					want = append(want, r)
				}
			}
			if state.Next != 30 || !slices.Equal(state.Records, want) || len(state.Result.Exclusions) != 0 || state.Result.PromptTokens != 117 || state.Result.CompletionTokens != 53 {
				t.Fatalf("repair replaced saved list, duplicated peers, or lost usage: %+v", state)
			}
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				if !strings.Contains(request.Messages[1].Content, "Harbor 33 opens at dawn.") || strings.Contains(request.Messages[1].Content, "TARGET 1: Harbor 28") {
					t.Fatalf("replay regenerated committed batch: %+v", request)
				}
				return providers.Completion{}, stop
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) {
				t.Fatal(err)
			}
			if replay := savedState(t, input); !slices.Equal(replay.Records, want) || replay.Next != 30 {
				t.Fatal("replay changed committed examples")
			}
		})
	}
}

func TestGenerationDuplicateRecovery(t *testing.T) {
	for _, name := range []string{"older than exclusion window", "recent prompt", "same batch"} {
		t.Run(name, func(t *testing.T) {
			input, original := generationInput(t)
			duplicate := "  WHEN does  harbor 0 open?  "
			switch name {
			case "recent prompt":
				duplicate = original.Records[27].Prompt
			case "same batch":
				duplicate = "When does harbor 28 open?"
			}
			response := func(reject bool) string {
				var rows []map[string]any
				for i := range 5 {
					prompt := fmt.Sprintf("When does harbor %d open?", i+28)
					if reject && i == 2 {
						prompt = duplicate
					}
					rows = append(rows, spanDecision(i+1, prompt, fmt.Sprintf("Harbor %d opens at dawn.", i+28)))
				}
				data, err := json.Marshal(map[string]any{"decisions": rows})
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			calls := 0
			interrupted := errors.New("interrupted correction request")
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				if calls == 2 {
					return providers.Completion{}, interrupted
				}
				if request.Seed != 3407 {
					t.Fatal("fresh batch changed seed")
				}
				return providers.Completion{Content: response(true), PromptTokens: 13, CompletionTokens: 5}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, interrupted) || calls != 2 {
				t.Fatalf("duplicate correction did not preserve provider failure: calls=%d err=%v", calls, err)
			}
			reopen(t, &input)
			state := savedState(t, input)
			if state.Next != 25 || state.GenerationAttempts != 2 || state.Generated || !reflect.DeepEqual(state.Records, original.Records) || state.Result.PromptTokens != 114 || state.Result.CompletionTokens != 52 {
				t.Fatalf("duplicate changed committed records or lost rejection usage: next=%d records=%d usage=%+v", state.Next, len(state.Records), state.Result)
			}
			stop := errors.New("stop before next batch")
			calls = 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				if calls == 2 {
					if request.Seed != 3407 || strings.Contains(request.Messages[1].Content, "rejected duplicate") {
						t.Fatal("rejection feedback leaked into the next batch")
					}
					return providers.Completion{}, stop
				}
				if request.Seed != 3409 || !strings.Contains(request.Messages[1].Content, strings.TrimSpace(duplicate)) || !strings.Contains(request.Messages[1].Content, "rejected duplicate") || !strings.Contains(request.Messages[1].Content, original.Units[27].Evidence) || strings.Contains(request.Messages[1].Content, "[EVIDENCE 2]") {
					t.Fatalf("retry lost feedback or pending evidence: %+v", request)
				}
				return providers.Completion{Content: `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does harbor 30 open?","answer_quote":"Harbor 30 opens at dawn.","target_id":0}],"reason":"none"}]}`, PromptTokens: 17, CompletionTokens: 3}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 2 {
				t.Fatalf("recovery did not commit exactly one batch: calls=%d err=%v", calls, err)
			}
			reopen(t, &input)
			state = savedState(t, input)
			if state.Next != 30 || len(state.Records) != 33 || state.Generated || state.EvaluationNext != 0 || !reflect.DeepEqual(state.Records[:28], original.Records) || state.Result.PromptTokens != 131 || state.Result.CompletionTokens != 55 {
				t.Fatalf("recovery changed saved progress or usage: next=%d records=%d usage=%+v", state.Next, len(state.Records), state.Result)
			}
			for i, r := range state.Records[28:] {
				evidence := original.Units[25+i].Evidence
				if r.Target != evidence || r.Evidence != evidence || r.FactID != training.FactID(evidence, evidence) || r.SourceDocument != "source" || r.UserApproved || r.ValidationPrompt || r.IndependentPrompt {
					t.Fatalf("recovery changed provenance or split: %+v", r)
				}
			}
			if err := training.ValidateRecords(state.Records); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGenerationResponseRecovery(t *testing.T) {
	for _, tc := range []struct{ name, feedback string }{
		{"repeated evidence", "invalid or repeated evidence ID"},
		{"zero evidence", "invalid or repeated evidence ID"},
		{"out of range evidence", "invalid or repeated evidence ID"},
		{"empty quote", "exact source span"},
		{"positive target", "exact source span"},
		{"wrong count", "one decision for every source unit"},
		{"malformed", "required decisions object"},
		{"unknown field", "required decisions object"},
		{"trailing", "required decisions object"},
		{"invalid question", "question mark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, original := generationInput(t)
			var rows []map[string]any
			for i := range 5 {
				rows = append(rows, spanDecision(i+1, fmt.Sprintf("When does harbor %d open?", i+28), fmt.Sprintf("Harbor %d opens at dawn.", i+28)))
			}
			valid, err := json.Marshal(map[string]any{"decisions": rows})
			if err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "repeated evidence":
				rows[2]["evidence_id"] = 1
			case "zero evidence":
				rows[2]["evidence_id"] = 0
			case "out of range evidence":
				rows[2]["evidence_id"] = 6
			case "empty quote":
				rows[2]["examples"].([]map[string]any)[0]["answer_quote"] = ""
			case "positive target":
				rows[2]["examples"].([]map[string]any)[0]["target_id"] = 2
			case "wrong count":
				rows = rows[:4]
			case "unknown field":
				rows[2]["answer"] = "invented target"
			case "invalid question":
				rows[2]["examples"].([]map[string]any)[0]["prompt"] = "Describe the harbor"
			}
			invalid, err := json.Marshal(map[string]any{"decisions": rows})
			if err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "malformed":
				invalid = []byte(`{`)
			case "trailing":
				invalid = append(invalid, []byte(`{}`)...)
			}
			attempts := 0
			interrupted := errors.New("interrupted correction request")
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				attempts++
				if attempts == 2 {
					return providers.Completion{}, interrupted
				}
				return providers.Completion{Content: string(invalid), PromptTokens: 12, CompletionTokens: 7}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, interrupted) || attempts != 2 {
				t.Fatalf("invalid response did not reach correction: attempts=%d err=%v", attempts, err)
			}
			reopen(t, &input)
			state := savedState(t, input)
			if state.GenerationRejections != 1 || state.GenerationAttempts != 2 || state.Next != 25 || state.Generated || !reflect.DeepEqual(state.Records, original.Records) || state.Result.PromptTokens != 113 || state.Result.CompletionTokens != 54 {
				t.Fatalf("rejection lost recovery state or changed committed progress: next=%d rejections=%d usage=%+v", state.Next, state.GenerationRejections, state.Result)
			}
			stop := errors.New("stop before next batch")
			calls := 0
			input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
				calls++
				if calls == 2 {
					if request.Seed != 3407 || strings.Contains(request.Messages[1].Content, tc.feedback) {
						t.Fatal("rejection feedback leaked into the next batch")
					}
					return providers.Completion{}, stop
				}
				if request.Seed != 3409 || !strings.Contains(request.Messages[1].Content, tc.feedback) {
					t.Fatalf("repair lost rejection feedback: %+v", request)
				}
				if tc.name == "invalid question" || tc.name == "empty quote" || tc.name == "positive target" {
					return providers.Completion{Content: `{"decisions":[{"evidence_id":1,"examples":[{"prompt":"When does harbor 30 open?","answer_quote":"Harbor 30 opens at dawn.","target_id":0}],"reason":"none"}]}`, PromptTokens: 17, CompletionTokens: 3}, nil
				}
				return providers.Completion{Content: string(valid), PromptTokens: 17, CompletionTokens: 3}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 2 {
				t.Fatalf("corrected retry did not reach the next batch: calls=%d err=%v", calls, err)
			}
			reopen(t, &input)
			state = savedState(t, input)
			if state.Next != 30 || len(state.Records) != 33 || state.GenerationRejections != 0 || !reflect.DeepEqual(state.Records[:28], original.Records) || state.Result.PromptTokens != 130 || state.Result.CompletionTokens != 57 {
				t.Fatalf("recovery changed committed progress or usage: next=%d records=%d usage=%+v", state.Next, len(state.Records), state.Result)
			}
			for i, record := range state.Records[28:] {
				evidence := original.Units[25+i].Evidence
				if record.Evidence != evidence || record.Target != evidence || record.FactID != training.FactID(evidence, evidence) || record.SourceDocument != "source" {
					t.Fatalf("recovery changed evidence or target: %+v", record)
				}
			}
		})
	}
}

func TestGenerationPersistentDuplicates(t *testing.T) {
	input, original := generationInput(t)
	calls := 0
	stop := errors.New("stop at the next source batch")
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		if calls == 3 {
			calls++
			if request.Seed != 3407 || !strings.Contains(request.Messages[1].Content, "Harbor 33 opens at dawn.") {
				t.Fatal("generation did not advance to the next batch with a fresh request budget")
			}
			return providers.Completion{}, stop
		}
		if request.Seed != uint32(3407+calls) {
			t.Fatalf("attempt %d seed = %d", calls, request.Seed)
		}
		calls++
		if calls > 1 {
			if strings.Contains(request.Messages[1].Content, "[EVIDENCE 2]") {
				t.Fatal("repair regenerated staged peers")
			}
			if calls == 2 {
				return providers.Completion{Content: `{"decisions":[{"reason":"none","examples":[{"prompt":"Second question?","answer_quote":"Harbor 28 opens at dawn.","target_id":0}],"evidence_id":1}]}`, PromptTokens: 7, CompletionTokens: 2}, nil
			}
			if !strings.Contains(request.Messages[1].Content, `repeats an existing prompt: "Second question?"`) {
				t.Fatal("repair accepted a duplicate of a staged candidate")
			}
			return providers.Completion{Content: `{"decisions":[{"reason":"none","examples":[{"prompt":"When does harbor 0 open?","answer_quote":"Harbor 28 opens at dawn.","target_id":0}],"evidence_id":1}]}`, PromptTokens: 7, CompletionTokens: 2}, nil
		}
		var rows []map[string]any
		for i, prompt := range []string{"When does harbor 0 open?", "Second question?", "Third question?", "Fourth question?", "Fifth question?"} {
			rows = append(rows, spanDecision(i+1, prompt, fmt.Sprintf("Harbor %d opens at dawn.", 28+i)))
		}
		data, err := json.Marshal(map[string]any{"decisions": rows})
		return providers.Completion{Content: string(data), PromptTokens: 7, CompletionTokens: 2}, err
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 4 {
		t.Fatalf("source-specific rejection blocked the next batch: calls=%d err=%v", calls, err)
	}
	state := savedState(t, input)
	if state.GenerationAttempts != 1 || state.GenerationRejections != 0 || state.Next != 30 || len(state.Records) != len(original.Records)+4 || !reflect.DeepEqual(state.Records[:len(original.Records)], original.Records) || state.Generated {
		t.Fatalf("failed source changed retained records or blocked progress: %+v", state)
	}
	if len(state.Result.Exclusions) != 1 || state.Result.Exclusions[0].Reference != "harbor-28" || !strings.Contains(state.Result.Exclusions[0].Reason, "generation failed after 3 batch requests") || !strings.Contains(state.Result.Exclusions[0].Reason, "repeats an existing prompt") || len(state.Result.Warnings) != 1 {
		t.Fatalf("source generation failure was not reported: %+v", state.Result)
	}
	for i, record := range state.Records[len(original.Records):] {
		if record.Target != fmt.Sprintf("Harbor %d opens at dawn.", 29+i) {
			t.Fatalf("valid peer changed: %+v", record)
		}
	}
	if state.Result.PromptTokens != 122 || state.Result.CompletionTokens != 53 {
		t.Fatalf("rejection usage was not counted exactly once: %+v", state.Result)
	}
	reopen(t, &input)
	if loaded, _, err := checkpoint.Load(t.Context(), input, false); err != nil || !reflect.DeepEqual(loaded, state) {
		t.Fatalf("reported partial progress did not survive reopening: %+v %v", loaded, err)
	}
}

// Only a last reply that fails validation leaves passages out. Passages that
// resolved keep their records, and a request with an unknown outcome stops the run.
func TestGenerationInvalidLastReply(t *testing.T) {
	malformed := providers.Completion{Content: `{`, PromptTokens: 5, CompletionTokens: 1}
	nextBatch := func(t *testing.T, request providers.CompletionRequest) {
		t.Helper()
		if request.Seed != 3407 || !strings.Contains(request.Messages[1].Content, "Harbor 33 opens at dawn.") {
			t.Fatal("generation did not advance to the next batch")
		}
	}
	stop := errors.New("stop at the next source batch")

	t.Run("invalid replies exclude the batch", func(t *testing.T) {
		input, original := generationInput(t)
		calls := 0
		input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
			calls++
			if calls == 4 {
				nextBatch(t, request)
				return providers.Completion{}, stop
			}
			return malformed, nil
		})
		if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 4 {
			t.Fatalf("invalid replies stopped the run: calls=%d err=%v", calls, err)
		}
		state := savedState(t, input)
		if state.Next != 30 || !reflect.DeepEqual(state.Records, original.Records) || state.GenerationAttempts != 1 || state.GenerationLastRejected {
			t.Fatalf("batch was not closed: next=%d records=%d attempts=%d", state.Next, len(state.Records), state.GenerationAttempts)
		}
		excluded := state.Result.Exclusions[len(original.Result.Exclusions):]
		if len(excluded) != 5 || len(state.Result.Warnings) != len(original.Result.Warnings)+5 {
			t.Fatalf("excluded passages were not reported: %+v", state.Result)
		}
		for i, exclusion := range excluded {
			if exclusion.Reference != fmt.Sprintf("harbor-%d", 28+i) || !strings.Contains(exclusion.Reason, "invalid model reply: return only the required decisions object") {
				t.Fatalf("exclusion %d: %+v", i, exclusion)
			}
		}
	})

	t.Run("resolved passages keep their records", func(t *testing.T) {
		input, original := generationInput(t)
		calls := 0
		input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
			calls++
			switch calls {
			case 1:
				// The second passage repeats a prompt already in the dataset; the others resolve.
				var rows []map[string]any
				for i, prompt := range []string{"First question?", original.Records[0].Prompt, "Third question?", "Fourth question?", "Fifth question?"} {
					rows = append(rows, spanDecision(i+1, prompt, fmt.Sprintf("Harbor %d opens at dawn.", 28+i)))
				}
				data, err := json.Marshal(map[string]any{"decisions": rows})
				return providers.Completion{Content: string(data), PromptTokens: 7, CompletionTokens: 2}, err
			case 4:
				nextBatch(t, request)
				return providers.Completion{}, stop
			}
			return malformed, nil
		})
		if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 4 {
			t.Fatalf("invalid repairs stopped the run: calls=%d err=%v", calls, err)
		}
		state := savedState(t, input)
		if state.Next != 30 || len(state.Records) != len(original.Records)+4 || !reflect.DeepEqual(state.Records[:len(original.Records)], original.Records) {
			t.Fatalf("resolved passages lost their records: next=%d records=%d", state.Next, len(state.Records))
		}
		excluded := state.Result.Exclusions[len(original.Result.Exclusions):]
		if len(excluded) != 1 || excluded[0].Reference != "harbor-29" || !strings.Contains(excluded[0].Reason, "invalid model reply") {
			t.Fatalf("only the unresolved passage may be excluded: %+v", excluded)
		}
	})

	t.Run("interrupted last request stops the run", func(t *testing.T) {
		input, original := generationInput(t)
		interrupted := errors.New("interrupted last request")
		calls := 0
		input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
			calls++
			if calls == 3 {
				return providers.Completion{}, interrupted
			}
			return malformed, nil
		})
		if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, interrupted) || calls != 3 {
			t.Fatalf("interruption was not returned: calls=%d err=%v", calls, err)
		}
		reopen(t, &input)
		input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
			t.Fatal("request dispatched past the request budget")
			return providers.Completion{}, nil
		})
		_, err := (generate.Step{}).Execute(t.Context(), input)
		application, ok := errors.AsType[*temporal.ApplicationError](err)
		if !ok || !application.NonRetryable() || application.Type() != "preparation-request-budget-exhausted" {
			t.Fatalf("unknown outcome did not stop the run: %v", err)
		}
		state := savedState(t, input)
		if state.Next != 25 || len(state.Result.Exclusions) != len(original.Result.Exclusions) {
			t.Fatalf("passages were excluded after an unknown outcome: next=%d exclusions=%d", state.Next, len(state.Result.Exclusions))
		}
	})
}

func TestGenerationFailuresPreserveCheckpoint(t *testing.T) {
	providerError := errors.New("provider unavailable")
	for _, name := range []string{"provider", "canceled before call", "canceled duplicate", "canceled malformed", "canceled success", "write rejected", "write invalid", "write accepted"} {
		t.Run(name, func(t *testing.T) {
			input, pending := generationInput(t)
			pending.GenerationAttempts = 1
			pending.GenerationRejections, pending.GenerationRejectedPrompt = 1, pending.Records[0].Prompt
			for _, unit := range pending.Units[pending.Next : pending.Next+5] {
				pending.GenerationDecisions = append(pending.GenerationDecisions, checkpoint.GenerationDecision{Reference: unit.Reference})
			}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, pending); err != nil {
				t.Fatal(err)
			}
			before, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				calls++
				var rows []map[string]any
				for i := range 5 {
					rows = append(rows, spanDecision(i+1, fmt.Sprintf("When does harbor %d open?", i+28), fmt.Sprintf("Harbor %d opens at dawn.", i+28)))
				}
				switch name {
				case "provider":
					return providers.Completion{}, providerError
				case "write invalid":
					rows[2]["evidence_id"] = 1
				case "canceled duplicate", "write rejected":
					rows[0]["examples"].([]map[string]any)[0]["prompt"] = pending.Records[0].Prompt
				}
				data, err := json.Marshal(map[string]any{"decisions": rows})
				if err != nil {
					t.Fatal(err)
				}
				out := providers.Completion{Content: string(data), PromptTokens: 23, CompletionTokens: 9}
				if name == "canceled malformed" {
					out.Content = `{`
				}
				if strings.HasPrefix(name, "canceled") {
					cancel()
				}
				if strings.HasPrefix(name, "write") {
					input.Workspace.Close()
				}
				return out, nil
			})
			if name == "canceled before call" {
				cancel()
			}
			_, err = (generate.Step{}).Execute(ctx, input)
			if err == nil || calls > 1 {
				t.Fatalf("failure was hidden or retried internally: calls=%d err=%v", calls, err)
			}
			if name == "provider" && !errors.Is(err, providerError) || strings.HasPrefix(name, "canceled") && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost failure identity: %v", err)
			}
			if name == "canceled before call" && calls != 0 {
				t.Fatal("called provider with canceled context")
			}
			if strings.HasPrefix(name, "write") {
				if _, ok := errors.AsType[*os.PathError](err); !ok {
					t.Fatalf("checkpoint write failure must take precedence: %v", err)
				}
			}
			reopen(t, &input)
			after, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil {
				t.Fatal(err)
			}
			if calls == 0 && string(before) != string(after) {
				t.Fatal("cancellation before dispatch changed the checkpoint")
			}
			if calls == 1 {
				pending.GenerationAttempts++
			}
			if got := savedState(t, input); !reflect.DeepEqual(got, pending) {
				t.Fatalf("failure must preserve progress and consume only its reservation: got %+v; want %+v", got, pending)
			}
		})
	}
}

func TestGenerationRejectsInvalidCheckpoint(t *testing.T) {
	for _, name := range []string{"missing prompt", "missing count", "blank prompt", "reason without count", "blank reason", "already generated", "no pending passages", "evaluations started", "candidate count", "candidate reference", "candidate evidence", "candidate target", "candidate identity", "candidate owner", "candidate approval", "candidate duplicate", "candidate reason"} {
		t.Run(name, func(t *testing.T) {
			input, state := generationInput(t)
			state.GenerationAttempts = 1
			state.GenerationRejections, state.GenerationRejectedPrompt = 1, state.Records[0].Prompt
			if strings.HasPrefix(name, "candidate") {
				for _, unit := range state.Units[state.Next : state.Next+5] {
					state.GenerationDecisions = append(state.GenerationDecisions, checkpoint.GenerationDecision{Reference: unit.Reference})
				}
				unit := state.Units[state.Next]
				r := training.Record{Prompt: "When does harbor 28 open?", Evidence: unit.Evidence, Target: unit.Targets[0], SourceDocument: "source"}
				r.FactID = training.FactID(r.Evidence, r.Target)
				state.GenerationDecisions[0].Records = []training.Record{r}
				state.GenerationDecisions[0].Complete = true
			}
			switch name {
			case "missing prompt":
				state.GenerationRejectedPrompt = ""
			case "missing count":
				state.GenerationRejections = 0
			case "blank prompt":
				state.GenerationRejectedPrompt = " \t "
			case "reason without count":
				state.GenerationRejections, state.GenerationRejectedPrompt, state.GenerationRejectionReason = 0, "", "model returned an invalid target ID"
			case "blank reason":
				state.GenerationRejectedPrompt, state.GenerationRejectionReason = "", " \t "
			case "already generated":
				state.Generated = true
			case "no pending passages":
				state.Next = len(state.Units)
			case "evaluations started":
				state.EvaluationNext = 1
			case "candidate count":
				state.GenerationDecisions = state.GenerationDecisions[:4]
			case "candidate reference":
				state.GenerationDecisions[0].Reference = "other"
			case "candidate evidence":
				state.GenerationDecisions[0].Records[0].Evidence = "Other evidence."
			case "candidate target":
				state.GenerationDecisions[0].Records[0].Target = "Other target."
			case "candidate identity":
				state.GenerationDecisions[0].Records[0].FactID = "other"
			case "candidate owner":
				state.GenerationDecisions[0].Records[0].SourceDocument = "other"
			case "candidate approval":
				state.GenerationDecisions[0].Records[0].UserApproved = true
			case "candidate duplicate":
				state.GenerationDecisions[0].Records[0].Prompt = state.Records[0].Prompt
			case "candidate reason":
				state.GenerationDecisions[0].Reason = "cannot have both candidate and rejection"
			}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("invalid checkpoint reached the provider")
				return providers.Completion{}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); err == nil {
				t.Fatal("accepted inconsistent rejection feedback")
			}
		})
	}
}

func TestGenerationBatchLocalRecovery(t *testing.T) {
	input, original := generationInput(t)
	for i := 34; i < 43; i++ {
		evidence := fmt.Sprintf("Harbor %d opens at dawn.", i)
		original.Units = append(original.Units, preparation.SourceUnit{Reference: fmt.Sprintf("harbor-%d", i), Evidence: evidence, Targets: []string{evidence}})
	}
	if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, original); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop before held-out generation")
	calls := 0
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		if strings.Contains(request.Messages[1].Content, "Locked target:") {
			return providers.Completion{}, stop
		}
		batch, attempt := calls/2, calls%2
		calls++
		if request.Seed != uint32(3407+attempt) {
			t.Fatalf("batch-local seed: %d", request.Seed)
		}
		if attempt == 0 {
			return providers.Completion{Content: `{`, PromptTokens: 3, CompletionTokens: 2}, nil
		}
		var rows []map[string]any
		for i := range 5 {
			rows = append(rows, spanDecision(i+1, fmt.Sprintf("When does harbor %d open?", 28+batch*5+i), fmt.Sprintf("Harbor %d opens at dawn.", 28+batch*5+i)))
		}
		data, err := json.Marshal(map[string]any{"decisions": rows})
		return providers.Completion{Content: string(data), PromptTokens: 3, CompletionTokens: 2}, err
	})
	for range 2 {
		if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 6 {
			t.Fatalf("three corrected batches must complete in one execution and not regenerate on replay: calls=%d err=%v", calls, err)
		}
		reopen(t, &input)
		state := savedState(t, input)
		if state.Next != 40 || len(state.Records) != 43 || state.GenerationAttempts != 0 || state.GenerationRejections != 0 || state.Result.PromptTokens != 119 || state.Result.CompletionTokens != 59 || !slices.Equal(state.Records[:28], original.Records) {
			t.Fatalf("batch-local recovery lost progress or usage: %+v", state)
		}
		for i, record := range state.Records[28:] {
			evidence := fmt.Sprintf("Harbor %d opens at dawn.", 28+i)
			if record.Evidence != evidence || record.Target != evidence || record.FactID != training.FactID(evidence, evidence) {
				t.Fatalf("repaired batch changed grounding: %+v", record)
			}
		}
	}
}

func TestGenerationReservationBoundary(t *testing.T) {
	for _, name := range []string{"reservation write failure", "uncertain last request"} {
		t.Run(name, func(t *testing.T) {
			input, state := generationInput(t)
			if name == "reservation write failure" {
				if os.Geteuid() == 0 {
					t.Skip("root can bypass directory write permissions")
				}
				if err := os.Chmod(input.Workspace.Name(), 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(input.Workspace.Name(), 0o700) })
			} else {
				state.GenerationAttempts = 3
				if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
					t.Fatal(err)
				}
			}
			before, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("completion dispatched without an available durable reservation")
				return providers.Completion{}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); err == nil {
				t.Fatal("reservation failure was ignored")
			}
			after, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil || string(after) != string(before) {
				t.Fatalf("reservation failure changed saved progress: %v", err)
			}
		})
	}
}

func TestOldCheckpointRequiresFreshPreparation(t *testing.T) {
	for _, step := range []preparation.StepHandler{extract.Step{}, generate.Step{}} {
		for _, version := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s-version-%d", step.Name(), version), func(t *testing.T) {
				input, state := generationInput(t)
				legacy := map[string]any{"fingerprint": state.Fingerprint, "passages": []string{"We read"}, "next": 60, "records": slices.Repeat(state.Records[:1], 63)}
				if version == 2 {
					state.Version = version
					data, err := json.Marshal(state)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(data, &legacy); err != nil {
						t.Fatal(err)
					}
				}
				data, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				if err := input.Workspace.WriteFile(checkpoint.File, data, 0o600); err != nil {
					t.Fatal(err)
				}
				input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
					t.Fatal("old checkpoint reached generation")
					return providers.Completion{}, nil
				})
				_, err = step.Execute(t.Context(), input)
				application, ok := errors.AsType[*temporal.ApplicationError](err)
				if !ok || !application.NonRetryable() || application.Type() != "preparation-checkpoint-version" {
					t.Fatalf("old checkpoint did not request fresh Preparation: %v", err)
				}
				after, err := input.Workspace.ReadFile(checkpoint.File)
				if err != nil || string(after) != string(data) {
					t.Fatalf("old checkpoint was rewritten: %v", err)
				}
			})
		}
	}
}

func TestEvaluationRecovery(t *testing.T) {
	input, original := evaluationInput(t)
	// A normalized duplicate from a DIFFERENT fact must also be rejected.
	duplicate := "  WHEN does  harbor 12 open?  "
	calls := 0
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		calls++
		if request.Seed != 3407 {
			t.Fatalf("first seed = %d", request.Seed)
		}
		if len(request.Messages) != 4 || request.Messages[2].Role != "assistant" || request.Messages[3].Role != "user" {
			t.Fatalf("evaluation must request a rewrite of the existing question: %+v", request.Messages)
		}
		var example map[string]string
		if err := json.Unmarshal([]byte(request.Messages[2].Content), &example); err != nil || example["record_1"] != original.Records[4].Prompt {
			t.Fatalf("initial rewrite lost the original question: %+v, %v", example, err)
		}
		return providers.Completion{Content: `{"record_1":"` + duplicate + `"}`, PromptTokens: 13, CompletionTokens: 5}, nil
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); err == nil || strings.Contains(err.Error(), "harbor") || calls != 1 {
		t.Fatalf("duplicate must fail once without logging content: calls=%d err=%v", calls, err)
	}
	reopen(t, &input)
	state := savedState(t, input)
	if !reflect.DeepEqual(state.Records, original.Records) || state.Next != 17 || state.EvaluationNext != 4 || state.Generated {
		t.Fatalf("rejection changed committed progress: %+v", state)
	}
	if state.Result.PromptTokens != 114 || state.Result.CompletionTokens != 52 {
		t.Fatalf("rejected completion usage was not persisted once: %+v", state.Result)
	}
	stop := errors.New("stop at next question")
	calls = 0
	input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
		calls++
		if len(request.Messages) != 4 {
			t.Fatalf("missing rewrite conversation: %+v", request.Messages)
		}
		var example map[string]string
		if err := json.Unmarshal([]byte(request.Messages[2].Content), &example); err != nil {
			t.Fatal(err)
		}
		if calls == 2 {
			if request.Seed != 3407 || example["record_1"] != original.Records[5].Prompt {
				t.Fatal("rejection feedback leaked into the next question")
			}
			return providers.Completion{}, stop
		}
		if request.Seed != 3408 || example["record_1"] != strings.TrimSpace(duplicate) || !strings.Contains(request.Messages[1].Content, original.Records[4].Evidence) || !strings.Contains(request.Messages[3].Content, "original evidence and locked target") {
			t.Fatalf("reloaded retry did not change seed/feedback while preserving the fact: %+v", request)
		}
		return providers.Completion{Content: `{"record_1":"At which hour does harbor 4 open?"}`, PromptTokens: 17, CompletionTokens: 3}, nil
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); !errors.Is(err, stop) || calls != 2 {
		t.Fatalf("recovery: calls=%d err=%v", calls, err)
	}
	reopen(t, &input)
	state = savedState(t, input)
	if len(state.Records) != 25 || state.EvaluationNext != 5 || !reflect.DeepEqual(state.Records[:24], original.Records) {
		t.Fatalf("accepted record did not advance exactly once: %+v", state)
	}
	want := original.Records[4]
	want.Prompt, want.ValidationPrompt, want.UserApproved = "At which hour does harbor 4 open?", true, false
	if state.Records[24] != want || state.Result.PromptTokens != 131 || state.Result.CompletionTokens != 55 {
		t.Fatalf("recovery changed provenance/flags or usage: record=%+v usage=%+v", state.Records[24], state.Result)
	}
	if state.EvaluationRejections != 0 || state.EvaluationRejectedPrompt != "" {
		t.Fatal("accepted question retained rejection feedback")
	}
}

func TestEvaluationPersistentDuplicates(t *testing.T) {
	input, original := evaluationInput(t)
	for attempt := range 3 {
		calls := 0
		input.Preparation.Completer = completionFunc(func(_ context.Context, request providers.CompletionRequest) (providers.Completion, error) {
			calls++
			if request.Seed != uint32(3407+attempt) {
				t.Fatalf("attempt %d seed = %d", attempt, request.Seed)
			}
			// Exercise both the selected fact and a different saved fact.
			return providers.Completion{Content: fmt.Sprintf(`{"record_1":%q}`, original.Records[4+attempt].Prompt), PromptTokens: 7 + attempt, CompletionTokens: 2}, nil
		})
		if _, err := (generate.Step{}).Execute(t.Context(), input); err == nil || calls != 1 {
			t.Fatalf("attempt %d: calls=%d err=%v", attempt, calls, err)
		}
		reopen(t, &input)
		state := savedState(t, input)
		if state.EvaluationRejections != uint32(attempt+1) || state.EvaluationRejectedPrompt != original.Records[4+attempt].Prompt || state.EvaluationNext != 4 || !reflect.DeepEqual(state.Records, original.Records) || state.Generated {
			t.Fatalf("persistent rejection changed progress or lost feedback: %+v", state)
		}
	}
	state := savedState(t, input)
	if state.Result.PromptTokens != 125 || state.Result.CompletionTokens != 53 {
		t.Fatalf("repeated rejection usage = %+v", state.Result)
	}
}

func TestEvaluationFailuresPreserveCheckpoint(t *testing.T) {
	providerError := errors.New("provider unavailable")
	for _, name := range []string{"provider", "malformed", "unknown field", "invalid question", "canceled before call", "canceled duplicate", "canceled success", "write rejected", "write accepted"} {
		t.Run(name, func(t *testing.T) {
			input, pending := evaluationInput(t)
			pending.EvaluationRejections, pending.EvaluationRejectedPrompt = 2, pending.Records[4].Prompt
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, pending); err != nil {
				t.Fatal(err)
			}
			before, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				calls++
				out := providers.Completion{Content: `{"record_1":"Which hour marks harbor 4 opening?"}`, PromptTokens: 23, CompletionTokens: 9}
				switch name {
				case "provider":
					return out, providerError
				case "malformed":
					out.Content = `{`
				case "unknown field":
					out.Content = `{"record_1":"When does harbor 4 open?","extra":true}`
				case "invalid question":
					out.Content = `{"record_1":"Describe opening hours"}`
				case "canceled duplicate", "write rejected":
					out.Content = `{"record_1":"When does harbor 4 open?"}`
				}
				if strings.HasPrefix(name, "canceled") {
					cancel()
				}
				if strings.HasPrefix(name, "write") {
					input.Workspace.Close()
				}
				return out, nil
			})
			if name == "canceled before call" {
				cancel()
			}
			_, err = (generate.Step{}).Execute(ctx, input)
			if err == nil || calls > 1 {
				t.Fatalf("failure was hidden or retried internally: calls=%d err=%v", calls, err)
			}
			if name == "provider" && !errors.Is(err, providerError) || strings.HasPrefix(name, "canceled") && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost failure identity: %v", err)
			}
			if name == "canceled before call" && calls != 0 {
				t.Fatal("called provider with canceled context")
			}
			if strings.HasPrefix(name, "write") {
				if _, ok := errors.AsType[*os.PathError](err); !ok {
					t.Fatalf("checkpoint write failure must take precedence: %v", err)
				}
			}
			reopen(t, &input)
			after, err := input.Workspace.ReadFile(checkpoint.File)
			if err != nil || string(before) != string(after) {
				t.Fatalf("failure changed durable progress: %v", err)
			}
		})
	}
}

func TestEvaluationRejectsInvalidCheckpoint(t *testing.T) {
	for _, name := range []string{"missing prompt", "missing count", "blank prompt", "blank prompt without count", "already generated", "incomplete training generation"} {
		t.Run(name, func(t *testing.T) {
			input, state := evaluationInput(t)
			state.EvaluationRejections, state.EvaluationRejectedPrompt = 1, state.Records[4].Prompt
			switch name {
			case "missing prompt":
				state.EvaluationRejectedPrompt = ""
			case "missing count":
				state.EvaluationRejections = 0
			case "blank prompt":
				state.EvaluationRejectedPrompt = " \t "
			case "blank prompt without count":
				state.EvaluationRejections, state.EvaluationRejectedPrompt = 0, " \t "
			case "already generated":
				state.Generated = true
			case "incomplete training generation":
				state.Next--
			}
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				t.Fatal(err)
			}
			input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
				t.Fatal("invalid checkpoint reached the provider")
				return providers.Completion{}, nil
			})
			if _, err := (generate.Step{}).Execute(t.Context(), input); err == nil {
				t.Fatal("accepted inconsistent rejection metadata")
			}
		})
	}
}

func TestEvaluationCompletesAndReplays(t *testing.T) {
	input, original := evaluationInput(t)
	calls := 0
	input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
		calls++
		return providers.Completion{Content: fmt.Sprintf(`{"record_1":"What is the opening time for harbor question %d?"}`, calls), PromptTokens: 3, CompletionTokens: 2}, nil
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	reopen(t, &input)
	state := savedState(t, input)
	// Twenty training records need 8 validation questions (4 saved) and 16 test questions.
	if calls != 20 || len(state.Records) != 44 || !state.Generated || state.Result.PromptTokens != 161 || state.Result.CompletionTokens != 87 || !reflect.DeepEqual(state.Records[:24], original.Records) {
		t.Fatalf("completion changed counts/usage/saved examples: %+v", state)
	}
	for i, r := range state.Records[24:] {
		// Validation continues with facts 4-7; test questions start again at fact 0.
		validation, index := i < 4, i-4
		if validation {
			index = i + 4
		}
		fact := original.Records[index]
		if r.Evidence != fact.Evidence || r.Target != fact.Target || r.FactID != fact.FactID || r.SourceDocument != fact.SourceDocument || r.UserApproved || r.ValidationPrompt != validation || r.IndependentPrompt == validation {
			t.Fatalf("evaluation %d changed fact or split: %+v", i, r)
		}
	}
	input.Preparation.Completer = completionFunc(func(context.Context, providers.CompletionRequest) (providers.Completion, error) {
		t.Fatal("completed generation called the provider on replay")
		return providers.Completion{}, nil
	})
	if _, err := (generate.Step{}).Execute(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if got := savedState(t, input); !reflect.DeepEqual(got, state) {
		t.Fatal("completed replay changed durable state")
	}
}
