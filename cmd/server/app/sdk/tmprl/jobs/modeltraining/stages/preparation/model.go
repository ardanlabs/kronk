// Package preparation defines document preparation contracts and durable payloads.
package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// ErrInvalidArgument identifies invalid preparation input.
var ErrInvalidArgument = tmprl.ErrInvalidArgument

// ErrCompletionTruncated identifies completions that reach their token limit.
var ErrCompletionTruncated = errors.New("preparation completion reached its token limit")

// Goal identifies the supervised task generated from a source document.
type Goal string

const (
	GoalQuestionAnswer    Goal = "question_answer"
	GoalClassification    Goal = "classification"
	GoalNumericPrediction Goal = "numeric_prediction"
	GoalTextGeneration    Goal = "text_generation"
)

// PreparationKind distinguishes bounded previews from full preparation.
type PreparationKind string

const (
	PreparationPreview PreparationKind = "preview"
	PreparationFull    PreparationKind = "full"
)

// StagePrepare preserves the historical preparation progress identity.
const StagePrepare tmprl.StageName = "prepare"

// PreparationRequest refers to an imported document, never an arbitrary path.
type PreparationRequest struct {
	ID              string            `json:"id"`
	DatasetID       string            `json:"dataset_id"`
	Model           string            `json:"model"`
	Goal            Goal              `json:"goal"`
	Instructions    string            `json:"instructions,omitzero"`
	PreviewRecords  int               `json:"preview_records,omitzero"`
	ReviewedRecords []training.Record `json:"reviewed_records,omitzero"`
}

// PreparationResult identifies published records and preparation usage.
type PreparationResult struct {
	DatasetID   string `json:"dataset_id"`
	RecordsPath string `json:"records_path"`
	ReportPath  string `json:"report_path"`
	// Reserved serialized field; current preparation does not produce spare rows.
	SpareRecordsPath string            `json:"spare_records_path,omitzero"`
	RecordCount      int               `json:"record_count"`
	Preview          []training.Record `json:"preview,omitzero"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	TrainingReady    bool              `json:"training_ready"`
	Warnings         []string          `json:"warnings,omitzero"`
}

// PreparationRun describes a durable preparation execution.
type PreparationRun struct {
	tmprl.Execution
	Kind      PreparationKind    `json:"kind"`
	Request   PreparationRequest `json:"request"`
	Status    tmprl.Status       `json:"status"`
	Result    *PreparationResult `json:"result,omitzero"`
	Progress  *tmprl.Progress    `json:"progress,omitzero"`
	StartedAt time.Time          `json:"started_at"`
	ClosedAt  time.Time          `json:"closed_at,omitzero"`
	Error     string             `json:"error,omitzero"`
}

// PreparationInput preserves durable preparation and checkpoint identity.
type PreparationInput struct {
	Request PreparationRequest `json:"request"`
	Kind    PreparationKind    `json:"kind"`
	Timeout time.Duration      `json:"timeout"`
}

// Document preserves extracted document content and conversion metadata.
type Document struct {
	Markdown       string          `json:"markdown"`
	StructuredJSON json.RawMessage `json:"structured_json"`
	ProcessingTime float64         `json:"processing_time"`
}

// Message is one model conversation message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest requests a structured preparation completion.
type CompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseSchema map[string]any `json:"response_schema"`
	MaxTokens      int            `json:"max_tokens"`
	Seed           uint32         `json:"seed"`
}

// Completion contains generated content and model usage.
type Completion struct {
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

// DocumentExtractor converts a source and identifies its extraction profile.
type DocumentExtractor interface {
	Convert(context.Context, string) (Document, error)
	ExtractionProfile() string
}

// ModelCompleter performs structured model completions.
type ModelCompleter interface {
	Complete(context.Context, CompletionRequest) (Completion, error)
}

// Artifacts contains only the outputs owned by preparation.
type Artifacts struct {
	Preparation *PreparationResult `json:"preparation,omitzero"`
}

// StepHandler executes one concrete preparation step.
type StepHandler interface {
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
	Source     imports.PreparationSource
	SourceRoot *os.Root
	Extractor  DocumentExtractor
	Completer  ModelCompleter
}
