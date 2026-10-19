package training

import "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"

type (
	Request          = model.Request
	Training         = model.Training
	TrainingRequest  = model.TrainingRequest
	TrainingJob      = model.TrainingJob
	Trainer          = model.Trainer
	Record           = model.Record
	Execution        = model.Execution
	Stage            = model.Stage
	Artifacts        = model.Artifacts
	Run              = model.Run
	Progress         = model.Progress
	StageRequest     = model.StageRequest
	EvaluationReport = model.EvaluationReport
)

const (
	StageSplit      = model.StageSplit
	StageTrain      = model.StageTrain
	StageEvaluate   = model.StageEvaluate
	StagePublish    = model.StagePublish
	StatusRunning   = model.StatusRunning
	StatusCompleted = model.StatusCompleted
	StatusFailed    = model.StatusFailed
	StatusStopped   = model.StatusStopped
	PreparationFull = model.PreparationFull
)
