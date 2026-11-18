// Package checkpoint owns the preparation steps' shared durable state format.
// Steps own their transitions; this package only loads and validates state.
package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"go.temporal.io/sdk/temporal"
)

// File is the shared checkpoint path within the preparation workspace.
const File = "steps.json"

// Version identifies the passage plan, multi-example recovery, omission policy,
// inline verification and per-dataset evaluation questions.
const Version = 5

// SafePoint is called only where no work is in flight and File contains the
// committed cursor/results. A suspension must propagate without wrapping it as
// a computation failure. File-only callers retain their original behavior.
func SafePoint(ctx context.Context, input preparation.StepContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.SafePoint != nil {
		return input.SafePoint(ctx, File)
	}
	return nil
}

// GenerationDecision retains up to three candidates while a passage is repaired.
// A completed empty result is a reported model-selected omission.
type GenerationDecision struct {
	Reference string            `json:"reference"`
	Records   []training.Record `json:"records,omitzero"`
	Complete  bool              `json:"complete"`
	Reason    string            `json:"reason,omitzero"`
}

// State tracks independent generation and verification cursors. No final
// artifact exists until verification succeeds.
type State struct {
	Version        int                      `json:"version"`
	Fingerprint    string                   `json:"fingerprint"`
	Units          []preparation.SourceUnit `json:"units"`
	Records        []training.Record        `json:"records"`
	ReviewID       string                   `json:"review_id,omitzero"`
	ReviewedCount  int                      `json:"reviewed_count,omitzero"`
	Next           int                      `json:"next"`
	EvaluationNext int                      `json:"evaluation_next"`
	// Reserve before dispatch. An uncertain interrupted request stays consumed.
	GenerationAttempts uint32 `json:"generation_attempts,omitzero"`
	// Rejection feedback belongs only to the pending Next training batch.
	GenerationRejections      uint32 `json:"generation_rejections,omitzero"`
	GenerationRejectedPrompt  string `json:"generation_rejected_prompt,omitzero"`
	GenerationRejectionReason string `json:"generation_rejection_reason,omitzero"`
	// GenerationLastRejected reports that the last reserved request returned a
	// reply that failed validation. An interrupted request leaves it false.
	GenerationLastRejected bool `json:"generation_last_rejected,omitzero"`
	// Decisions remain provisional until every unit in the original batch resolves.
	GenerationDecisions []GenerationDecision `json:"generation_decisions,omitzero"`
	// Rejection feedback belongs only to the pending EvaluationNext question.
	EvaluationRejections     uint32                        `json:"evaluation_rejections,omitzero"`
	EvaluationRejectedPrompt string                        `json:"evaluation_rejected_prompt,omitzero"`
	Verified                 int                           `json:"verified"`
	Generated                bool                          `json:"generated"`
	Result                   preparation.PreparationResult `json:"result"`
}

