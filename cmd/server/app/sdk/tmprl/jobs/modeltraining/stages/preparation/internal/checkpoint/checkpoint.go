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

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
)

// File is the shared checkpoint path within the preparation workspace.
const File = "steps.json"

// State preserves v2 checkpoint compatibility, including independent generation
// and verification cursors. No final artifact exists until verification succeeds.
type State struct {
	Fingerprint    string                        `json:"fingerprint"`
	Passages       []string                      `json:"passages"`
	Records        []training.Record             `json:"records"`
	Next           int                           `json:"next"`
	EvaluationNext int                           `json:"evaluation_next"`
	Verified       int                           `json:"verified"`
	Generated      bool                          `json:"generated"`
	Result         preparation.PreparationResult `json:"result"`
}

// Load validates identity and checkpoint bounds. Only extraction allows a missing
// checkpoint; exists distinguishes an extraction replay from initialization.
func Load(ctx context.Context, input preparation.StepContext, allowMissing bool) (state State, exists bool, err error) {
	if err := ctx.Err(); err != nil {
		return State{}, false, err
	}
	p := input.Preparation
	if p == nil || input.Workspace == nil || p.SourceRoot == nil {
		return State{}, false, fmt.Errorf("%w: preparation resources are required", preparation.ErrInvalidArgument)
	}
	if p.Input.Kind != preparation.PreparationFull && p.Input.Kind != preparation.PreparationPreview || p.Input.Kind == preparation.PreparationPreview && (p.Input.Request.PreviewRecords < 1 || p.Input.Request.PreviewRecords > 5) {
		return State{}, false, fmt.Errorf("%w: invalid preparation kind or preview count", preparation.ErrInvalidArgument)
	}
	identity, err := json.Marshal(struct {
		Input  preparation.PreparationInput
		Source imports.PreparationSource
	}{p.Input, p.Source})
	if err != nil {
		return State{}, false, err
	}
	hash := sha256.Sum256(identity)
	fingerprint := "preparation-steps-v2/" + hex.EncodeToString(hash[:])
	state = State{Fingerprint: fingerprint, Result: preparation.PreparationResult{DatasetID: p.Source.ID}}
	data, err := input.Workspace.ReadFile(File)
	if errors.Is(err, os.ErrNotExist) {
		if !allowMissing {
			return State{}, false, fmt.Errorf("extract must complete before this step")
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
	if state.Next < 0 || state.Next > len(state.Passages) || state.EvaluationNext < 0 || state.EvaluationNext > 30 || state.Verified < 0 || state.Verified > len(state.Records) {
		return State{}, false, fmt.Errorf("invalid checkpoint bounds")
	}
	if len(state.Records) > 0 {
		if err := training.ValidateRecords(state.Records); err != nil {
			return State{}, false, err
		}
	}
	return state, true, nil
}
