package modeltraining

import (
	"context"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// JobRequest selects the inputs for the combined preparation and training job.
// Select exactly one of Prepare, DatasetID, or Preparation.
type JobRequest struct {
	ID          string                          `json:"id,omitzero"`
	Model       string                          `json:"model"`
	Training    training.Training               `json:"training"`
	Prepare     *preparation.PreparationRequest `json:"prepare,omitzero"`
	DatasetID   string                          `json:"dataset_id,omitzero"`
	Preparation tmprl.Execution                 `json:"preparation,omitzero"`
}

// Request is the durable job payload, including the original flat-plan inputs.
// Preserve its JSON representation when evolving existing Temporal histories.
type Request struct {
	ID          string                          `json:"id"`
	Sources     []string                        `json:"sources"`
	Model       string                          `json:"model"`
	Output      string                          `json:"output"`
	Training    training.Training               `json:"training"`
	DatasetID   string                          `json:"dataset_id,omitzero"`
	Preparation tmprl.Execution                 `json:"preparation,omitzero"`
	Prepare     *preparation.PreparationRequest `json:"prepare,omitzero"`
}

// TrainingRequest submits a job using prepared data and a host-approved model.
type TrainingRequest struct {
	ID          string            `json:"id,omitzero"`
	DatasetID   string            `json:"dataset_id,omitzero"`
	Preparation tmprl.Execution   `json:"preparation,omitzero"`
	Model       string            `json:"model"`
	Training    training.Training `json:"training"`
}

// Artifacts carries stage outputs across the combined job's durable boundaries.
// Stage-local handlers do not depend on this whole-job payload.
type Artifacts struct {
	Dataset     string                         `json:"dataset,omitzero"`
	Checkpoint  string                         `json:"checkpoint,omitzero"`
	Report      string                         `json:"report,omitzero"`
	Adapter     string                         `json:"adapter,omitzero"`
	Accepted    bool                           `json:"accepted"`
	Preparation *preparation.PreparationResult `json:"preparation,omitzero"`
}

// LegacyStepRequest preserves the original activity payload and operation key.
// Stage is its historical wire field, not a YAML stage definition.
type LegacyStepRequest struct {
	tmprl.Execution
	Key       string          `json:"key"`
	Stage     tmprl.StageName `json:"stage"`
	Request   Request         `json:"request"`
	Artifacts Artifacts       `json:"artifacts"`
}

// StepHandler implements an application operation registered with the job.
// Stage adapters translate its job-wide inputs into stage-owned contracts.
type StepHandler interface {
	Execute(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)
}

// StepContext supplies job identity and resources borrowed for one operation.
// The caller owns the workspace, trainer and scratch-job lifetimes.
type StepContext struct {
	JobStep     *tmprl.StepRequest[Request, Artifacts]
	Request     LegacyStepRequest
	Workspace   *os.Root
	Trainer     training.Trainer
	Job         training.TrainingJob
	Records     []training.Record
	Fingerprint string
	Publish     func(tmprl.Progress)
}

// TrainingStep preserves the original flat-plan registration contract.
// New stage handlers use their stage-owned StepHandler contracts.
type TrainingStep interface {
	StepHandler
	Name() string
}

// Historical flat-plan activity identities remain stable for workflow replay.
const (
	StageSplit    tmprl.StageName = "prepare"
	StageTrain    tmprl.StageName = "train"
	StageEvaluate tmprl.StageName = "evaluate"
	StagePublish  tmprl.StageName = "publish"
)
