package generate

import (
	"context"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step generates examples and held-out questions with durable batch checkpoints.
type Step struct{}

// Name identifies the generation step within the preparation stage.
func (Step) Name() string { return "generate" }

// Execute resumes generation without repeating committed batches. It never
// verifies records or publishes a dataset; verification owns those decisions.
func (Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	state, _, err := checkpoint.Load(ctx, input, false)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	p := input.Preparation
	if p.Completer == nil {
		return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("%w: preparation completer is required", model.ErrInvalidArgument)
	}
	report := func(progress tmprl.Progress) {
		if input.Publish != nil {
			input.Publish(progress)
		}
	}
	report(tmprl.Progress{Completed: state.Next, Total: len(state.Passages), Message: "Generating training examples"})
	for state.Next < len(state.Passages) {
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		end := min(state.Next+5, len(state.Passages))
		records, usage, err := Run(ctx, p.Completer, p.Input.Request, state.Passages[state.Next:end], state.Records)
		if err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		for i := range records {
			records[i].SourceDocument = p.Source.ID
		}
		state.Records = append(state.Records, records...)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.Next = end
		if err := dataset.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		report(tmprl.Progress{Completed: state.Next, Total: len(state.Passages), Message: "Generating training examples"})
	}
	training := slices.DeleteFunc(slices.Clone(state.Records), func(r model.Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
	if p.Input.Kind == model.PreparationFull && p.Input.Request.Goal == model.GoalQuestionAnswer && len(training) >= 20 {
		report(tmprl.Progress{Completed: state.EvaluationNext, Total: 30, Message: "Generating evaluation questions"})
		for state.EvaluationNext < 30 {
			if err := ctx.Err(); err != nil {
				return tmprl.StepResult[model.Artifacts]{}, err
			}
			index := state.EvaluationNext
			r, usage, err := EvaluationRecord(ctx, p.Completer, p.Input.Request, training[index%20], state.Records)
			if err != nil {
				return tmprl.StepResult[model.Artifacts]{}, err
			}
			r.SourceDocument = p.Source.ID
			r.UserApproved = false
			r.ValidationPrompt = index < 10
			r.IndependentPrompt = index >= 10
			state.Records = append(state.Records, r)
			state.Result.PromptTokens += usage.PromptTokens
			state.Result.CompletionTokens += usage.CompletionTokens
			state.EvaluationNext++
			if err := dataset.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				return tmprl.StepResult[model.Artifacts]{}, err
			}
			report(tmprl.Progress{Completed: state.EvaluationNext, Total: 30, Message: "Generating evaluation questions"})
		}
	}
	if !state.Generated {
		state.Generated = true
		if err := dataset.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
	}
	return tmprl.StepResult[model.Artifacts]{Artifacts: input.Request.Artifacts}, nil
}
