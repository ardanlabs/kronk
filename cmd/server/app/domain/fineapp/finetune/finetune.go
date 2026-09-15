// Package finetune provides Moxie's product-facing training boundary.
package finetune

import (
	"context"
)

// Runtime identifies the local inference process used to evaluate candidates.
type Runtime struct {
	LlamaServer string `json:"llama_server"`
	Port        int    `json:"port"`
	ContextSize int    `json:"context_size"`
	GPULayers   int    `json:"gpu_layers"`
}

// Request contains user and platform selections for one training run. Model
// recipes intentionally remain behind Backend.
type Request struct {
	Dataset string  `json:"dataset"`
	Output  string  `json:"output"`
	Model   string  `json:"model"`
	Runtime Runtime `json:"runtime"`
}

// Stage identifies a user-understandable part of a training run.
type Stage string

const (
	StageImporting       Stage = "importing"
	StageValidating      Stage = "validating"
	StageTokenizing      Stage = "tokenizing"
	StageTraining        Stage = "training"
	StageFinalEvaluation Stage = "final_evaluation"
	StagePublishing      Stage = "publishing"
	StageComplete        Stage = "complete"
)

// Progress is safe for the desktop presentation layer to consume.
type Progress struct {
	Stage              Stage  `json:"stage"`
	Completed          int    `json:"completed"`
	Total              int    `json:"total"`
	Message            string `json:"message"`
	Resumable          bool   `json:"resumable,omitempty"`
	SelectedEpoch      int    `json:"selected_epoch,omitempty"`
	SelectedCheckpoint string `json:"selected_checkpoint,omitempty"`
}

// ProgressFunc receives synchronous progress updates from Backend.Run.
type ProgressFunc func(Progress)

// Status is the durable lifecycle status stored for Current Run and History.
type Status string

const (
	StatusPreparing        Status = "preparing"
	StatusTraining         Status = "training"
	StatusEvaluating       Status = "evaluating"
	StatusCompleted        Status = "completed"
	StatusNeedsImprovement Status = "needs_improvement"
	StatusFailed           Status = "failed"
	StatusStopped          Status = "stopped"
)

// Result identifies the durable outputs produced by a backend. Adapter remains
// empty when protected evaluation rejects a candidate.
type Result struct {
	Adapter            string `json:"adapter,omitempty"`
	Report             string `json:"report,omitempty"`
	Status             Status `json:"status,omitempty"`
	Error              string `json:"error,omitempty"`
	Resumable          bool   `json:"resumable,omitempty"`
	SelectedEpoch      int    `json:"selected_epoch,omitempty"`
	SelectedCheckpoint string `json:"selected_checkpoint,omitempty"`
}

// Backend is the model-neutral seam implemented by the real Gemma and Qwen
// training workflow. It has no Fyne dependency.
type Backend interface {
	Preflight(context.Context, Request) error
	Run(context.Context, Request, ProgressFunc) (Result, error)
}
