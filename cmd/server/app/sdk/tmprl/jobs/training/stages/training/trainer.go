package training

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
)

// GoMLXTrainer translates stage-owned records to computation inputs. Provenance,
// approvals, and held-out answers do not belong to the reusable provider.
type GoMLXTrainer struct {
	computer *providers.GoMLX
}

// NewGoMLXTrainer constructs the stage's GoMLX adapter without loading a model.
func NewGoMLXTrainer(modelPool *pool.Pool) (*GoMLXTrainer, error) {
	computer, err := providers.NewGoMLX(modelPool)
	if err != nil {
		return nil, err
	}
	t := GoMLXTrainer{computer: computer}
	return &t, nil
}

// Validate checks split context limits without exposing held-out answers.
func (t *GoMLXTrainer) Validate(ctx context.Context, job TrainingJob) error {
	return t.computer.Validate(ctx, computationJob(job))
}

// Train converts training inputs and delegates optimization to GoMLX.
func (t *GoMLXTrainer) Train(ctx context.Context, job TrainingJob, publish func(tmprl.Progress)) (TrainingCandidate, error) {
	result, err := t.computer.Train(ctx, computationJob(job), publish)
	return TrainingCandidate(result), err
}

// Predict sends only prompts to inference, withholding targets and evidence.
func (t *GoMLXTrainer) Predict(ctx context.Context, job TrainingJob, adapter string, records []Record, answered func(int)) ([]string, error) {
	questions := make([]providers.Record, len(records))
	for i, record := range records {
		questions[i] = providers.Record{Prompt: record.Prompt}
	}
	return t.computer.Predict(ctx, computationJob(job), adapter, questions, answered)
}

func computationJob(job TrainingJob) providers.TrainingJob {
	records := func(input []Record) []providers.Record {
		output := make([]providers.Record, len(input))
		for i, record := range input {
			output[i] = providers.Record{Prompt: record.Prompt, Target: record.Target}
		}
		return output
	}
	test := make([]providers.Record, len(job.Dataset.Test))
	for i, record := range job.Dataset.Test {
		test[i] = providers.Record{Prompt: record.Prompt}
	}
	return providers.TrainingJob{
		ModelID: job.ModelID, Model: job.Model, Workdir: job.Workdir,
		PausePoint: job.PausePoint,
		SafePoint:  job.SafePoint,
		Training:   providers.Training(job.Training), PredictionTokens: 512,
		Dataset: providers.TrainingDataset{
			Train: records(job.Dataset.Train), Validation: records(job.Dataset.Validation), Test: test,
		},
	}
}
