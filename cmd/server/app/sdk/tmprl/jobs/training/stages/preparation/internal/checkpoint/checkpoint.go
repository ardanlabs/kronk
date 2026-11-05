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

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// File is the shared checkpoint path within the preparation workspace.
const File = "steps.json"

// State tracks independent generation and verification cursors. No final
// artifact exists until verification succeeds.
type State struct {
	Fingerprint    string            `json:"fingerprint"`
	Passages       []string          `json:"passages"`
	Records        []training.Record `json:"records"`
	ReviewID       string            `json:"review_id,omitzero"`
	ReviewedCount  int               `json:"reviewed_count,omitzero"`
	Next           int               `json:"next"`
	EvaluationNext int               `json:"evaluation_next"`
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
	state = State{Fingerprint: fingerprint, Result: preparation.PreparationResult{DatasetID: p.Source.ID}}
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
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, err
	}
	if state.Fingerprint != fingerprint {
		return State{}, false, fmt.Errorf("preparation checkpoint does not match source or configuration")
	}
	// Earlier captured plans supplied reviewed records in the original request.
	if state.ReviewID == "" {
		state.ReviewedCount = len(p.Input.Request.ReviewedRecords)
	}
	if state.ReviewedCount < 0 || state.ReviewedCount > len(state.Records) {
		return State{}, false, fmt.Errorf("invalid reviewed checkpoint bounds")
	}
	if state.ReviewID != "" && (input.Review == nil || input.ReviewID != state.ReviewID || !slices.Equal(state.Records[:state.ReviewedCount], input.Review.Records)) {
		return State{}, false, fmt.Errorf("%w: accepted review changed after generation started", preparation.ErrInvalidArgument)
	}
	if state.Next < 0 || state.Next > len(state.Passages) || state.EvaluationNext < 0 || state.EvaluationNext > 30 || state.Verified < 0 || state.Verified > len(state.Records) {
		return State{}, false, fmt.Errorf("invalid checkpoint bounds")
	}
	if state.EvaluationRejections == 0 && state.EvaluationRejectedPrompt != "" || state.EvaluationRejections > 0 && strings.TrimSpace(state.EvaluationRejectedPrompt) == "" {
		return State{}, false, fmt.Errorf("inconsistent evaluation rejection checkpoint")
	}
	if state.EvaluationRejections > 0 && (state.Generated || state.Next != len(state.Passages) || state.EvaluationNext >= 30 || state.Verified != 0 || p.Input.Kind != preparation.PreparationFull || p.Input.Request.Goal != preparation.GoalQuestionAnswer || len(state.Records)-state.EvaluationNext < 20) {
		return State{}, false, fmt.Errorf("evaluation rejection checkpoint has no pending evaluation question")
	}
	if len(state.Records) > 0 {
		if err := training.ValidateRecords(state.Records); err != nil {
			return State{}, false, err
		}
	}
	return state, true, nil
}
