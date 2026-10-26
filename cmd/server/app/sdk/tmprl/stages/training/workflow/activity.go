package training

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"go.temporal.io/sdk/temporal"
)

const (
	workflowName  = WorkflowName
	stateQuery    = StateQuery
	controlSignal = ControlSignal
)

type workflowInput = Input

// RunStage is the registered training activity entrypoint. It supplies progress
// heartbeats, delegates managed input to trainingStage in pipeline.go (or uses
// the configured Backend), and validates the returned stage artifacts.
func (b *Business) RunStage(ctx context.Context, input StageRequest) (Artifacts, error) {
	publish, stop := model.ActivityProgress(ctx, input.Execution, input.Stage, b.publishProgress)
	defer stop()
	var output Artifacts
	var err error
	switch {
	case input.Request.DatasetID != "" || input.Request.Preparation != (Execution{}):
		output, err = b.trainingStage(ctx, input, publish)
	case b.cfg.Backend != nil:
		output, err = b.cfg.Backend.RunStage(ctx, input, publish)
	default:
		return Artifacts{}, temporal.NewNonRetryableApplicationError("training backend is not configured", "invalid-training", nil)
	}
	if err != nil {
		return Artifacts{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifacts{}, err
	}
	return stageArtifacts(input, output)
}

func stageArtifacts(input StageRequest, output Artifacts) (Artifacts, error) {
	result := input.Artifacts
	switch input.Stage {
	case StageSplit:
		result.Dataset = output.Dataset
		if result.Dataset == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StageTrain:
		result.Checkpoint = output.Checkpoint
		if result.Checkpoint == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StageEvaluate:
		result.Report = output.Report
		result.Accepted = output.Accepted
		if result.Report == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StagePublish:
		if !result.Accepted {
			return Artifacts{}, invalidOutput(input.Stage)
		}
		result.Adapter = output.Adapter
		if result.Adapter == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	default:
		return Artifacts{}, invalidOutput(input.Stage)
	}
	return result, nil
}

func invalidOutput(stage Stage) error {
	return model.InvalidOutput(stage)
}
