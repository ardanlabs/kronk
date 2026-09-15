package fineapp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

// DatasetResponse identifies a stored document without exposing server paths.
type DatasetResponse struct {
	DatasetID  string `json:"dataset_id"`
	SourceName string `json:"source_name"`
}

// PreparationModelsResponse lists the models allowed to prepare datasets.
type PreparationModelsResponse struct {
	Models []string `json:"models"`
}

// PreviewRequest contains the user selections for a preview build.
type PreviewRequest struct {
	DatasetID   string `json:"dataset_id"`
	Goal        string `json:"goal"`
	ModelID     string `json:"model_id"`
	RecordCount int    `json:"record_count"`
}

// Decode implements the web decoder interface.
func (req *PreviewRequest) Decode(data []byte) error {
	return json.Unmarshal(data, req)
}

// Validate checks the preview request at the HTTP boundary.
func (req *PreviewRequest) Validate() error {
	if err := validateResourceID(req.DatasetID); err != nil {
		return fmt.Errorf("dataset_id %w", err)
	}
	if req.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}
	if _, err := goalToBusiness(req.Goal); err != nil {
		return err
	}
	if req.RecordCount < 1 || req.RecordCount > prepare.MaxPreviewRecords {
		return fmt.Errorf("record_count must be between 1 and %d", prepare.MaxPreviewRecords)
	}
	return nil
}

// Record represents one editable prepared training example.
type Record struct {
	Prompt            string `json:"prompt"`
	Target            string `json:"target"`
	Evidence          string `json:"evidence"`
	FactID            string `json:"fact_id,omitempty"`
	IndependentPrompt bool   `json:"independent_prompt,omitempty"`
}

// Verification represents the grounding result for one preview record.
type Verification struct {
	RecordNumber   int    `json:"record_number"`
	Passed         bool   `json:"passed"`
	ExpectedTarget string `json:"expected_target"`
	Reason         string `json:"reason"`
}

// PreviewResponse contains the reviewable records and generation usage.
type PreviewResponse struct {
	DatasetID            string         `json:"dataset_id"`
	Records              []Record       `json:"records"`
	Verification         []Verification `json:"verification"`
	GenerationAttempts   int            `json:"generation_attempts"`
	VerificationAttempts int            `json:"verification_attempts"`
	PromptTokens         int            `json:"prompt_tokens"`
	CompletionTokens     int            `json:"completion_tokens"`
}

// BuildRequest contains the reviewed preview used to build the full dataset.
type BuildRequest struct {
	DatasetID              string   `json:"dataset_id"`
	Goal                   string   `json:"goal"`
	ModelID                string   `json:"model_id"`
	ReviewedRecords        []Record `json:"reviewed_records"`
	VerifiedPreviewRecords []Record `json:"verified_preview_records"`
}

// Decode implements the web decoder interface.
func (req *BuildRequest) Decode(data []byte) error {
	return json.Unmarshal(data, req)
}

// Validate checks the full-build request at the HTTP boundary.
func (req *BuildRequest) Validate() error {
	if err := validateResourceID(req.DatasetID); err != nil {
		return fmt.Errorf("dataset_id %w", err)
	}
	if req.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}
	if _, err := goalToBusiness(req.Goal); err != nil {
		return err
	}
	if len(req.ReviewedRecords) == 0 {
		return fmt.Errorf("reviewed_records is required")
	}
	return nil
}

// BuildResponse reports the durable dataset build without exposing file paths.
type BuildResponse struct {
	DatasetID            string `json:"dataset_id"`
	RecordCount          int    `json:"record_count"`
	ReviewedRecordCount  int    `json:"reviewed_record_count"`
	GeneratedRecordCount int    `json:"generated_record_count"`
	GenerationAttempts   int    `json:"generation_attempts"`
	VerificationAttempts int    `json:"verification_attempts"`
	PromptTokens         int    `json:"prompt_tokens"`
	CompletionTokens     int    `json:"completion_tokens"`
	ExcludedEvidence     int    `json:"excluded_evidence"`
}

