// Package training defines the document-preparation and model-training job.
// Foundation owns execution; this package supplies business inputs and steps.
package training

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/imports"
	provider "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/providers/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/applog"
	"go.temporal.io/sdk/client"
)

//go:embed job.yaml
var jobYAML []byte

const storageDirectory = "training-jobs"

// Request selects either document preparation or an imported prepared dataset.
type Request struct {
	ID        string                          `json:"id"`
	Model     string                          `json:"model"`
	Training  optimization.Training           `json:"training"`
	DatasetID string                          `json:"dataset_id,omitzero"`
	Prepare   *preparation.PreparationRequest `json:"prepare,omitzero"`
}

// Artifacts retains the outputs owned by each stage of the job.
type Artifacts struct {
	Preparation *preparation.PreparationResult `json:"preparation,omitzero"`
	Training    optimization.Artifacts         `json:"training"`
}

// Config supplies worker-local business resources, not execution policies.
// Models maps approved identifiers to GGUF paths. Trainer defaults to GoMLX.
type Config struct {
	TaskQueue   string
	Imports     *imports.Business
	Models      map[string]string
	Trainer     optimization.Trainer
	LlamaServer string
	Extractor   preparation.DocumentExtractor
	Completer   preparation.ModelCompleter
	Log         applog.Logger
}

// Business validates submissions and connects stage resources to the job runner.
type Business struct {
	cfg  Config
	jobs *tmprl.Jobs[Request, Artifacts]
}

// New constructs the job without contacting Temporal or loading a model.
// The host owns the client, providers, storage, and worker lifetimes.
func New(c client.Client, cfg Config) (*Business, error) {
	if cfg.Imports == nil || len(cfg.Models) == 0 {
		return nil, fmt.Errorf("%w: imports and approved models are required", tmprl.ErrInvalidArgument)
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "moxie-training"
	}
	cfg.Models = maps.Clone(cfg.Models)
	for id, path := range cfg.Models {
		if strings.TrimSpace(id) == "" || path == "" {
			return nil, fmt.Errorf("%w: model identifiers and paths are required", tmprl.ErrInvalidArgument)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolving model: %w", err)
		}
		cfg.Models[id] = absolute
	}
	if cfg.Trainer == nil {
		cfg.Trainer = provider.NewGoMLX(cfg.LlamaServer)
	}
	if cfg.Log == nil {
		cfg.Log = applog.FmtLogger
	}
	b := Business{cfg: cfg}
	jobs, err := tmprl.New(c, tmprl.Config[Request, Artifacts]{
		Names:     tmprl.Names{Workflow: "moxie.training.Job", Activity: "moxie.training.Step", Memo: "moxie.training.job", Query: "training-state", Control: "training-control"},
		TaskQueue: cfg.TaskQueue, StorageRoot: cfg.Imports.Root(), StorageDirectory: storageDirectory,
		Steps: b.steps(), Outcomes: []tmprl.Status{optimization.StatusNeedsImprovement}, Log: cfg.Log,
	})
	if err != nil {
		return nil, err
	}
	plan, err := tmprl.ParseJob(jobYAML)
	if err != nil {
		return nil, fmt.Errorf("loading training definition: %w", err)
	}
	if err := jobs.ValidatePlan(plan); err != nil {
		return nil, err
	}
	b.jobs = jobs
	return &b, nil
}

// Jobs exposes foundation's worker, status, history, progress, and control APIs.
// Submit training requests through Start so business validation is applied.
func (b *Business) Jobs() *tmprl.Jobs[Request, Artifacts] { return b.jobs }

// Start submits the embedded plan after validating and copying business inputs.
// Prepared JSONL skips preparation; documents execute both stages on one worker.
func (b *Business) Start(ctx context.Context, input Request) (tmprl.Run[Request, Artifacts], error) {
	input, err := b.validate(ctx, input)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	plan, err := tmprl.ParseJob(jobYAML)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	if input.Prepare == nil {
		plan.Stages = slices.DeleteFunc(plan.Stages, func(stage tmprl.Stage) bool { return stage.Name == "preparation" })
	}
	if input.ID == "" {
		input.ID = "training-" + rand.Text()
	}
	return b.jobs.Submit(ctx, input.ID, tmprl.JobInput[Request]{Job: &plan, Request: input})
}
