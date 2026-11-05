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
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
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
	passages := []string{"The harbor opens at dawn.", "The ferry leaves at noon."}
	valid := `{"records":[{"prompt":" When does the ferry leave? ","evidence_id":2,"target_id":1},{"prompt":"When does the harbor open?","evidence_id":1,"target_id":1}]}`
	records, usage, err := generate.Run(t.Context(), completer{content: valid}, request, passages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Prompt != "When does the ferry leave?" || records[0].Target != passages[1] || records[0].Evidence != passages[1] || records[0].FactID == "" || records[1].Target != passages[0] || records[0].FactID == records[1].FactID {
		t.Fatalf("incorrect grounded records: %+v", records)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 {
		t.Fatalf("lost usage: %+v", usage)
	}
	for name, response := range map[string]string{
		"malformed":         `{`,
		"trailing":          valid + `{}`,
		"wrong count":       `{"records":[]}`,
		"repeated evidence": `{"records":[{"prompt":"First question?","evidence_id":1,"target_id":1},{"prompt":"Second question?","evidence_id":1,"target_id":1}]}`,
		"unknown field":     `{"records":[],"extra":true}`,
		"bad evidence":      `{"records":[{"prompt":"First question?","evidence_id":0,"target_id":1},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
		"bad target":        `{"records":[{"prompt":"First question?","evidence_id":1,"target_id":2},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
		"not question":      `{"records":[{"prompt":"Describe harbor","evidence_id":1,"target_id":1},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, _, err := generate.Run(t.Context(), completer{content: response}, request, passages, nil); err == nil || len(got) != 0 {
				t.Fatalf("accepted invalid response: %+v, %v", got, err)
			}
		})
	}
	if _, _, err := generate.Run(t.Context(), completer{content: valid}, request, passages, records); err == nil {
		t.Fatal("accepted repeated previous prompts")
	}
	want := errors.New("provider unavailable")
	if _, usage, err := generate.Run(t.Context(), completer{err: want}, request, passages, nil); !errors.Is(err, want) || usage.PromptTokens != 12 {
		t.Fatalf("provider failure/usage: %+v, %v", usage, err)
	}
}

type completionFunc func(context.Context, providers.CompletionRequest) (providers.Completion, error)

func (f completionFunc) Complete(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
	return f(ctx, request)
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
			state.Passages = append(state.Passages, evidence)
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
	for _, name := range []string{"missing prompt", "missing count", "blank prompt", "blank prompt without count", "already generated", "incomplete training generation", "all evaluations done", "verification started", "insufficient training records"} {
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
			case "all evaluations done":
				state.EvaluationNext = 30
			case "verification started":
				state.Verified = 1
			case "insufficient training records":
				state.Records = state.Records[:19]
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
	if calls != 26 || len(state.Records) != 50 || state.EvaluationNext != 30 || !state.Generated || state.Result.PromptTokens != 179 || state.Result.CompletionTokens != 99 || !reflect.DeepEqual(state.Records[:24], original.Records) {
		t.Fatalf("completion changed counts/usage/saved examples: %+v", state)
	}
	for i, r := range state.Records[24:] {
		index := i + 4
		fact := original.Records[index%20]
		if r.Evidence != fact.Evidence || r.Target != fact.Target || r.FactID != fact.FactID || r.SourceDocument != fact.SourceDocument || r.UserApproved || r.ValidationPrompt != (index < 10) || r.IndependentPrompt != (index >= 10) {
			t.Fatalf("evaluation %d changed fact or split: %+v", index, r)
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