// Load validates identity and checkpoint bounds. initialize permits a missing
// checkpoint for Extract or for Preview within its isolated workspace.
// Generate and Verify require existing state and must pass false.
// exists distinguishes saved business progress from initialization.
func Load(ctx context.Context, input preparation.StepContext, initialize bool) (state State, exists bool, err error) {
	if err := ctx.Err(); err != nil {
		return State{}, false, err
	}
	p := input.Preparation
	if p == nil || input.Workspace == nil || p.OpenSource == nil {
		return State{}, false, fmt.Errorf("%w: preparation resources are required", preparation.ErrInvalidArgument)
	}
	if p.Input.Kind != preparation.PreparationFull && p.Input.Kind != preparation.PreparationPreview || p.Input.Kind == preparation.PreparationPreview && (p.Input.Request.PreviewRecords < 1 || p.Input.Request.PreviewRecords > 5) {
		return State{}, false, fmt.Errorf("%w: invalid preparation kind or preview count", preparation.ErrInvalidArgument)
	}
	identity, err := json.Marshal(struct {
		Input  preparation.PreparationInput
		Source uploads.File
	}{p.Input, p.Source})
	if err != nil {
		return State{}, false, err
	}
	hash := sha256.Sum256(identity)
	fingerprint := "preparation/" + hex.EncodeToString(hash[:])
	state = State{Version: Version, Fingerprint: fingerprint, Result: preparation.PreparationResult{DatasetID: p.Source.ID}}
	data, err := input.Workspace.ReadFile(File)
	if errors.Is(err, os.ErrNotExist) {
		if !initialize {
			return State{}, false, fmt.Errorf("preparation checkpoint must be initialized before this step")
		}
		return state, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return State{}, false, err
	}
	if header.Version != Version {
		return State{}, false, temporal.NewNonRetryableApplicationError("incompatible preparation checkpoint: start a new Preparation with a new Preview; saved progress was not modified", "preparation-checkpoint-version", nil)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, err
	}
	if state.Fingerprint != fingerprint {
		return State{}, false, fmt.Errorf("preparation checkpoint does not match source or configuration")
	}
	// Captured requests may supply reviewed records before the review gate.
	if state.ReviewID == "" {
		state.ReviewedCount = len(p.Input.Request.ReviewedRecords)
	}
	if state.ReviewedCount < 0 || state.ReviewedCount > len(state.Records) {
		return State{}, false, fmt.Errorf("invalid reviewed checkpoint bounds")
	}
	if state.ReviewID != "" && (input.Review == nil || input.ReviewID != state.ReviewID || !slices.Equal(state.Records[:state.ReviewedCount], input.Review.Records)) {
		return State{}, false, fmt.Errorf("%w: accepted review changed after generation started", preparation.ErrInvalidArgument)
	}
	if state.Next < 0 || state.Next > len(state.Units) || state.EvaluationNext < 0 || state.Verified < 0 || state.Verified > len(state.Records) {
		return State{}, false, fmt.Errorf("invalid checkpoint bounds")
	}
	seen := make(map[string]bool, len(state.Units))
	for _, unit := range state.Units {
		if strings.TrimSpace(unit.Reference) == "" || strings.TrimSpace(unit.Evidence) == "" || len(unit.Targets) == 0 || seen[unit.Reference] {
			return State{}, false, fmt.Errorf("invalid source-unit plan")
		}
		seen[unit.Reference] = true
		for _, target := range unit.Targets {
			if strings.TrimSpace(target) == "" {
				return State{}, false, fmt.Errorf("empty selectable source target")
			}
		}
	}
	if state.GenerationAttempts > 3 || state.GenerationRejections > state.GenerationAttempts {
		return State{}, false, fmt.Errorf("invalid request reservation bounds")
	}
	// Full preparation verifies records while it generates, so only Preview
	// requires generation to precede verification.
	verifiedEarly := p.Input.Kind == preparation.PreparationPreview && state.Verified != 0
	if state.GenerationAttempts > 0 && (state.Next >= len(state.Units) || state.Generated || state.EvaluationNext != 0 || verifiedEarly) {
		return State{}, false, fmt.Errorf("request reservation does not match pending source batch")
	}
	if state.GenerationRejections == 0 && (state.GenerationRejectedPrompt != "" || state.GenerationRejectionReason != "") || state.GenerationRejections > 0 && strings.TrimSpace(state.GenerationRejectedPrompt+state.GenerationRejectionReason) == "" || state.GenerationLastRejected && strings.TrimSpace(state.GenerationRejectionReason) == "" {
		return State{}, false, fmt.Errorf("inconsistent generation rejection checkpoint")
	}
	if state.GenerationRejections > 0 && (state.Generated || state.Next >= len(state.Units) || state.EvaluationNext != 0 || verifiedEarly) {
		return State{}, false, fmt.Errorf("generation rejection checkpoint has no pending training batch")
	}
	if len(state.GenerationDecisions) > 0 {
		end := min(state.Next+5, len(state.Units))
		// A record limit shrinks the last batch to the records still needed.
		validSize := len(state.GenerationDecisions) == end-state.Next
		if p.Input.Request.MaxQuestions > 0 {
			validSize = len(state.GenerationDecisions) <= end-state.Next
		}
		if state.GenerationAttempts == 0 || state.Generated || state.EvaluationNext != 0 || verifiedEarly || !validSize {
			return State{}, false, fmt.Errorf("pending generation decisions do not match the reserved batch")
		}
		candidates := slices.Clone(state.Records)
		for i, decision := range state.GenerationDecisions {
			unit := state.Units[state.Next+i]
			if decision.Reference != unit.Reference {
				return State{}, false, fmt.Errorf("pending generation decision has the wrong source reference")
			}
			count := len(decision.Records)
			facts := make(map[string]bool)
			for _, r := range state.Records {
				if r.Evidence == unit.Evidence {
					count++
					facts[r.FactID] = true
				}
			}
			if count > 3 {
				return State{}, false, fmt.Errorf("a passage has more than three staged examples")
			}
			if decision.Complete && (count == 0 && decision.Reason != "model abstained: no_supported_example" || count > 0 && decision.Reason != "") {
				return State{}, false, fmt.Errorf("completed passage has an inconsistent omission reason")
			}
			for _, r := range decision.Records {
				grounded := slices.Contains(unit.Targets, r.Target)
				if p.Input.Request.Goal == preparation.GoalQuestionAnswer || p.Input.Request.Goal == preparation.GoalTextGeneration {
					grounded = strings.TrimSpace(r.Target) != "" && utf8.RuneCountInString(r.Target) <= 800 && slices.ContainsFunc(unit.Targets, func(text string) bool { return strings.Contains(text, r.Target) })
				}
				if !grounded || facts[r.FactID] || r.Evidence != unit.Evidence || r.FactID != training.FactID(r.Evidence, r.Target) || r.SourceDocument != p.Source.ID || r.UserApproved || r.ValidationPrompt || r.IndependentPrompt {
					return State{}, false, fmt.Errorf("pending generation candidate changed source, target or ownership")
				}
				facts[r.FactID] = true
				candidates = append(candidates, r)
			}
		}
		if len(candidates) > 0 {
			if err := training.ValidateRecords(candidates); err != nil {
				return State{}, false, err
			}
		}
	}
	if state.EvaluationRejections == 0 && state.EvaluationRejectedPrompt != "" || state.EvaluationRejections > 0 && strings.TrimSpace(state.EvaluationRejectedPrompt) == "" {
		return State{}, false, fmt.Errorf("inconsistent evaluation rejection checkpoint")
	}
	if state.EvaluationRejections > 0 && (state.Generated || state.Next != len(state.Units) || p.Input.Kind != preparation.PreparationFull || p.Input.Request.Goal != preparation.GoalQuestionAnswer) {
		return State{}, false, fmt.Errorf("evaluation rejection checkpoint has no pending evaluation question")
	}
	if len(state.Records) > 0 {
		if err := training.ValidateRecords(state.Records); err != nil {
			return State{}, false, err
		}
	}
	return state, true, nil
}
