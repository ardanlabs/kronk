package generate

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step generates examples and held-out questions with durable batch checkpoints.
type Step struct{}

// Name identifies the generation step within the preparation stage.
func (Step) Name() string { return "generate" }

// Execute resumes generation without repeating committed batches. It never
// verifies records or publishes a dataset; verification owns those decisions.
func (Step) Execute(ctx context.Context, input preparation.StepContext) (tmprl.StepResult[preparation.Artifacts], error) {
	state, _, err := checkpoint.Load(ctx, input, false)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	p := input.Preparation
	if p.Completer == nil {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preparation completer is required", preparation.ErrInvalidArgument)
	}
	if input.Review != nil && state.ReviewID == "" {
		if input.ReviewID == "" || len(input.Review.Records) == 0 || len(input.Review.Records) > 5 || len(state.Records) != 0 || state.Next != 0 || state.Generated {
			return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: review must precede full generation", preparation.ErrInvalidArgument)
		}
		if err := training.ValidateReviewedRecords(input.Review.Records); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: %s", preparation.ErrInvalidArgument, err)
		}
		for _, r := range input.Review.Records {
			if !r.UserApproved || r.SourceDocument != p.Source.ID || !slices.Contains(state.Passages, r.Evidence) {
				return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: reviewed example must belong to the extracted passages", preparation.ErrInvalidArgument)
			}
			if err := ValidateRecord(p.Input.Request.Goal, r); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: %s", preparation.ErrInvalidArgument, err)
			}
		}
		state.Records = slices.Clone(input.Review.Records)
		state.ReviewID, state.ReviewedCount = input.ReviewID, len(state.Records)
		state.Passages = slices.DeleteFunc(state.Passages, func(passage string) bool {
			return slices.ContainsFunc(state.Records, func(r training.Record) bool { return r.Evidence == passage })
		})
		if input.Artifacts.Preview != nil {
			state.Result.PromptTokens += input.Artifacts.Preview.PromptTokens
			state.Result.CompletionTokens += input.Artifacts.Preview.CompletionTokens
		}
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}
	report := func(progress tmprl.Progress) {
		if input.Publish != nil {
			input.Publish(progress)
		}
	}
	report(tmprl.Progress{Completed: state.Next, Total: len(state.Passages), Message: "Generating training examples"})
	for state.Next < len(state.Passages) {
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		end := min(state.Next+5, len(state.Passages))
		records, usage, err := Run(ctx, p.Completer, p.Input.Request, state.Passages[state.Next:end], state.Records)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		for i := range records {
			records[i].SourceDocument = p.Source.ID
		}
		state.Records = append(state.Records, records...)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.Next = end
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		report(tmprl.Progress{Completed: state.Next, Total: len(state.Passages), Message: "Generating training examples"})
	}
	training := slices.DeleteFunc(slices.Clone(state.Records), func(r training.Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
	if p.Input.Kind == preparation.PreparationFull && p.Input.Request.Goal == preparation.GoalQuestionAnswer && len(training) >= 20 {
		report(tmprl.Progress{Completed: state.EvaluationNext, Total: 30, Message: "Generating evaluation questions"})
		for state.EvaluationNext < 30 {
			if err := ctx.Err(); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			index := state.EvaluationNext
			r, usage, err := EvaluationRecord(ctx, p.Completer, p.Input.Request, training[index%20], state.Records, state.EvaluationRejections, state.EvaluationRejectedPrompt)
			if canceled := ctx.Err(); canceled != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, canceled
			}
			if err != nil {
				if duplicate, ok := errors.AsType[*duplicateQuestion](err); ok {
					state.EvaluationRejections++
					state.EvaluationRejectedPrompt = duplicate.prompt
					state.Result.PromptTokens += usage.PromptTokens
					state.Result.CompletionTokens += usage.CompletionTokens
					if saveErr := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); saveErr != nil {
						return tmprl.StepResult[preparation.Artifacts]{}, saveErr
					}
				}
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			r.SourceDocument = p.Source.ID
			r.UserApproved = false
			r.ValidationPrompt = index < 10
			r.IndependentPrompt = index >= 10
			state.Records = append(state.Records, r)
			state.Result.PromptTokens += usage.PromptTokens
			state.Result.CompletionTokens += usage.CompletionTokens
			state.EvaluationNext++
			state.EvaluationRejections, state.EvaluationRejectedPrompt = 0, ""
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			report(tmprl.Progress{Completed: state.EvaluationNext, Total: 30, Message: "Generating evaluation questions"})
		}
	}
	if !state.Generated {
		state.Generated = true
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: input.Artifacts}, nil
}
