// Package preview produces a bounded, checked sample for user review.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
)

// ValidateReview decodes edits for this dataset's saved preview without changing
// either input. Returned records are normalized, approved training examples with
// fresh fact identities; evidence and source associations cannot change.
func ValidateReview(request preparation.PreparationRequest, saved *preparation.PreparationResult, payload json.RawMessage) (preparation.Review, error) {
	if saved == nil || saved.DatasetID != request.DatasetID {
		return preparation.Review{}, fmt.Errorf("%w: saved Preview is unavailable for this dataset", preparation.ErrInvalidArgument)
	}
	var review preparation.Review
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&review); err != nil {
		return preparation.Review{}, fmt.Errorf("%w: review payload: %s", preparation.ErrInvalidArgument, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return preparation.Review{}, fmt.Errorf("%w: review requires one JSON object", preparation.ErrInvalidArgument)
	}
	for _, r := range saved.Preview {
		if r.SourceDocument != request.DatasetID {
			return preparation.Review{}, fmt.Errorf("%w: Preview source does not match dataset", preparation.ErrInvalidArgument)
		}
	}
	switch request.Goal {
	case preparation.GoalQuestionAnswer, preparation.GoalClassification, preparation.GoalNumericPrediction, preparation.GoalTextGeneration:
	default:
		return preparation.Review{}, fmt.Errorf("%w: unsupported preparation goal", preparation.ErrInvalidArgument)
	}
	if saved.TrainingReady || len(saved.Preview) == 0 || len(saved.Preview) > 5 {
		return preparation.Review{}, fmt.Errorf("%w: review requires a saved preview of one to five records", preparation.ErrInvalidArgument)
	}
	if len(review.Records) != len(saved.Preview) {
		return preparation.Review{}, fmt.Errorf("%w: reviewed count must match the saved preview", preparation.ErrInvalidArgument)
	}
	reviewed := slices.Clone(review.Records)
	for i, r := range reviewed {
		original := saved.Preview[i]
		if r.Evidence != original.Evidence || r.SourceDocument != original.SourceDocument {
			return preparation.Review{}, fmt.Errorf("%w: record %d must preserve saved evidence and source", preparation.ErrInvalidArgument, i+1)
		}
		if r.ValidationPrompt || r.IndependentPrompt {
			return preparation.Review{}, fmt.Errorf("%w: record %d must be a training example", preparation.ErrInvalidArgument, i+1)
		}
		r.Prompt = strings.TrimSpace(r.Prompt)
		r.Target = training.Normalized(r.Target)
		// Preserve the versioned unit's exact evidence, including code boundaries.
		r.UserApproved = true
		r.ValidationPrompt = false
		r.IndependentPrompt = false
		r.FactID = training.FactID(r.Evidence, r.Target)
		if err := generate.ValidateRecord(request.Goal, r); err != nil {
			return preparation.Review{}, fmt.Errorf("%w: record %d: %s", preparation.ErrInvalidArgument, i+1, err)
		}
		reviewed[i] = r
	}
	if err := training.ValidateReviewedRecords(reviewed); err != nil {
		return preparation.Review{}, fmt.Errorf("%w: reviewed records: %s", preparation.ErrInvalidArgument, err)
	}
	return preparation.Review{Records: reviewed}, nil
}

// Units orders the structural source plan with an evenly spaced sample first.
// Remaining passages replace omissions until the preview record target is met.
func Units(document providers.Document, goal preparation.Goal, count int) ([]preparation.SourceUnit, []preparation.SourceExclusion, error) {
	if count < 1 || count > 5 {
		return nil, nil, fmt.Errorf("%w: preview count must be between 1 and 5", preparation.ErrInvalidArgument)
	}
	units, exclusions, err := extract.SourceUnits(document, goal)
	if err != nil {
		return nil, exclusions, err
	}
	count = min(count, len(units))
	selected := make([]preparation.SourceUnit, 0, len(units))
	sampled := make(map[string]bool, count)
	for i := range count {
		unit := units[i*len(units)/count]
		selected = append(selected, unit)
		sampled[unit.Reference] = true
	}
	for _, unit := range units {
		if !sampled[unit.Reference] {
			selected = append(selected, unit)
		}
	}
	return selected, exclusions, nil
}

// sampleRecords takes one example per passage before taking second examples.
// Preview's record limit is independent of its number of sampled passages.
func sampleRecords(units []preparation.SourceUnit, records []training.Record, count int) []training.Record {
	selected := make([]training.Record, 0, min(count, len(records)))
	for round := range 3 {
		for _, unit := range units {
			index := 0
			for _, record := range records {
				if record.Evidence != unit.Evidence {
					continue
				}
				if index == round {
					selected = append(selected, record)
					if len(selected) == count {
						return selected
					}
					break
				}
				index++
			}
		}
	}
	return selected
}
