// Package tuning owns headless fine-tuning orchestration and progress.
package tuning

import (
	"context"
	"encoding/json"
	"time"
)

// These models are SDK contracts, not HTTP models. JSON tags also define the
// payloads persisted by Temporal's default data converter. Keep them compatible
// with histories created by older workers when evolving the package.
type Request struct {
	ID       string   `json:"id"`
	Sources  []string `json:"sources"`
	Model    string   `json:"model"`
	Output   string   `json:"output"`
	Training Training `json:"training"`
	// Set by StartTraining; legacy Start requests continue using Sources.
	DatasetID   string    `json:"dataset_id,omitzero"`
	Preparation Execution `json:"preparation,omitzero"`
}

type Training struct {
	MaximumEpochs int     `json:"maximum_epochs"`
	BatchSize     int     `json:"batch_size"`
	LearningRate  float64 `json:"learning_rate"`
	Seed          int64   `json:"seed"`
	LoRARank      int     `json:"lora_rank,omitzero"`
	LoRAAlpha     float64 `json:"lora_alpha,omitzero"`
}

// TrainingRequest selects SDK-managed prepared data and a host-approved model.
// Exactly one of DatasetID or Preparation is required. No storage paths are
// accepted. The SDK owns the output directory and uses the existing run APIs.
type TrainingRequest struct {
	ID          string    `json:"id,omitzero"`
	DatasetID   string    `json:"dataset_id,omitzero"`
	Preparation Execution `json:"preparation,omitzero"`
	Model       string    `json:"model"`
	Training    Training  `json:"training"`
}

// TrainingDataset contains the locked splits. Test is never passed to GoMLX
// tokenization, optimization, or checkpoint selection.
type TrainingDataset struct {
	Train      []Record `json:"train"`
	Validation []Record `json:"validation"`
	Test       []Record `json:"test"`
}

// TrainingJob is worker-local, not a workflow payload. Workdir is disposable;
// returned candidates are copied into SDK-managed storage before committing.
type TrainingJob struct {
	Model    string          `json:"model"`
	Training Training        `json:"training"`
	Dataset  TrainingDataset `json:"dataset"`
	Workdir  string          `json:"workdir"`
}

type TrainingCandidate struct {
	Path           string  `json:"path"`
	BestEpoch      int     `json:"best_epoch"`
	TrainingLoss   float64 `json:"training_loss"`
	ValidationLoss float64 `json:"validation_loss"`
}

// EvaluationReport locks the MVP policy: normalized exact answers, at least
// 90% protected-test accuracy, and no regression against the frozen base.
type EvaluationReport struct {
	BaseCorrect    int                `json:"base_correct"`
	AdapterCorrect int                `json:"adapter_correct"`
	Total          int                `json:"total"`
	Accepted       bool               `json:"accepted"`
	Answers        []EvaluationAnswer `json:"answers,omitzero"`
}

// EvaluationAnswer preserves the actual outputs, not just normalized scores.
type EvaluationAnswer struct {
	Prompt        string `json:"prompt"`
	Expected      string `json:"expected"`
	BaseAnswer    string `json:"base_answer"`
	AdapterAnswer string `json:"adapter_answer"`
}

// Trainer performs computation only. The SDK owns input validation, stage
// orchestration, acceptance policy, persistence, and publication. Predict must
// load the candidate when adapter is nonempty, or the frozen base otherwise.
type Trainer interface {
	Train(context.Context, TrainingJob, func(Progress)) (TrainingCandidate, error)
	Predict(context.Context, TrainingJob, string, []Record) ([]string, error)
}

type Execution struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
}

type Stage string

const (
	StagePrepare Stage = "prepare"
	// StageSplit locks the training/validation/test splits of prepared records;
	// it does not extract documents or generate examples. The serialized value
	// stays "prepare" for compatibility with existing Temporal histories and
	// generic backends that use StagePrepare for their first stage.
	StageSplit    Stage = StagePrepare
	StageTrain    Stage = "train"
	StageEvaluate Stage = "evaluate"
	StagePublish  Stage = "publish"
)

type Status string

const (
	StatusRunning          Status = "running"
	StatusPauseRequested   Status = "pause_requested"
	StatusPaused           Status = "paused"
	StatusCompleted        Status = "completed"
	StatusNeedsImprovement Status = "needs_improvement"
	StatusStopped          Status = "stopped"
	StatusFailed           Status = "failed"
)

type Artifacts struct {
	Dataset    string `json:"dataset,omitzero"`
	Checkpoint string `json:"checkpoint,omitzero"`
	Report     string `json:"report,omitzero"`
	Adapter    string `json:"adapter,omitzero"`
	Accepted   bool   `json:"accepted"`
}

