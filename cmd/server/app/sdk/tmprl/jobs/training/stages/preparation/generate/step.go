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
type Step struct {
	// Verify verifies the next batch of unverified records. Full preparation
	// sets it so rejected records are replaced up to the record limit; Preview
	// leaves it nil and the verify step checks every record afterwards.
	Verify func(context.Context, preparation.StepContext, *checkpoint.State) error
}

// Name identifies the generation step within the preparation stage.
func (Step) Name() string { return "generate" }

// Execute resumes generation without repeating committed batches. It never
// publishes a dataset; the verify step owns publication.
func (s Step) Execute(ctx context.Context, input preparation.StepContext) (tmprl.StepResult[preparation.Artifacts], error) {
	state, _, err := checkpoint.Load(ctx, input, false)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	p := input.Preparation
	limit := p.Input.Request.MaxQuestions
	if p.Input.Kind == preparation.PreparationPreview {
		limit = p.Input.Request.PreviewRecords
	}
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
		// Reviewed examples count toward the requested records.
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
		if limit > 0 && p.Input.Kind == preparation.PreparationFull {
			// Count this run's retained records, not its source-passage cursor.
			// Accepted preview records count immediately; pending candidates do not.
			progress.Completed, progress.Total = len(state.Records), limit
			if s.Verify != nil {
				progress.Completed = min(len(state.Records), max(state.Verified, state.ReviewedCount))
			}
		}
		if input.Publish != nil {
			input.Publish(progress)
		}
	}
	// The dataset's base version decides which evaluation questions it still
	// needs. Datasets without fact identities cannot hold evaluation questions.
	var base []training.Record
	baseKeys := make(map[string]bool)
	baseValidation, baseTest := make(map[string]bool), make(map[string]bool)
	for _, r := range p.DatasetRecords {
		baseKeys[training.Key(r.Prompt)] = true
		switch {
		case r.ValidationPrompt:
			baseValidation[r.FactID] = true
		case r.IndependentPrompt:
			baseTest[r.FactID] = true
		default:
			base = append(base, r)
		}
	}
	evaluate := p.Input.Kind == preparation.PreparationFull && p.Input.Request.Goal == preparation.GoalQuestionAnswer && !slices.ContainsFunc(p.DatasetRecords, func(r training.Record) bool { return r.FactID == "" })
	missing := func(trainingRecords int) (validation, test int) {
		if !evaluate || trainingRecords < training.MinimumTrainingRecords {
			return 0, 0
		}
		validation, test = training.EvaluationNeeds(trainingRecords)
		return max(0, validation-len(baseValidation)), max(0, test-len(baseTest))
	}
	// The record limit covers this run's training records and the evaluation
	// questions the dataset then needs. Without room for both, the run saves
	// training records only.
	target := limit
	for t := limit; t >= 1; t-- {
		if validation, test := missing(len(base) + t); len(base)+t >= training.MinimumTrainingRecords && t+validation+test <= limit {
			target = t
			break
		}
	}
	// Multiple question variants of a fact belong to training, not to the
	// one-training-question-per-fact evaluation recipe.
	updateTarget := func() {
		seen := make(map[string]bool)
		records := state.Records
		if s.Verify != nil {
			records = records[:state.Verified]
		}
		for _, r := range records {
			if r.ValidationPrompt || r.IndependentPrompt {
				continue
			}
			if seen[r.FactID] {
				evaluate = false
				target = limit
			}
			seen[r.FactID] = true
		}
	}
	updateTarget()
	// verifyAll verifies pending records so rejected ones are replaced while
	// passages or evaluation candidates remain.
	verifyAll := func() error {
		for s.Verify != nil && state.Verified < len(state.Records) {
			if err := checkpoint.SafePoint(ctx, input); err != nil {
				return err
			}
			if err := s.Verify(ctx, input, &state); err != nil {
				return err
			}
		}
		return nil
	}
	commitBatch := func(end int) error {
		// Keep preview diversity when a passage supplies multiple examples:
		// take one from each passage before taking second or third examples.
		if p.Input.Kind == preparation.PreparationPreview {
			for round := range 3 {
				for _, decision := range state.GenerationDecisions {
					if round < len(decision.Records) && len(state.Records) < target {
						state.Records = append(state.Records, decision.Records[round])
					}
				}
			}
		}
		for _, decision := range state.GenerationDecisions {
			records := decision.Records
			if target > 0 {
				records = records[:min(len(records), max(0, target-len(state.Records)))]
			}
			if p.Input.Kind == preparation.PreparationFull {
				state.Records = append(state.Records, records...)
			}
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
		state.GenerationAttempts = 0
		state.GenerationRejections, state.GenerationRejectedPrompt, state.GenerationRejectionReason, state.GenerationLastRejected = 0, "", "", false
		state.GenerationDecisions = nil
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return err
		}
		if limit > 0 && p.Input.Kind == preparation.PreparationFull {
			if err := verifyAll(); err != nil {
				return err
			}
		}
		updateTarget()
		if target > 0 && len(state.Records) >= target {
			// Verified records count toward the limit; rejected ones are replaced.
			if err := verifyAll(); err != nil {
				return err
			}
			if len(state.Records) >= target {
				state.Next = len(state.Units)
				if p.Input.Kind == preparation.PreparationFull {
					state.Result.Warnings = append(state.Result.Warnings, "Record limit reached; the remaining passages were not used")
				}
				if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
					return err
				}
			}
		}
		report(tmprl.Progress{Completed: state.Next, Total: len(state.Units), Message: "Generated batch saved; verification still required"})
		return nil
	}
	report(tmprl.Progress{Completed: state.Next, Total: len(state.Units), Message: "Generating training examples"})
	for !state.Generated {
		if err := checkpoint.SafePoint(ctx, input); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if state.Next == len(state.Units) {
			if err := verifyAll(); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			updateTarget()
			if target == 0 || p.Input.Kind != preparation.PreparationFull || len(state.Records) >= target || len(state.Units) == 0 || state.EvaluationNext > 0 {
				break
			}
			state.GenerationStalls++
			if len(state.Records) > state.GenerationRoundStart {
				state.GenerationStalls = 0
			}
			if state.GenerationStalls >= 3 {
				state.Result.Warnings = append(state.Result.Warnings, fmt.Sprintf("Requested %d records; generation stopped after three passes without additional verified records", limit))
				break
			}
			state.GenerationRound++
			state.GenerationRoundStart = len(state.Records)
			state.Next = 0
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			report(tmprl.Progress{Completed: len(state.Records), Total: limit, Message: "Generating additional records to reach the requested count"})
		}
		if target > 0 && len(state.Records) >= target {
			if err := commitBatch(state.Next); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		batchSize := 5
		if target > 0 {
			batchSize = min(batchSize, target-len(state.Records))
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
			// Verification exclusions survive batch commits and checkpoint resumes.
			// Supply only the latest rejection for this pending passage.
			for _, exclusion := range slices.Backward(state.Result.Exclusions) {
				if exclusion.Reference != decision.Reference || !strings.HasPrefix(exclusion.Reason, "verification rejected example ") {
					continue
				}
				var repair string
				switch {
				case strings.HasSuffix(exclusion.Reason, ": grounding"):
					repair = "Recheck the entire evidence, including headings, negation, conditions and named referents. Ask only what the quoted answer fully supports."
				case strings.HasSuffix(exclusion.Reason, ": task_quality"):
					repair = "Write a useful standalone prompt for the requested goal. Request the answer without disclosing it or relying on unseen context."
				case strings.HasSuffix(exclusion.Reason, ": user_requirements"):
					repair = "Follow the requested topic and explicit guidance using only supported evidence; do not invent facts to satisfy guidance."
				default:
					continue
				}
				fmt.Fprintf(&feedback, "\nEVIDENCE %d prior %s. %s", len(pending), exclusion.Reason, repair)
				break
			}
		}
		if limit > 0 && p.Input.Kind == preparation.PreparationFull {
			fmt.Fprintf(&feedback, "\nGenerate toward the requested %d verified records including evaluation questions. %d more training records are needed. Return up to three NEW questions per passage on this request. Prefer unused supported facts; different standalone questions about a saved fact are also allowed. Do not repeat saved prompts or invent facts.", limit, max(0, target-len(previous)))
		}
		if len(pending) == 0 {
			if err := commitBatch(end); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		if state.GenerationAttempts >= maxRequestAttempts {
			// Only a last reply that failed validation excludes the unresolved
			// passages. An interrupted request has an unknown outcome and stops the run.
			if !state.GenerationLastRejected {
				var references []string
				for _, unit := range pending {
					references = append(references, unit.Reference)
				}
				return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("generation request budget exhausted at source index %d after a request with an unknown outcome; unresolved sources: %s; provisional candidates preserved", state.Next, strings.Join(references, ", ")), "preparation-request-budget-exhausted", nil)
			}
			// Passages that already resolved, and candidates already staged, are kept.
			for _, i := range positions {
				state.GenerationDecisions[i].Reason = "invalid model reply: " + state.GenerationRejectionReason
			}
			if err := commitBatch(end); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			continue
		}
		state.GenerationAttempts++
		state.GenerationLastRejected = false
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		outcome, usage, err := Run(ctx, p.Completer, p.Input.Request, pending, previous, uint32(state.GenerationRound)*maxRequestAttempts+state.GenerationAttempts-1, feedback.String())
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
				state.GenerationLastRejected = true
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
		if target > 0 && candidates >= target {
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
		// Committed exclusions preserve exhausted generation attempts on replay.
		// Distinguish rejected replies from a valid model-selected abstention.
		if slices.ContainsFunc(state.Result.Exclusions, func(exclusion preparation.SourceExclusion) bool {
			return strings.HasPrefix(exclusion.Reason, "generation failed after ")
		}) {
			return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError("generation request budget exhausted without valid examples; rejected model replies are recorded in the preparation checkpoint", "preparation-request-budget-exhausted", nil)
		}
		return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError("no supported examples were generated; source exclusions are saved in the preparation checkpoint", "preparation-no-examples", nil)
	}
	if err := verifyAll(); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	// Evaluation questions reword training records from any document in the
	// dataset: validation questions first, then test questions, one of each kind
	// per record at most. A rejected question is replaced from the next record.
	facts := slices.DeleteFunc(slices.Clone(state.Records), func(r training.Record) bool {
		return r.ValidationPrompt || r.IndependentPrompt || baseKeys[training.Key(r.Prompt)]
	})
	facts = append(facts, base...)
	needValidation, needTest := missing(len(facts))
	asked := func() (validation, test int) {
		for _, r := range state.Records {
			switch {
			case r.ValidationPrompt:
				validation++
			case r.IndependentPrompt:
				test++
			}
		}
		return validation, test
	}
	// A partial evaluation set cannot split, so it is added only when it fits.
	if validation, test := asked(); limit > 0 && len(state.Records)-validation-test+needValidation+needTest > limit {
		needValidation, needTest = 0, 0
	}
	type slot struct {
		fact       training.Record
		validation bool
	}
	var slots []slot
	for _, f := range facts {
		if !baseValidation[f.FactID] {
			slots = append(slots, slot{f, true})
		}
	}
	for _, f := range facts {
		if !baseTest[f.FactID] {
			slots = append(slots, slot{f, false})
		}
	}
	if needValidation+needTest > 0 {
		validation, test := asked()
		report(tmprl.Progress{Completed: validation + test, Total: needValidation + needTest, Message: "Generating evaluation questions"})
	}
	for state.EvaluationNext < len(slots) && (limit == 0 || len(state.Records) < limit) {
		validation, test := asked()
		if validation >= needValidation && test >= needTest {
			break
		}
		next := slots[state.EvaluationNext]
		if next.validation && validation >= needValidation || !next.validation && test >= needTest {
			state.EvaluationNext++
			continue
		}
		if err := checkpoint.SafePoint(ctx, input); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		// A run record that repeats a dataset question is skipped when saved.
		previous := append(slices.Clone(p.DatasetRecords), slices.DeleteFunc(slices.Clone(state.Records), func(r training.Record) bool { return baseKeys[training.Key(r.Prompt)] })...)
		r, usage, err := EvaluationRecord(ctx, p.Completer, p.Input.Request, next.fact, previous, state.EvaluationRejections, state.EvaluationRejectedPrompt)
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
		r.ValidationPrompt = next.validation
		r.IndependentPrompt = !next.validation
		state.Records = append(state.Records, r)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.EvaluationNext++
		state.EvaluationRejections, state.EvaluationRejectedPrompt = 0, ""
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if err := verifyAll(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		validation, test = asked()
		report(tmprl.Progress{Completed: validation + test, Total: needValidation + needTest, Message: "Generating evaluation questions"})
	}
	if !state.Generated {
		state.Generated = true
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}
	if limit > 0 && p.Input.Kind == preparation.PreparationFull {
		report(tmprl.Progress{Message: "Generated batch saved; verification still required"})
	}
	if err := checkpoint.SafePoint(ctx, input); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: input.Artifacts}, nil
}
