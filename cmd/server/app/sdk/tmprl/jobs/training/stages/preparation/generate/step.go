package generate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// Three durable request reservations per pending source batch, across activity runs.
const maxRequestAttempts uint32 = 3

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
	limit := p.Input.Request.MaxQuestions
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
		if limit > 0 && len(input.Review.Records) > limit {
			return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: reviewed examples exceed max_questions", preparation.ErrInvalidArgument)
		}
		for _, r := range input.Review.Records {
			if !r.UserApproved || r.SourceDocument != p.Source.ID || !slices.ContainsFunc(state.Units, func(u preparation.SourceUnit) bool { return u.Evidence == r.Evidence }) {
				return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: reviewed example must belong to the extracted passages", preparation.ErrInvalidArgument)
			}
			if err := ValidateRecord(p.Input.Request.Goal, r); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: %s", preparation.ErrInvalidArgument, err)
			}
		}
		state.Records = slices.Clone(input.Review.Records)
		state.ReviewID, state.ReviewedCount = input.ReviewID, len(state.Records)
		// Reviewed examples consume passage capacity, not the entire passage.
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
	commitBatch := func(end int) error {
		for _, decision := range state.GenerationDecisions {
			records := decision.Records
			if limit > 0 {
				records = records[:min(len(records), max(0, limit-len(state.Records)))]
			}
			state.Records = append(state.Records, records...)
			switch {
			case !decision.Complete:
				reason := fmt.Sprintf("generation failed after %d batch requests; valid candidates retained: %s", state.GenerationAttempts, decision.Reason)
				state.Result.Exclusions = append(state.Result.Exclusions, preparation.SourceExclusion{Reference: decision.Reference, Reason: reason})
				state.Result.Warnings = append(state.Result.Warnings, "Incomplete source coverage: "+decision.Reference+": "+reason)
			case decision.Reason == "model abstained: no_supported_example":
				state.Result.Exclusions = append(state.Result.Exclusions, preparation.SourceExclusion{Reference: decision.Reference, Reason: decision.Reason})
			}
		}
		state.Next = end
		if limit > 0 && len(state.Records) >= limit {
			state.Next = len(state.Units)
			state.Result.Warnings = append(state.Result.Warnings, "Document question cap reached; remaining source coverage and evaluation questions may be omitted")
		}
		state.GenerationAttempts = 0
		state.GenerationRejections, state.GenerationRejectedPrompt, state.GenerationRejectionReason = 0, "", ""
		state.GenerationDecisions = nil
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return err
		}
		report(tmprl.Progress{Completed: state.Next, Total: len(state.Units), Message: "Generated batch saved; verification still required"})
		return nil
	}
	report(tmprl.Progress{Completed: state.Next, Total: len(state.Units), Message: "Generating training examples"})
	for state.Next < len(state.Units) {
		if err := checkpoint.SafePoint(ctx, input); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if limit > 0 && len(state.Records) >= limit {
			if err := commitBatch(state.Next); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			break
		}
		batchSize := 5
		if limit > 0 {
			batchSize = min(batchSize, limit-len(state.Records))
		}
		// A resumed request keeps its original passage boundaries.
		if len(state.GenerationDecisions) > 0 {
			batchSize = len(state.GenerationDecisions)
		}
		end := min(state.Next+batchSize, len(state.Units))
		if len(state.GenerationDecisions) == 0 {
			for _, unit := range state.Units[state.Next:end] {
				state.GenerationDecisions = append(state.GenerationDecisions, checkpoint.GenerationDecision{Reference: unit.Reference})
			}
		}
		var pending []preparation.SourceUnit
		var positions []int
		previous := slices.Clone(state.Records)
		var feedback strings.Builder
		feedback.WriteString(state.GenerationRejectionReason)
		if state.GenerationRejectedPrompt != "" {
			feedback.WriteString(" The previous batch contained this rejected duplicate prompt: " + strconv.Quote(state.GenerationRejectedPrompt))
		}
		for i, decision := range state.GenerationDecisions {
			for _, record := range decision.Records {
				if err := ValidateRecord(p.Input.Request.Goal, record); err != nil {
					return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("invalid staged candidate: %w", err)
				}
				previous = append(previous, record)
			}
			if decision.Complete {
				continue
			}
			pending = append(pending, state.Units[state.Next+i])
			positions = append(positions, i)
			if decision.Reason != "" {
				feedback.WriteString(fmt.Sprintf("\nSource %s: %s", decision.Reference, decision.Reason))
			}
		}
		if len(pending) == 0 {
			if err := commitBatch(end); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		if state.GenerationAttempts >= maxRequestAttempts {
			var references []string
			for _, unit := range pending {
				references = append(references, unit.Reference)
			}
			return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("generation request budget exhausted at source index %d; unresolved sources: %s; provisional candidates preserved; invalid responses are not source omissions", state.Next, strings.Join(references, ", ")), "preparation-request-budget-exhausted", nil)
		}
		state.GenerationAttempts++
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		outcome, usage, err := Run(ctx, p.Completer, p.Input.Request, pending, previous, state.GenerationAttempts-1, feedback.String())
		if canceled := ctx.Err(); canceled != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, canceled
		}
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		if err != nil {
			if rejected, ok := errors.AsType[*rejectedCompletion](err); ok {
				state.GenerationRejections++
				state.GenerationRejectionReason = rejected.reason
				state.GenerationRejectedPrompt = ""
				if saveErr := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); saveErr != nil {
					return tmprl.StepResult[preparation.Artifacts]{}, saveErr
				}
				continue // Local correction; does not spend a Temporal retry.
			}
			if saveErr := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); saveErr != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, saveErr
			}
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		state.GenerationRejectionReason, state.GenerationRejectedPrompt = "", outcome.RejectedPrompt
		unresolved := false
		for i, decision := range outcome.Decisions {
			for j := range decision.Records {
				decision.Records[j].SourceDocument = p.Source.ID
			}
			if !decision.Complete {
				unresolved = true
			}
			decision.Records = append(state.GenerationDecisions[positions[i]].Records, decision.Records...)
			state.GenerationDecisions[positions[i]] = decision
		}
		candidates := len(state.Records)
		for _, decision := range state.GenerationDecisions {
			candidates += len(decision.Records)
		}
		// The envelope was validated and these candidates passed grounding and
		// integrity checks. Unneeded candidates must not block a capped run.
		if limit > 0 && candidates >= limit {
			if err := commitBatch(end); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		// Only a validated final response can resolve individual source failures.
		// Unknown/interrupted requests and malformed envelopes retain the fatal gate.
		if unresolved && state.GenerationAttempts == maxRequestAttempts {
			if err := commitBatch(end); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		if unresolved {
			state.GenerationRejections++
			state.GenerationRejectionReason = "repair only the unresolved source units; saved candidates must not be repeated"
		} else if state.GenerationRejections > 0 {
			state.GenerationRejectionReason = "all batch candidates staged; commit pending"
		}
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}
	if len(state.Records) == 0 {
		return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError("no supported examples were generated; source exclusions are saved in the preparation checkpoint", "preparation-no-examples", nil)
	}
	training := slices.DeleteFunc(slices.Clone(state.Records), func(r training.Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
	if p.Input.Kind == preparation.PreparationFull && p.Input.Request.Goal == preparation.GoalQuestionAnswer && len(training) >= 20 {
		report(tmprl.Progress{Completed: state.EvaluationNext, Total: 30, Message: "Generating evaluation questions"})
		for state.EvaluationNext < 30 && (limit == 0 || len(state.Records) < limit) {
			if err := checkpoint.SafePoint(ctx, input); err != nil {
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
	if err := checkpoint.SafePoint(ctx, input); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: input.Artifacts}, nil
}
