package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// ValidateRequest normalizes and validates full document preparation without
// starting execution. The returned request owns a copy of ReviewedRecords.
func ValidateRequest(ctx context.Context, store *uploads.Store, input PreparationRequest) (PreparationRequest, error) {
	if err := ctx.Err(); err != nil {
		return PreparationRequest{}, err
	}
	if store == nil {
		return PreparationRequest{}, fmt.Errorf("%w: dataset service is required", ErrInvalidArgument)
	}
	input.DatasetID = strings.TrimSpace(input.DatasetID)
	input.TargetDatasetID = strings.TrimSpace(input.TargetDatasetID)
	if (input.TargetDatasetID == "") != (input.BaseVersion == 0) || input.BaseVersion < 0 || input.TargetDatasetID != "" && !validDatasetID(input.TargetDatasetID) {
		return PreparationRequest{}, fmt.Errorf("%w: select a valid destination dataset and base version together", ErrInvalidArgument)
	}
	input.Model = strings.TrimSpace(input.Model)
	input.Instructions = strings.TrimSpace(input.Instructions)
	input.ReviewedRecords = slices.Clone(input.ReviewedRecords)
	if input.Model == "" || utf8.RuneCountInString(input.Instructions) > 2000 {
		return PreparationRequest{}, fmt.Errorf("%w: invalid preparation model or instructions", ErrInvalidArgument)
	}
	switch input.Goal {
	case GoalQuestionAnswer, GoalClassification, GoalNumericPrediction, GoalTextGeneration:
	default:
		return PreparationRequest{}, fmt.Errorf("%w: unsupported preparation goal", ErrInvalidArgument)
	}
	if input.PreviewRecords < 0 || input.PreviewRecords > 5 || len(input.ReviewedRecords) > 5 {
		return PreparationRequest{}, fmt.Errorf("%w: preparation accepts a preview count of zero through five and at most five reviewed records", ErrInvalidArgument)
	}
	if input.MaxQuestions < 0 || input.MaxQuestions > 0 && len(input.ReviewedRecords) > input.MaxQuestions {
		return PreparationRequest{}, fmt.Errorf("%w: max_questions must be nonnegative and include all reviewed records", ErrInvalidArgument)
	}
	if err := (providers.DocumentOptions{PageStart: input.PageStart, PageEnd: input.PageEnd}).Validate(); err != nil {
		return PreparationRequest{}, err
	}
	if err := training.ValidateReviewedRecords(input.ReviewedRecords); err != nil {
		return PreparationRequest{}, fmt.Errorf("%w: reviewed records: %s", ErrInvalidArgument, err)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return PreparationRequest{}, err
	}
	if len(data) > 64<<10 {
		return PreparationRequest{}, fmt.Errorf("%w: preparation input exceeds 64 KiB", ErrInvalidArgument)
	}
	source, err := store.Get(ctx, input.DatasetID)
	if errors.Is(err, uploads.ErrInvalidArgument) {
		return PreparationRequest{}, fmt.Errorf("%w: source: %s", ErrInvalidArgument, err)
	}
	if err != nil {
		return PreparationRequest{}, err
	}
	if source.Format == ".jsonl" {
		return PreparationRequest{}, fmt.Errorf("%w: prepared JSONL does not need document preparation", ErrInvalidArgument)
	}
	if (input.PageStart != 0 || input.PageEnd != 0) && source.Format != ".pdf" {
		return PreparationRequest{}, fmt.Errorf("%w: page ranges require a PDF", ErrInvalidArgument)
	}
	switch source.Format {
	case ".pdf", ".docx", ".md", ".txt":
	default:
		return PreparationRequest{}, fmt.Errorf("%w: unsupported document format %q", ErrInvalidArgument, source.Format)
	}
	return input, nil
}