type Run struct {
	Execution
	Request   Request   `json:"request"`
	Status    Status    `json:"status"`
	Stage     Stage     `json:"stage,omitzero"`
	Artifacts Artifacts `json:"artifacts"`
	Progress  *Progress `json:"progress,omitzero"`
	StartedAt time.Time `json:"started_at"`
	ClosedAt  time.Time `json:"closed_at,omitzero"`
	Error     string    `json:"error,omitzero"`
}

type Progress struct {
	Execution
	Stage     Stage  `json:"stage"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
}

// StageRequest contains all durable inputs to one backend operation. The key
// identifies the logical operation; a backend should use it to recognize work
// already committed if an activity is ever retried by an operator.
type StageRequest struct {
	Execution
	Key       string    `json:"key"`
	Stage     Stage     `json:"stage"`
	Request   Request   `json:"request"`
	Artifacts Artifacts `json:"artifacts"`
}

type ImportInput struct {
	Name string `json:"name"`
}

// Dataset identifies an immutable SDK-managed import. Raw bytes and streams
// never become Temporal payloads. Workers must share the configured storage.
type Dataset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Format    string    `json:"format"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	Records   int       `json:"records,omitzero"`
	CreatedAt time.Time `json:"created_at"`
}

type Goal string

const (
	GoalQuestionAnswer    Goal = "question_answer"
	GoalClassification    Goal = "classification"
	GoalNumericPrediction Goal = "numeric_prediction"
	GoalTextGeneration    Goal = "text_generation"
)

type Record struct {
	Prompt         string `json:"prompt"`
	Target         string `json:"target"`
	Evidence       string `json:"evidence"`
	FactID         string `json:"fact_id,omitzero"`
	SourceDocument string `json:"source_document,omitzero"`
	// Optional imported provenance; preparation does not assign chunk IDs yet.
	SourceChunkID     string `json:"source_chunk_id,omitzero"`
	ValidationPrompt  bool   `json:"validation_prompt,omitzero"`
	IndependentPrompt bool   `json:"independent_prompt,omitzero"`
	UserApproved      bool   `json:"user_approved,omitzero"`
}

type PreparationKind string

const (
	PreparationPreview PreparationKind = "preview"
	PreparationFull    PreparationKind = "full"
)

// PreparationRequest refers to an imported document, never an arbitrary path.
// PreviewRecords is used only by preview jobs; reviewed examples by full jobs.
type PreparationRequest struct {
	ID              string   `json:"id"`
	DatasetID       string   `json:"dataset_id"`
	Model           string   `json:"model"`
	Goal            Goal     `json:"goal"`
	Instructions    string   `json:"instructions,omitzero"`
	PreviewRecords  int      `json:"preview_records,omitzero"`
	ReviewedRecords []Record `json:"reviewed_records,omitzero"`
}

type PreparationResult struct {
	DatasetID   string `json:"dataset_id"`
	RecordsPath string `json:"records_path"`
	ReportPath  string `json:"report_path"`
	// Reserved serialized field; current preparation does not produce spare rows.
	SpareRecordsPath string `json:"spare_records_path,omitzero"`
	RecordCount      int    `json:"record_count"`
	// Only bounded preview rows are returned inline; full records remain in storage.
	Preview          []Record `json:"preview,omitzero"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TrainingReady    bool     `json:"training_ready"`
	Warnings         []string `json:"warnings,omitzero"`
}

type PreparationRun struct {
	Execution
	Kind      PreparationKind    `json:"kind"`
	Request   PreparationRequest `json:"request"`
	Status    Status             `json:"status"`
	Result    *PreparationResult `json:"result,omitzero"`
	Progress  *Progress          `json:"progress,omitzero"`
	StartedAt time.Time          `json:"started_at"`
	ClosedAt  time.Time          `json:"closed_at,omitzero"`
	Error     string             `json:"error,omitzero"`
}

// preparationInput preserves the durable preparation identity. Timeout
// participates in checkpoint identity. Submission context cancellation does not
// stop an already-started workflow; StopPreparation requests durable cancellation.
type preparationInput struct {
	Request PreparationRequest `json:"request"`
	Kind    PreparationKind    `json:"kind"`
	Timeout time.Duration      `json:"timeout"`
}

// PreparationSource is a verified SDK import resolved on the worker, not an
// arbitrary filesystem path supplied by an app.
type PreparationSource struct {
	ID     string `json:"id"`
	Format string `json:"format"`
	SHA256 string `json:"sha256"`
}

type Document struct {
	Markdown       string          `json:"markdown"`
	StructuredJSON json.RawMessage `json:"structured_json"`
	ProcessingTime float64         `json:"processing_time"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseSchema map[string]any `json:"response_schema"`
	MaxTokens      int            `json:"max_tokens"`
	Seed           uint32         `json:"seed"`
}

type Completion struct {
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

type DocumentExtractor interface {
	Convert(context.Context, string) (Document, error)
	ExtractionProfile() string
}

type ModelCompleter interface {
	Complete(context.Context, CompletionRequest) (Completion, error)
}
