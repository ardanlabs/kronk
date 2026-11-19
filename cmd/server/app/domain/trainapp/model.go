package trainapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Upload identifies a stored input without exposing its server filesystem path.
// An upload is not necessarily a validated training dataset.
type Upload struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Format    string    `json:"format"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

// Encode implements web.Encoder.
func (app Upload) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

// NewTraining submits one job using an uploaded document, a prepared JSONL
// dataset, or a saved dataset version. The job owns model approval, dataset
// readiness, defaults, and training policy.
type NewTraining struct {
	ID              string              `json:"id"`
	PreparationOnly bool                `json:"preparation_only"`
	Model           string              `json:"model"`
	DatasetID       string              `json:"dataset_id"`
	Prepare         *PreparationOptions `json:"prepare,omitzero"`
	SavedDataset    *SavedDataset       `json:"saved_dataset,omitzero"`
	Training        TrainingOptions     `json:"training"`
}

// SavedDataset selects a saved dataset version to train on.
type SavedDataset struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// PreparationOptions selects a source document and its preparation model.
type PreparationOptions struct {
	DatasetID       string `json:"dataset_id"`
	TargetDatasetID string `json:"target_dataset_id,omitzero"`
	BaseVersion     int    `json:"base_version,omitzero"`
	Model           string `json:"model"`
	Goal            string `json:"goal"`
	Instructions    string `json:"instructions,omitzero"`
	PreviewRecords  int    `json:"preview_records,omitzero"`
	MaxQuestions    int    `json:"max_questions,omitzero"` // Zero leaves document preparation unlimited.
	PageStart       int    `json:"page_start,omitzero"`
	PageEnd         int    `json:"page_end,omitzero"`
}

// TrainingOptions carries user-selected settings without applying job policy.
type TrainingOptions struct {
	MaximumEpochs int     `json:"maximum_epochs"`
	BatchSize     int     `json:"batch_size"`
	LearningRate  float64 `json:"learning_rate"`
	Seed          int64   `json:"seed"`
	LoRARank      int     `json:"lora_rank"`
	LoRAAlpha     float64 `json:"lora_alpha"`
}

// Decode implements web.Decoder and accepts exactly one request object.
func (app *NewTraining) Decode(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(app); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func toJobRequest(app NewTraining) (training.Request, error) {
	var fields errs.FieldErrors
	if !app.PreparationOnly && strings.TrimSpace(app.Model) == "" {
		fields.Add("model", fmt.Errorf("is required"))
	}
	if app.Prepare == nil && app.SavedDataset == nil && strings.TrimSpace(app.DatasetID) == "" {
		fields.Add("dataset_id", fmt.Errorf("is required"))
	}
	if len(fields) != 0 {
		return training.Request{}, fields
	}
	request := training.Request{
		ID: app.ID, Model: app.Model, DatasetID: app.DatasetID, PreparationOnly: app.PreparationOnly,
		Training: optimization.Training{
			MaximumEpochs: app.Training.MaximumEpochs, BatchSize: app.Training.BatchSize,
			LearningRate: app.Training.LearningRate, Seed: app.Training.Seed,
			LoRARank: app.Training.LoRARank, LoRAAlpha: app.Training.LoRAAlpha,
		},
	}
	if saved := app.SavedDataset; saved != nil {
		request.SavedDataset = &training.SavedDataset{ID: saved.ID, Version: saved.Version}
	}
	if app.Prepare != nil {
		request.Prepare = &preparation.PreparationRequest{
			DatasetID: app.Prepare.DatasetID, Model: app.Prepare.Model,
			TargetDatasetID: app.Prepare.TargetDatasetID, BaseVersion: app.Prepare.BaseVersion,
			Goal: preparation.Goal(app.Prepare.Goal), Instructions: app.Prepare.Instructions,
			PreviewRecords: app.Prepare.PreviewRecords,
			MaxQuestions:   app.Prepare.MaxQuestions,
			PageStart:      app.Prepare.PageStart,
			PageEnd:        app.Prepare.PageEnd,
		}
	}
	return request, nil
}

// Runs is one visibility page. The continuation token is opaque to callers.
type Runs struct {
	Runs          []Run  `json:"runs"`
	NextPageToken string `json:"next_page_token,omitzero"`
	Revision      string `json:"revision"`
}

// Encode implements web.Encoder.
func (r Runs) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

// Run is the public state of one training execution. Internal requests,
// artifact paths, worker identities, and backend error messages are not exposed.
type Run struct {
	JobID                  string          `json:"job_id,omitzero"`
	AttemptID              string          `json:"attempt_id,omitzero"`
	ObservationRevision    int64           `json:"observation_revision,omitzero"`
	ControlRevision        int64           `json:"control_revision"`
	ControlAction          string          `json:"control_action,omitzero"`
	ControlStage           string          `json:"control_stage,omitzero"`
	ObservedAt             time.Time       `json:"observed_at,omitzero"`
	PendingActions         []PendingAction `json:"pending_actions,omitzero"`
	WorkflowID             string          `json:"workflow_id"`
	PreparationOnly        bool            `json:"preparation_only"`
	PreparedDatasetID      string          `json:"prepared_dataset_id,omitzero"`
	PreparedDatasetVersion int             `json:"prepared_dataset_version,omitzero"`
	TargetDatasetID        string          `json:"target_dataset_id,omitzero"`
	TargetDatasetName      string          `json:"target_dataset_name,omitzero"`
	BaseVersion            int             `json:"base_version,omitzero"`
	SourceDocument         string          `json:"source_document,omitzero"`
	Settings               RunSettings     `json:"settings,omitzero"`
	RunID                  string          `json:"run_id"`
	Status                 string          `json:"status"`
	Stage                  string          `json:"stage,omitzero"`
	Step                   string          `json:"step,omitzero"`
	StateStale             bool            `json:"state_stale"`
	StartedAt              time.Time       `json:"started_at"`
	UpdatedAt              time.Time       `json:"updated_at,omitzero"`
	ClosedAt               time.Time       `json:"closed_at,omitzero"`
	Stages                 []Stage         `json:"stages"`
	Progress               Progress        `json:"progress,omitzero"`
	Checkpoint             Checkpoint      `json:"checkpoint,omitzero"`
	Preview                Preview         `json:"preview,omitzero"`
	DatasetAvailable       bool            `json:"dataset_available"`
	DatasetRecordCount     int             `json:"dataset_record_count"`
	SkippedRecords         int             `json:"skipped_records,omitzero"`
	FailureReason          string          `json:"failure_reason,omitzero"`
	FailureLog             []FailureEntry  `json:"failure_log,omitempty"`
	Issues                 []RunIssue      `json:"issues,omitempty"`
	TrainingReady          bool            `json:"training_ready"`
	EvaluationAvailable    bool            `json:"evaluation_available"`
	AdapterAvailable       bool            `json:"adapter_available"`
	// A training run's dataset version, its settings (present only when the live
	// run is readable; the base model is not echoed), and its evaluation outcome:
	// accepted, not_compared, or needs_improvement once it finishes.
	TrainingDatasetID      string          `json:"training_dataset_id,omitzero"`
	TrainingDatasetName    string          `json:"training_dataset_name,omitzero"`
	TrainingDatasetVersion int             `json:"training_dataset_version,omitzero"`
	TrainingSettings       TrainingOptions `json:"training_settings,omitzero"`
	EvaluationResult       string          `json:"evaluation_result,omitzero"`
}

// FailureEntry exposes an allowlisted diagnostic, never a raw backend error.
type FailureEntry struct {
	Step    string    `json:"step"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Time    time.Time `json:"time,omitzero"`
	Attempt int32     `json:"attempt,omitzero"`
}

