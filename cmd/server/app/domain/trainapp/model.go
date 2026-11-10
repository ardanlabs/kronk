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

// NewTraining submits one job using an uploaded document or prepared JSONL dataset.
// The job owns model approval, dataset readiness, defaults, and training policy.
type NewTraining struct {
	ID        string              `json:"id"`
	Model     string              `json:"model"`
	DatasetID string              `json:"dataset_id"`
	Prepare   *PreparationOptions `json:"prepare,omitzero"`
	Training  TrainingOptions     `json:"training"`
}

// PreparationOptions selects a source document and its preparation model.
type PreparationOptions struct {
	DatasetID      string `json:"dataset_id"`
	Model          string `json:"model"`
	Goal           string `json:"goal"`
	Instructions   string `json:"instructions,omitzero"`
	PreviewRecords int    `json:"preview_records,omitzero"`
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
	if strings.TrimSpace(app.Model) == "" {
		fields.Add("model", fmt.Errorf("is required"))
	}
	if app.Prepare == nil && strings.TrimSpace(app.DatasetID) == "" {
		fields.Add("dataset_id", fmt.Errorf("is required"))
	}
	if len(fields) != 0 {
		return training.Request{}, fields
	}
	request := training.Request{
		ID: app.ID, Model: app.Model, DatasetID: app.DatasetID,
		Training: optimization.Training{
			MaximumEpochs: app.Training.MaximumEpochs, BatchSize: app.Training.BatchSize,
			LearningRate: app.Training.LearningRate, Seed: app.Training.Seed,
			LoRARank: app.Training.LoRARank, LoRAAlpha: app.Training.LoRAAlpha,
		},
	}
	if app.Prepare != nil {
		request.Prepare = &preparation.PreparationRequest{
			DatasetID: app.Prepare.DatasetID, Model: app.Prepare.Model,
			Goal: preparation.Goal(app.Prepare.Goal), Instructions: app.Prepare.Instructions,
			PreviewRecords: app.Prepare.PreviewRecords,
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
	WorkflowID          string     `json:"workflow_id"`
	RunID               string     `json:"run_id"`
	Status              string     `json:"status"`
	Stage               string     `json:"stage,omitzero"`
	Step                string     `json:"step,omitzero"`
	StateStale          bool       `json:"state_stale"`
	StartedAt           time.Time  `json:"started_at"`
	UpdatedAt           time.Time  `json:"updated_at,omitzero"`
	ClosedAt            time.Time  `json:"closed_at,omitzero"`
	Stages              []Stage    `json:"stages"`
	Progress            Progress   `json:"progress,omitzero"`
	Checkpoint          Checkpoint `json:"checkpoint,omitzero"`
	Preview             Preview    `json:"preview,omitzero"`
	EvaluationAvailable bool       `json:"evaluation_available"`
	AdapterAvailable    bool       `json:"adapter_available"`
}

// Evaluation exposes persisted scores, not a new acceptance calculation.
type Evaluation struct {
	ScoreKind      string             `json:"score_kind"`
	BaseCorrect    int                `json:"base_correct"`
	AdapterCorrect int                `json:"adapter_correct"`
	Total          int                `json:"total"`
	Accepted       bool               `json:"accepted"`
	Answers        []EvaluationAnswer `json:"answers"`
}

// EvaluationAnswer preserves report text without interpreting factual accuracy.
type EvaluationAnswer struct {
	Prompt        string `json:"prompt"`
	Expected      string `json:"expected"`
	BaseAnswer    string `json:"base_answer"`
	AdapterAnswer string `json:"adapter_answer"`
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
			BaseAnswer: answer.BaseAnswer, AdapterAnswer: answer.AdapterAnswer,
		}
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
	Stage     string `json:"stage"`
	Step      string `json:"step"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

// Encode implements web.Encoder.
func (r Run) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

func toAppRun(run tmprl.Run[training.Request, training.Artifacts]) Run {
	result := Run{
		WorkflowID: run.WorkflowID, RunID: run.RunID, Status: string(run.Status),
		Stage: string(run.Stage), Step: run.Step, StateStale: run.StateStale,
		StartedAt: run.StartedAt, UpdatedAt: run.UpdatedAt, ClosedAt: run.ClosedAt,
		Stages:              make([]Stage, len(run.Stages)),
		EvaluationAvailable: run.Artifacts.Training.Report != "",
		AdapterAvailable: run.Status == tmprl.StatusCompleted && run.Artifacts.Training.Accepted &&
			run.Artifacts.Training.Report != "" && run.Artifacts.Training.Adapter != "",
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
