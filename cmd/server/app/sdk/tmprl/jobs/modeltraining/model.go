package modeltraining

import "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"

type (
	JobRequest        = model.JobRequest
	StepHandler       = model.StepHandler
	StepContext       = model.StepContext
	Request           = model.Request
	Training          = model.Training
	TrainingRequest   = model.TrainingRequest
	TrainingJob       = model.TrainingJob
	Trainer           = model.Trainer
	Record            = model.Record
	Artifacts         = model.Artifacts
	LegacyStepRequest = model.LegacyStepRequest
	TrainingStep      = model.TrainingStep
	EvaluationReport  = model.EvaluationReport
)

const (
	StageSplit             = model.StageSplit
	StageTrain             = model.StageTrain
	StageEvaluate          = model.StageEvaluate
	StagePublish           = model.StagePublish
	StatusNeedsImprovement = model.StatusNeedsImprovement
	PreparationFull        = model.PreparationFull
)