// RunIssue is a safe diagnostic count, not a backend error or log entry.
type RunIssue struct {
	Code     string `json:"code"`
	Count    int    `json:"count"`
	Required int    `json:"required,omitempty"`
}

// RunSettings echoes the preparation request. It is present only when the
// live run is readable; the persisted catalog does not keep the request.
type RunSettings struct {
	Model          string `json:"model"`
	Goal           string `json:"goal"`
	PreviewRecords int    `json:"preview_records,omitzero"`
	MaxQuestions   int    `json:"max_questions,omitzero"` // Zero is unlimited.
	PageStart      int    `json:"page_start,omitzero"`
	PageEnd        int    `json:"page_end,omitzero"`
	Instructions   string `json:"instructions,omitzero"`
}

type PendingAction struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	State string `json:"state"`
}

// PreparationControl is an immutable request against a workflow control revision,
// not the independently advancing catalog observation revision.
type PreparationControl struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (app *PreparationControl) Decode(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(app); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func toControlRequest(app PreparationControl, action string) (tmprl.ControlRequest, error) {
	request := tmprl.ControlRequest{RequestID: app.RequestID, ExpectedRevision: app.ExpectedRevision, Action: action}
	return request, request.Validate()
}

// ControlReceipt acknowledges command acceptance, never confirmed suspension.
type ControlReceipt struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Revision  int64  `json:"revision"`
}

