package modeltraining

import (
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/definition"
)

// DefaultSteps supplies the legacy Start API's flat training-step plan.
// New jobs use Config.Steps for handlers and Config.Job for stage/step ordering.
func DefaultSteps() []model.TrainingStep {
	return []model.TrainingStep{split.Step{}, train.Step{}, evaluate.Step{}, publish.Step{}}
}

func validStepName(name string) bool {
	return definition.ValidName(name)
}

// legacyStepID preserves the original split activity's serialized "prepare"
// identity. Only the old payload boundary uses this naming convention.
func legacyStepID(name string) tmprl.Stage {
	if name == "split" {
		return model.StageSplit
	}
	return tmprl.Stage(name)
}
