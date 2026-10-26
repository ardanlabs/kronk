package model

import (
	"context"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// JobRequest selects preparation followed by training in one worker reservation.
// Set Prepare for a document, or DatasetID for already-prepared JSONL, or
// Preparation for an existing completed preparation execution. Select exactly one.
type JobRequest struct {
	ID          string              `json:"id,omitzero"`
	Model       string              `json:"model"`
	Training    Training            `json:"training"`
	Prepare     *PreparationRequest `json:"prepare,omitzero"`
	DatasetID   string              `json:"dataset_id,omitzero"`
	Preparation tmprl.Execution     `json:"preparation,omitzero"`
}

// StepHandler implements one registered operation. The job definition owns
// ordering and execution policy; handlers own computation and artifact validation.
// Implementations must honor cancellation and deduplicate by Request.Key.
type StepHandler interface {
	Execute(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)
}

// StepContext supplies the step identity, confined workspace and progress
// callback. Built-in training steps additionally receive resolved model/data
// resources from the managed training runner. The caller owns their lifetimes.
type StepContext struct {
	JobStep *tmprl.StepRequest[Request, Artifacts]
	// Request retains the original training operation's artifact contract.
	Request     LegacyStepRequest
	Preparation *PreparationContext
	Workspace   *os.Root
	Trainer     Trainer
	Job         TrainingJob
	Records     []Record
	Fingerprint string
	Publish     func(tmprl.Progress)
}

// PreparationContext supplies resolved document resources to preparation steps.
// The caller owns SourceRoot and the enclosing StepContext's Workspace.
type PreparationContext struct {
	Input      PreparationInput
	Source     PreparationSource
	SourceRoot *os.Root
	Extractor  DocumentExtractor
	Completer  ModelCompleter
}