func (app ControlReceipt) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppControlReceipt(receipt tmprl.ControlReceipt) ControlReceipt {
	return ControlReceipt{RequestID: receipt.RequestID, Action: receipt.Action, Revision: receipt.Revision}
}

// Evaluation exposes persisted scores, not a new acceptance calculation.
type Evaluation struct {
	ScoreKind      string             `json:"score_kind"`
	BaseCorrect    int                `json:"base_correct"`
	AdapterCorrect int                `json:"adapter_correct"`
	Total          int                `json:"total"`
	Accepted       bool               `json:"accepted"`
	Checks         []AcceptanceCheck  `json:"checks,omitempty"`
	Answers        []EvaluationAnswer `json:"answers"`
}

// AcceptanceCheck exposes the evaluator's persisted acceptance evidence.
type AcceptanceCheck struct {
	Code     string `json:"code"`
	Actual   int    `json:"actual"`
	Required int    `json:"required"`
	Passed   bool   `json:"passed"`
}

// EvaluationAnswer preserves report text without interpreting factual accuracy.
type EvaluationAnswer struct {
	Prompt        string `json:"prompt"`
	Expected      string `json:"expected"`
	BaseAnswer    string `json:"base_answer"`
	AdapterAnswer string `json:"adapter_answer"`
	AdapterPassed *bool  `json:"adapter_passed,omitempty"`
}

// Encode implements web.Encoder.
func (app Evaluation) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppEvaluation(report evaluate.EvaluationReport) Evaluation {
	result := Evaluation{
		ScoreKind: "normalized_exact_match", BaseCorrect: report.BaseCorrect,
		AdapterCorrect: report.AdapterCorrect, Total: report.Total, Accepted: report.Accepted,
		Answers: make([]EvaluationAnswer, len(report.Answers)),
	}
	for i, answer := range report.Answers {
		result.Answers[i] = EvaluationAnswer{
			Prompt: answer.Prompt, Expected: answer.Expected,
			BaseAnswer: answer.BaseAnswer, AdapterAnswer: answer.AdapterAnswer, AdapterPassed: answer.AdapterPassed,
		}
	}
	for _, check := range report.Checks {
		result.Checks = append(result.Checks, AcceptanceCheck{Code: check.Code, Actual: check.Actual, Required: check.Required, Passed: check.Passed})
	}
	return result
}

// Dataset exposes preparation assets without filesystem paths or record bodies.
type Dataset struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       int               `json:"version"`
	RecordCount   int               `json:"record_count"`
	TrainingReady bool              `json:"training_ready"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Versions      []int             `json:"versions"`
	Documents     []DatasetDocument `json:"documents"`
	// Training records and the evaluation questions the dataset holds and needs.
	TrainingRecords     int `json:"training_records"`
	ValidationQuestions int `json:"validation_questions"`
	ValidationNeeded    int `json:"validation_needed"`
	TestQuestions       int `json:"test_questions"`
	TestNeeded          int `json:"test_needed"`
}

type DatasetDocument struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RecordCount int    `json:"record_count"`
	PageStart   int    `json:"page_start"`
	PageEnd     int    `json:"page_end"`
}

func (d Dataset) Encode() ([]byte, string, error) {
	data, err := json.Marshal(d)
	return data, "application/json", err
}

type Datasets struct {
	Datasets []Dataset `json:"datasets"`
}

func (d Datasets) Encode() ([]byte, string, error) {
	data, err := json.Marshal(d)
	return data, "application/json", err
}

type NewDataset struct {
	Name string `json:"name"`
}

func (d *NewDataset) Decode(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(d); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body must contain one dataset object")
	}
	return nil
}

func toAppDataset(d datasetstore.Dataset) Dataset {
	result := Dataset{ID: d.ID, Name: d.Name, Version: d.Version, RecordCount: d.RecordCount, TrainingReady: d.TrainingReady,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Versions: slices.Clone(d.Versions), Documents: make([]DatasetDocument, len(d.Documents)),
		TrainingRecords: d.TrainingRecords, ValidationQuestions: d.ValidationQuestions, ValidationNeeded: d.ValidationNeeded, TestQuestions: d.TestQuestions, TestNeeded: d.TestNeeded}
	for i, document := range d.Documents {
		result.Documents[i] = DatasetDocument{ID: document.ID, Name: document.Name, RecordCount: document.RecordCount, PageStart: document.PageStart, PageEnd: document.PageEnd}
	}
	return result
}

// Checkpoint identifies the pending SDK decision, not an additional stage or step.
type Checkpoint struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage"`
	Step  string `json:"step,omitzero"`
}