// PreparationProgressResponse reports the current server-owned preparation operation.
type PreparationProgressResponse struct {
	Operation       string    `json:"operation"`
	Status          string    `json:"status"`
	Stage           string    `json:"stage"`
	Completed       int       `json:"completed"`
	Total           int       `json:"total"`
	AcceptedRecords int       `json:"accepted_records"`
	Error           string    `json:"error,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// StartRunRequest identifies managed resources used for one training run.
type StartRunRequest struct {
	DatasetID string `json:"dataset_id"`
	ModelID   string `json:"model_id"`
}

// Decode implements the web decoder interface.
func (req *StartRunRequest) Decode(data []byte) error {
	return json.Unmarshal(data, req)
}

// Validate checks the training request at the HTTP boundary.
func (req *StartRunRequest) Validate() error {
	if err := validateResourceID(req.DatasetID); err != nil {
		return fmt.Errorf("dataset_id %w", err)
	}
	if req.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}
	return nil
}

// ProgressResponse is the public representation of run progress.
type ProgressResponse struct {
	Stage         string `json:"stage"`
	Completed     int    `json:"completed"`
	Total         int    `json:"total"`
	Message       string `json:"message"`
	Resumable     bool   `json:"resumable,omitempty"`
	SelectedEpoch int    `json:"selected_epoch,omitempty"`
}

// RunResponse is the public representation of a fine-tuning run.
type RunResponse struct {
	ID            string           `json:"id"`
	DatasetID     string           `json:"dataset_id"`
	Model         string           `json:"model"`
	Status        string           `json:"status"`
	Progress      ProgressResponse `json:"progress"`
	Error         string           `json:"error,omitempty"`
	Resumable     bool             `json:"resumable,omitempty"`
	HasAdapter    bool             `json:"has_adapter"`
	HasReport     bool             `json:"has_report"`
	SelectedEpoch int              `json:"selected_epoch,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	FinishedAt    time.Time        `json:"finished_at"`
}

// HistoryResponse contains durable runs in newest-first order.
type HistoryResponse struct {
	Runs []RunResponse `json:"runs"`
}

// ActionResponse acknowledges a run action that completes asynchronously.
type ActionResponse struct {
	Status string `json:"status"`
}

type jsonResponse struct {
	value any
}

func (resp jsonResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(resp.value)
	return data, "application/json", err
}

func goalToBusiness(value string) (prepare.Goal, error) {
	goal := prepare.Goal(value)
	switch goal {
	case prepare.GoalQuestionAnswer, prepare.GoalClassification, prepare.GoalNumericPrediction, prepare.GoalTextGeneration:
		return goal, nil
	default:
		return "", fmt.Errorf("goal must be one of question_answer, classification, numeric_prediction, or text_generation")
	}
}

func recordsToBusiness(records []Record) []prepare.Record {
	converted := make([]prepare.Record, len(records))
	for i, record := range records {
		converted[i] = prepare.Record{
			Prompt:            record.Prompt,
			Target:            record.Target,
			Evidence:          record.Evidence,
			FactID:            record.FactID,
			IndependentPrompt: record.IndependentPrompt,
		}
	}
	return converted
}

func recordsFromBusiness(records []prepare.Record) []Record {
	converted := make([]Record, len(records))
	for i, record := range records {
		converted[i] = Record{
			Prompt:            record.Prompt,
			Target:            record.Target,
			Evidence:          record.Evidence,
			FactID:            record.FactID,
			IndependentPrompt: record.IndependentPrompt,
		}
	}
	return converted
}

func verificationFromBusiness(results []prepare.VerificationResult) []Verification {
	converted := make([]Verification, len(results))
	for i, result := range results {
		converted[i] = Verification{
			RecordNumber:   result.RecordNumber,
			Passed:         result.Passed,
			ExpectedTarget: result.ExpectedTarget,
			Reason:         result.Reason,
		}
	}
	return converted
}

func runFromBusiness(run finetune.RunInfo) RunResponse {
	datasetID := filepath.Base(filepath.Dir(run.Dataset))
	model := strings.TrimSuffix(filepath.Base(run.Model), filepath.Ext(run.Model))
	return RunResponse{
		ID:        run.ID,
		DatasetID: datasetID,
		Model:     model,
		Status:    string(run.Status),
		Progress: ProgressResponse{
			Stage:         string(run.Progress.Stage),
			Completed:     run.Progress.Completed,
			Total:         run.Progress.Total,
			Message:       run.Progress.Message,
			Resumable:     run.Progress.Resumable,
			SelectedEpoch: run.Progress.SelectedEpoch,
		},
		Error:         run.Error,
		Resumable:     run.Resumable,
		HasAdapter:    run.Adapter != "",
		HasReport:     run.Report != "",
		SelectedEpoch: run.SelectedEpoch,
		StartedAt:     run.StartedAt,
		UpdatedAt:     run.UpdatedAt,
		FinishedAt:    run.FinishedAt,
	}
}

func runsFromBusiness(runs []finetune.RunInfo) []RunResponse {
	converted := make([]RunResponse, len(runs))
	for i, run := range runs {
		converted[i] = runFromBusiness(run)
	}
	return converted
}
