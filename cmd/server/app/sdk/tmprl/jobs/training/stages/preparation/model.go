// Package preparation defines document preparation contracts and durable payloads.
package preparation

import (
	"context"
	"io"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// ErrInvalidArgument identifies invalid preparation input.
var ErrInvalidArgument = tmprl.ErrInvalidArgument

// Goal identifies the supervised task generated from a source document.
type Goal string

const (
	// GoalQuestionAnswer generates source-grounded questions and answers.
	GoalQuestionAnswer Goal = "question_answer"
	// GoalClassification generates source-supported category labels.
	GoalClassification Goal = "classification"
	// GoalNumericPrediction generates numeric targets from source evidence.
	GoalNumericPrediction Goal = "numeric_prediction"
	// GoalTextGeneration generates text targets from source passages.
	GoalTextGeneration Goal = "text_generation"
)

// PreparationKind distinguishes bounded previews from full preparation.
type PreparationKind string

const (
	// PreparationPreview bounds leaf computation to a sample of passages.
	PreparationPreview PreparationKind = "preview"
	// PreparationFull processes all supported passages and reviewed records.
	PreparationFull PreparationKind = "full"
)

// PreparationRequest refers to an imported document, never an arbitrary path.
type PreparationRequest struct {
	DatasetID       string            `json:"dataset_id"`
	Model           string            `json:"model"`
	Goal            Goal              `json:"goal"`
	Instructions    string            `json:"instructions,omitzero"`
	PreviewRecords  int               `json:"preview_records,omitzero"`
	ReviewedRecords []training.Record `json:"reviewed_records,omitzero"`
}

// PreparationResult identifies published records and preparation usage.
type PreparationResult struct {
	DatasetID        string            `json:"dataset_id"`
	RecordsPath      string            `json:"records_path"`
	ReportPath       string            `json:"report_path"`
	RecordCount      int               `json:"record_count"`
	Preview          []training.Record `json:"preview,omitzero"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	TrainingReady    bool              `json:"training_ready"`
	Warnings         []string          `json:"warnings,omitzero"`
}

// PreparationInput preserves durable preparation and checkpoint identity.
type PreparationInput struct {
	Request PreparationRequest `json:"request"`
	Kind    PreparationKind    `json:"kind"`
}

// DocumentExtractor converts a source and identifies its extraction profile.
type DocumentExtractor interface {
	Convert(context.Context, string) (providers.Document, error)
	ExtractionProfile() string
}

// ModelCompleter performs structured model completions.
type ModelCompleter interface {
	Complete(context.Context, providers.CompletionRequest) (providers.Completion, error)
}

// Artifacts contains only the outputs owned by preparation.
type Artifacts struct {
	Preparation *PreparationResult `json:"preparation,omitzero"`
}

// StepHandler executes one concrete preparation step.
type StepHandler interface {
	Name() string
	Execute(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)
}

// StepContext supplies confined resources, prior artifacts, and progress.
type StepContext struct {
	Preparation *PreparationContext
	Workspace   *os.Root
	Artifacts   Artifacts
	Publish     func(tmprl.Progress)
}

// PreparationContext supplies worker-local preparation dependencies.
type PreparationContext struct {
	Input      PreparationInput
	Source     uploads.File
	OpenSource func(context.Context) (io.ReadCloser, error)
	Extractor  DocumentExtractor
	Completer  ModelCompleter
}