// Preview contains saved examples and verification warnings for user review.
type Preview struct {
	Records  []PreviewRecord `json:"records"`
	Warnings []string        `json:"warnings,omitzero"`
}

// PreviewRecord exposes editable text and its retained source association.
type PreviewRecord struct {
	Prompt         string `json:"prompt"`
	Target         string `json:"target"`
	Evidence       string `json:"evidence"`
	SourceDocument string `json:"source_document"`
}

// ContinuePreview submits edits for the exact pending Preview checkpoint.
type ContinuePreview struct {
	CheckpointID string          `json:"checkpoint_id"`
	SubmissionID string          `json:"submission_id"`
	Records      []PreviewRecord `json:"records"`
}

// Decode accepts one review object without internal approval or split fields.
func (app *ContinuePreview) Decode(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(app); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func toPreviewDecision(app ContinuePreview) (tmprl.Decision, error) {
	review := preparation.Review{Records: make([]optimization.Record, len(app.Records))}
	for i, record := range app.Records {
		review.Records[i] = optimization.Record{
			Prompt: record.Prompt, Target: record.Target,
			Evidence: record.Evidence, SourceDocument: record.SourceDocument,
		}
	}
	payload, err := json.Marshal(review)
	if err != nil {
		return tmprl.Decision{}, err
	}
	return tmprl.Decision{CheckpointID: app.CheckpointID, SubmissionID: app.SubmissionID, Payload: payload}, nil
}

// StartTraining authorizes Training after this run's Preparation has completed.
type StartTraining struct {
	CheckpointID string `json:"checkpoint_id"`
	SubmissionID string `json:"submission_id"`
}

// Decode accepts one authorization object without records or training settings.
func (app *StartTraining) Decode(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(app); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

// DecisionReceipt acknowledges accepted input, not completed preparation or training.
type DecisionReceipt struct {
	CheckpointID string `json:"checkpoint_id"`
	SubmissionID string `json:"submission_id"`
}

// Encode implements web.Encoder without exposing the SDK's decision payload.
func (r DecisionReceipt) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

func toAppDecisionReceipt(receipt tmprl.Decision) DecisionReceipt {
	return DecisionReceipt{CheckpointID: receipt.CheckpointID, SubmissionID: receipt.SubmissionID}
}

// Stage contains the ordered step states reported by the runner.
type Stage struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitzero"`
	ClosedAt  time.Time `json:"closed_at,omitzero"`
	Deadline  time.Time `json:"deadline,omitzero"`
	Steps     []Step    `json:"steps"`
}

// Step describes execution state without reproducing runner policy.
type Step struct {
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	Attempt         int32     `json:"attempt"`
	StartedAt       time.Time `json:"started_at,omitzero"`
	ClosedAt        time.Time `json:"closed_at,omitzero"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitzero"`
}

// Progress contains backend-reported counts, not an estimated percentage.
type Progress struct {
	Stage            string `json:"stage"`
	Step             string `json:"step"`
	Completed        int    `json:"completed"`
	Total            int    `json:"total"`
	RemainingSeconds int64  `json:"remaining_seconds,omitzero"`
}

// Encode implements web.Encoder.
func (r Run) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

func toAppRun(run datasets.State) Run {
	result := Run{
		JobID: run.JobID, AttemptID: run.AttemptID, ObservationRevision: run.ObservationRevision, ObservedAt: run.ObservedAt,
		ControlRevision: run.ControlRevision, ControlAction: run.ControlAction,
		ControlStage: run.ControlStage,
		WorkflowID:   run.WorkflowID, RunID: run.RunID, Status: string(run.Status),
		PreparationOnly: run.Request.PreparationOnly,
		Stage:           string(run.Stage), Step: run.Step, StateStale: run.StateStale,
		StartedAt: run.StartedAt, UpdatedAt: run.UpdatedAt, ClosedAt: run.ClosedAt,
		Stages:              make([]Stage, len(run.Stages)),
		EvaluationAvailable: run.EvaluationAvailable, AdapterAvailable: run.AdapterAvailable,
		DatasetAvailable: run.DatasetAvailable, DatasetRecordCount: run.DatasetRecordCount,
		SkippedRecords: run.SkippedRecordCount, FailureReason: run.FailureReason,
		TrainingReady:     run.TrainingReady,
		PreparedDatasetID: run.PreparedDatasetID, PreparedDatasetVersion: run.PreparedDatasetVersion,
		TargetDatasetID: run.TargetDatasetID, TargetDatasetName: run.TargetDatasetName,
		BaseVersion: run.BaseVersion, SourceDocument: run.SourceDocument,
	}
	for _, issue := range run.Issues {
		result.Issues = append(result.Issues, RunIssue{Code: issue.Code, Count: issue.Count, Required: issue.Required})
	}
	for _, entry := range run.FailureLog {
		result.FailureLog = append(result.FailureLog, FailureEntry{Step: entry.Step, Code: entry.Code, Message: entry.Message, Time: entry.Time, Attempt: entry.Attempt})
	}
	if run.Mode == "training" {
		// A training run names the dataset version it trains on, not one it adds to.
		result.TrainingDatasetID, result.TrainingDatasetName, result.TrainingDatasetVersion = run.TargetDatasetID, run.TargetDatasetName, run.BaseVersion
		result.TargetDatasetID, result.TargetDatasetName, result.BaseVersion = "", "", 0
		t := run.Request.Training
		result.TrainingSettings = TrainingOptions{
			MaximumEpochs: t.MaximumEpochs, BatchSize: t.BatchSize, LearningRate: t.LearningRate,
			Seed: t.Seed, LoRARank: t.LoRARank, LoRAAlpha: t.LoRAAlpha,
		}
		switch run.Status {
		case optimization.StatusNeedsImprovement:
			result.EvaluationResult = "needs_improvement"
		case tmprl.StatusCompleted:
			result.EvaluationResult = "accepted"
			if run.NotCompared {
				result.EvaluationResult = "not_compared"
			}
		}
	}
	if prepare := run.Request.Prepare; prepare != nil {
		result.Settings = RunSettings{
			Model: prepare.Model, Goal: string(prepare.Goal), PreviewRecords: prepare.PreviewRecords,
			MaxQuestions: prepare.MaxQuestions, PageStart: prepare.PageStart, PageEnd: prepare.PageEnd,
			Instructions: prepare.Instructions,
		}
	}
	for _, action := range run.PendingActions {
		result.PendingActions = append(result.PendingActions, PendingAction{ID: action.ID, Kind: action.Kind, State: action.State})
	}
	for i, stage := range run.Stages {
		result.Stages[i] = Stage{
			Name: string(stage.Name), Status: string(stage.Status),
			StartedAt: stage.StartedAt, ClosedAt: stage.ClosedAt, Deadline: stage.Deadline,
			Steps: make([]Step, len(stage.Steps)),
		}
		for k, step := range stage.Steps {
			result.Stages[i].Steps[k] = Step{
				Name: step.Name, Status: string(step.Status), Attempt: step.Attempt,
				StartedAt: step.StartedAt, ClosedAt: step.ClosedAt, LastHeartbeatAt: step.LastHeartbeatAt,
			}
		}
	}
	if run.Progress != nil {
		result.Progress = Progress{
			Stage: string(run.Progress.Stage), Step: run.Progress.Step,
			Completed: run.Progress.Completed, Total: run.Progress.Total,
		}
		result.Progress.RemainingSeconds = run.RemainingSeconds
	}
	if run.Checkpoint != nil {
		result.Checkpoint = Checkpoint{
			ID: run.Checkpoint.ID, Name: run.Checkpoint.Name,
			Stage: run.Checkpoint.Stage, Step: run.Checkpoint.Step,
		}
	}
	if preview := run.Artifacts.Preview; preview != nil {
		result.Preview = Preview{Records: make([]PreviewRecord, len(preview.Preview)), Warnings: slices.Clone(preview.Warnings)}
		for i, record := range preview.Preview {
			result.Preview.Records[i] = PreviewRecord{
				Prompt: record.Prompt, Target: record.Target,
				Evidence: record.Evidence, SourceDocument: record.SourceDocument,
			}
		}
	}
	return result
}
