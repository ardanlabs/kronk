package prepare

import "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"

type (
	PreparationRequest = model.PreparationRequest
	PreparationRun     = model.PreparationRun
	PreparationResult  = model.PreparationResult
	PreparationKind    = model.PreparationKind
	DocumentExtractor  = model.DocumentExtractor
	ModelCompleter     = model.ModelCompleter
	preparationInput   = model.PreparationInput
)

const (
	GoalQuestionAnswer    = model.GoalQuestionAnswer
	GoalClassification    = model.GoalClassification
	GoalNumericPrediction = model.GoalNumericPrediction
	GoalTextGeneration    = model.GoalTextGeneration
	PreparationPreview    = model.PreparationPreview
	PreparationFull       = model.PreparationFull
	StagePrepare          = model.StagePrepare
)
