package modeltraining

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"strings"

	trainingdefinition "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/definition"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/definition"
)

func jobNames() tmprl.Names {
	return tmprl.Names{Workflow: WorkflowName, Activity: JobStepActivityName, Memo: runMemoKey, Query: StateQuery, Control: ControlSignal}
}

func (b *Business) configureJob() error {
	job, err := trainingdefinition.Load()
	if err != nil {
		return fmt.Errorf("loading embedded job: %w", err)
	}
	if b.cfg.Job != nil {
		job = *b.cfg.Job
	}
	job.Stages = slices.Clone(job.Stages)
	for i := range job.Stages {
		job.Stages[i].Steps = slices.Clone(job.Stages[i].Steps)
	}
	b.job = job
	b.steps = b.builtinSteps()
	for name, implementation := range b.cfg.Steps {
		if implementation == nil {
			return fmt.Errorf("%w: nil step implementation %q", ErrInvalidArgument, name)
		}
		b.steps[name] = implementation
	}
	b.jobs, err = tmprl.New(b.client, tmprl.Config[Request, Artifacts]{
		Names: jobNames(), TaskQueue: b.cfg.TaskQueue,
		StorageRoot: b.cfg.Datasets.Root(), StorageDirectory: "training",
		Steps: b.jobSteps(), Outcomes: []tmprl.Status{StatusNeedsImprovement},
		Log: b.cfg.Log, Progress: b.progress,
		ResultActivities: []string{SessionActivityName}, ArtifactActivities: []string{ActivityName},
	})
	if err != nil {
		return model.ExecutionError(err)
	}
	if err := b.jobs.ValidatePlan(job); err != nil {
		return model.ExecutionError(err)
	}
	b.enabled = b.enabled || len(b.cfg.Steps) > 0
	return nil
}

// StartJob submits the YAML-defined job. Preparation and training execute in
// one reserved worker, with one Temporal activity per step. Get, List, Pause,
// Resume, Stop and Wait use the returned execution identity.
func (b *Business) StartJob(ctx context.Context, input model.JobRequest) (tmprl.Run[model.Request, model.Artifacts], error) {
	r := Request{ID: strings.TrimSpace(input.ID), Model: strings.TrimSpace(input.Model), Training: input.Training, DatasetID: strings.TrimSpace(input.DatasetID), Preparation: input.Preparation, Prepare: input.Prepare}
	if r.Training == (Training{}) {
		r.Training = Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407}
	}
	if r.Training.LoRARank == 0 {
		r.Training.LoRARank = 8
	}
	if r.Training.LoRAAlpha == 0 {
		r.Training.LoRAAlpha = 8
	}
	if err := b.validateTraining(r); err != nil {
		return tmprl.Run[model.Request, model.Artifacts]{}, err
	}
	if r.Prepare != nil {
		prepared, err := b.cfg.Preparation.ValidateRequest(ctx, *r.Prepare, model.PreparationFull)
		if err != nil {
			return tmprl.Run[model.Request, model.Artifacts]{}, err
		}
		prepared.ID = ""
		r.Prepare = &prepared
	} else if _, err := b.trainingRecords(ctx, r); err != nil {
		return tmprl.Run[model.Request, model.Artifacts]{}, err
	}
	if r.ID == "" {
		r.ID = "training-" + rand.Text()
	}
	job := b.job
	job.Stages = slices.Clone(job.Stages)
	for i := range job.Stages {
		job.Stages[i].Steps = slices.Clone(job.Stages[i].Steps)
	}
	// Already-prepared data deliberately skips the preparation stage. Its steps
	// must not fabricate or overwrite the supplied dataset.
	if r.Prepare == nil {
		job.Stages = slices.DeleteFunc(job.Stages, func(stage definition.Stage) bool { return stage.Name == "preparation" })
	}
	state, err := b.jobs.Submit(ctx, r.ID, tmprl.JobInput[Request]{Job: &job, Request: r})
	return state, model.ExecutionError(err)
}
