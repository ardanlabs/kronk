// Package finetune provides Moxie's product-facing training boundary.
package finetune

import (
	"context"
	"errors"
	"math"
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
	Dataset  string         `json:"dataset"`
	Output   string         `json:"output"`
	Model    string         `json:"model"`
	Runtime  Runtime        `json:"runtime"`
	Training TrainingConfig `json:"training"`
}

// TrainingConfig contains the user-adjustable settings for one run. The
// backend continues to own architecture-specific choices such as target
// modules, loss, padding, and checkpoint selection.
type TrainingConfig struct {
	MaximumEpochs         int     `json:"maximum_epochs"`
	LearningRate          float64 `json:"learning_rate"`
	BatchSize             int     `json:"batch_size"`
	LoRARank              int     `json:"lora_rank"`
	LoRAAlpha             float64 `json:"lora_alpha"`
	WeightDecay           float64 `json:"weight_decay"`
	EarlyStoppingPatience int     `json:"early_stopping_patience"`
	Seed                  int64   `json:"seed"`
}

// Validate checks the model-neutral bounds accepted by the training API.
func (config TrainingConfig) Validate() error {
	switch {
	case config.MaximumEpochs < 1 || config.MaximumEpochs > 100:
		return errors.New("maximum epochs must be between 1 and 100")
	case config.LearningRate <= 0 || config.LearningRate > 1 || math.IsNaN(config.LearningRate) || math.IsInf(config.LearningRate, 0):
		return errors.New("learning rate must be finite and in (0,1]")
	case config.BatchSize < 1 || config.BatchSize > 128:
		return errors.New("batch size must be between 1 and 128")
	case config.LoRARank < 1:
		return errors.New("LoRA rank must be greater than zero")
	case config.LoRAAlpha <= 0 || math.IsNaN(config.LoRAAlpha) || math.IsInf(config.LoRAAlpha, 0):
		return errors.New("LoRA alpha must be finite and greater than zero")
	case config.WeightDecay < 0 || config.WeightDecay > 1 || math.IsNaN(config.WeightDecay) || math.IsInf(config.WeightDecay, 0):
		return errors.New("weight decay must be finite and between 0 and 1")
	case config.EarlyStoppingPatience < 0 || config.EarlyStoppingPatience > config.MaximumEpochs:
		return errors.New("early stopping patience must be between 0 and maximum epochs")
	case config.Seed < 0:
		return errors.New("seed must not be negative")
	}
	return nil
}

// LoRARecipe describes the low-rank adapters trained by a recipe.
type LoRARecipe struct {
	Rank          int
	Alpha         float64
	Scale         float64
	TargetModules []string
}

// SequenceRecipe describes how tokenized examples are bounded and padded.
type SequenceRecipe struct {
	MaximumTokens int
	Source        string
	Overflow      string
	Padding       string
}

// OutputRecipe describes the adapter artifact produced by a recipe.
type OutputRecipe struct {
	Format    string
	Precision string
	Filename  string
}

// Recipe describes the backend-owned settings for a fine-tuning run.
type Recipe struct {
	Workflow                     string
	Method                       string
	BaseWeights                  string
	Optimizer                    string
	LearningRate                 float64
	WeightDecay                  float64
	BatchSize                    int
	MaximumEpochs                int
	Seed                         uint64
	LoRA                         LoRARecipe
	Sequence                     SequenceRecipe
	Loss                         string
	CheckpointSelection          string
	Output                       OutputRecipe
	EarlyStoppingPatience        int
	MinimumValidationImprovement float64
	TrainingPhases               int
	CandidateCount               int
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
