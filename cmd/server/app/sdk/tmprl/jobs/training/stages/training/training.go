// Package training owns training records, split policy, and computation contracts.
package training

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// ErrInvalidArgument identifies invalid training arguments.
var ErrInvalidArgument = tmprl.ErrInvalidArgument

// Record contains a prompt, target, and their provenance and split policy.
type Record struct {
	Prompt            string `json:"prompt"`
	Target            string `json:"target"`
	Evidence          string `json:"evidence"`
	FactID            string `json:"fact_id,omitzero"`
	SourceDocument    string `json:"source_document,omitzero"`
	SourceChunkID     string `json:"source_chunk_id,omitzero"`
	ValidationPrompt  bool   `json:"validation_prompt,omitzero"`
	IndependentPrompt bool   `json:"independent_prompt,omitzero"`
	UserApproved      bool   `json:"user_approved,omitzero"`
}

// Training configures model optimization.
type Training struct {
	MaximumEpochs int     `json:"maximum_epochs"`
	BatchSize     int     `json:"batch_size"`
	LearningRate  float64 `json:"learning_rate"`
	Seed          int64   `json:"seed"`
	LoRARank      int     `json:"lora_rank,omitzero"`
	LoRAAlpha     float64 `json:"lora_alpha,omitzero"`
}

// TrainingDataset contains locked splits. Test never enters optimization.
type TrainingDataset struct {
	Train      []Record `json:"train"`
	Validation []Record `json:"validation"`
	Test       []Record `json:"test"`
}

// TrainingJob supplies worker-local computation inputs and disposable storage.
type TrainingJob struct {
	ModelID  string          `json:"model_id"`
	Model    string          `json:"model"`
	Training Training        `json:"training"`
	Dataset  TrainingDataset `json:"dataset"`
	Workdir  string          `json:"workdir"`
	// PausePoint waits between computation units without discarding optimizer state.
	PausePoint func(context.Context) error `json:"-"`
}

// TrainingCandidate identifies a trained checkpoint and its selection metrics.
type TrainingCandidate struct {
	Path           string  `json:"path"`
	BestEpoch      int     `json:"best_epoch"`
	TrainingLoss   float64 `json:"training_loss"`
	ValidationLoss float64 `json:"validation_loss"`
}

// Trainer performs computation without owning orchestration or persistence.
// Predict loads the candidate when adapter is nonempty, or the frozen base
// otherwise, and reports the number of records answered so far to answered,
// which may be nil.
type Trainer interface {
	Validate(context.Context, TrainingJob) error
	Train(context.Context, TrainingJob, func(tmprl.Progress)) (TrainingCandidate, error)
	Predict(ctx context.Context, job TrainingJob, adapter string, records []Record, answered func(int)) ([]string, error)
}

// StatusNeedsImprovement ends training when evaluation rejects the candidate.
const StatusNeedsImprovement tmprl.Status = "needs_improvement"

// Artifacts contains the persisted outputs of training operations.
type Artifacts struct {
	Dataset    string `json:"dataset,omitzero"`
	Checkpoint string `json:"checkpoint,omitzero"`
	Report     string `json:"report,omitzero"`
	Adapter    string `json:"adapter,omitzero"`
	Accepted   bool   `json:"accepted"`
	// NotCompared records an adapter accepted without evaluation questions to
	// compare it with the base model.
	NotCompared bool `json:"not_compared,omitzero"`
}

// StepContext supplies resolved resources and incoming artifacts to a step.
// The caller owns workspace and scratch directory lifetimes.
type StepContext struct {
	Artifacts   Artifacts
	Workspace   *os.Root
	Trainer     Trainer
	Job         TrainingJob
	Records     []Record
	Fingerprint string
	Publish     func(tmprl.Progress)
}

// StepHandler executes one operation and preserves incoming artifacts.
type StepHandler interface {
	Execute(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)
}

// TrainingStep identifies an executable training operation, not a stage group.
type TrainingStep interface {
	StepHandler
	Name() string
}

// InvalidOutput rejects invalid step results without retrying computation.
func InvalidOutput(step string) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for training step %s", step), "invalid-step-output", nil)
}
