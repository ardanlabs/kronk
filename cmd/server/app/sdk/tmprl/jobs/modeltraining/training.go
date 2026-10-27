package modeltraining

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	provider "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/providers/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	prepare "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/standalone/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/applog"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

var ErrNotFound = imports.ErrNotFound

// Backend performs model-specific computation, never presentation work. It
// must honor ctx and deduplicate side effects using LegacyStepRequest.Key because
// Temporal activities can execute more than once. Progress may be concurrent.
// Paths/references are resolved on the worker, not by an HTTP handler. No legacy
// fineapp types are exposed by this SDK.
// This optional seam supports alternative pipelines. StartTraining uses the
// SDK's built-in preparation-to-GoMLX pipeline instead.
type Backend interface {
	RunStage(context.Context, LegacyStepRequest, func(tmprl.Progress)) (Artifacts, error)
}

type Config struct {
	// Job defaults to the embedded YAML definition. Steps registers additional
	// implementations by "stage/step"; the YAML, not this map, owns execution order.
	Job       *tmprl.Job
	Steps     map[string]StepHandler
	TaskQueue string
	// StageTimeout and the following legacy timeout fields configure Start only.
	// StartJob and StartTraining use Job's YAML-derived policies instead.
	StageTimeout time.Duration
	// JobTimeout bounds a worker reservation, including pauses. QueueTimeout
	// bounds waiting for an available worker. Defaults are seven days and one day.
	JobTimeout   time.Duration
	QueueTimeout time.Duration
	// LegacySteps is the legacy Start API's flat plan. Nil uses DefaultSteps.
	// Keep implementations registered while submitted jobs still reference them.
	LegacySteps []TrainingStep
	Log         applog.Logger
	// Backend executes the optional alternative pipeline submitted through Start.
	// Workers using a Trainer do not require it.
	Backend     Backend
	Datasets    *imports.Business
	Preparation *prepare.Business
	Progress    *tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress]
	// Models maps approved IDs to local Gemma 3 GGUFs. Trainer defaults to the
	// SDK GoMLX implementation. These host settings never become workflow inputs.
	Models      map[string]string
	Trainer     training.Trainer
	LlamaServer string
}

type Business struct {
	client            client.Client
	cfg               Config
	progress          *tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress]
	legacySteps       map[tmprl.StageName]TrainingStep
	enabled           bool
	customLegacySteps bool
	job               tmprl.Job
	steps             map[string]StepHandler
	jobs              *tmprl.Jobs[Request, Artifacts]
}

// New borrows the Temporal client. The host owns client/worker shutdown.
func New(c client.Client, cfg Config) (*Business, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is required", ErrInvalidArgument)
	}
	if cfg.Datasets == nil || cfg.Preparation == nil {
		return nil, fmt.Errorf("%w: dataset and preparation services are required", ErrInvalidArgument)
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "moxie-tuning"
	}
	if strings.TrimSpace(cfg.TaskQueue) == "" || len(cfg.TaskQueue) > 200 {
		return nil, fmt.Errorf("%w: invalid task queue", ErrInvalidArgument)
	}
	if cfg.TaskQueue == cfg.Preparation.TaskQueue() {
		return nil, fmt.Errorf("%w: preparation and training require separate task queues", ErrInvalidArgument)
	}
	if cfg.StageTimeout == 0 {
		cfg.StageTimeout = 24 * time.Hour
	}
	if cfg.StageTimeout < 5*time.Second {
		return nil, fmt.Errorf("%w: stage timeout must be at least five seconds", ErrInvalidArgument)
	}
	if cfg.JobTimeout == 0 {
		cfg.JobTimeout = 7 * 24 * time.Hour
	}
	if cfg.QueueTimeout == 0 {
		cfg.QueueTimeout = 24 * time.Hour
	}
	if cfg.JobTimeout < 5*time.Second || cfg.QueueTimeout < 5*time.Second {
		return nil, fmt.Errorf("%w: job and queue timeouts must be at least five seconds", ErrInvalidArgument)
	}
	if cfg.JobTimeout > time.Duration(math.MaxInt64)-cfg.QueueTimeout {
		return nil, fmt.Errorf("%w: combined job and queue timeouts exceed maximum duration", ErrInvalidArgument)
	}
	if cfg.Log == nil {
		cfg.Log = applog.FmtLogger
	}
	customLegacySteps := cfg.LegacySteps != nil
	if cfg.LegacySteps == nil {
		cfg.LegacySteps = DefaultSteps()
	}
	cfg.LegacySteps = slices.Clone(cfg.LegacySteps)
	legacySteps := make(map[tmprl.StageName]TrainingStep, len(cfg.LegacySteps))
	for _, step := range cfg.LegacySteps {
		if step == nil || !validStepName(step.Name()) {
			return nil, fmt.Errorf("%w: steps require a safe, nonempty name", ErrInvalidArgument)
		}
		id := legacyStepID(step.Name())
		if _, exists := legacySteps[id]; exists {
			return nil, fmt.Errorf("%w: duplicate step %q", ErrInvalidArgument, step.Name())
		}
		legacySteps[id] = step
	}
	if len(legacySteps) == 0 {
		return nil, fmt.Errorf("%w: at least one step is required", ErrInvalidArgument)
	}
	cfg.Models = maps.Clone(cfg.Models)
	for id, path := range cfg.Models {
		if strings.TrimSpace(id) == "" || path == "" {
			return nil, fmt.Errorf("%w: model IDs and paths are required", ErrInvalidArgument)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		cfg.Models[id] = absolute
	}
	if cfg.Trainer == nil && len(cfg.Models) > 0 {
		cfg.Trainer = provider.NewGoMLX(cfg.LlamaServer)
	}
	if cfg.Progress == nil {
		cfg.Progress = new(tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress])
	}
	b := Business{client: c, cfg: cfg, progress: cfg.Progress, legacySteps: legacySteps, enabled: customLegacySteps || cfg.Backend != nil || cfg.Trainer != nil, customLegacySteps: customLegacySteps}
	if err := b.configureJob(); err != nil {
		return nil, err
	}
	return &b, nil
}

// NewWorker registers training execution. The host must Start/Run and Stop it.
func (b *Business) NewWorker() (worker.Worker, error) {
	if !b.Enabled() {
		return nil, fmt.Errorf("%w: training backend is required", ErrInvalidArgument)
	}
	w := tmprl.NewWorker(b.client, b.cfg.TaskQueue, b.WorkerOptions(), b.Register)
	return w, nil
}

func (b *Business) Enabled() bool { return b.enabled }

// WorkerOptions reserves each worker for one entire job, including pauses and
// stage retries. Workflows remain responsive to queries and control signals.
func (b *Business) WorkerOptions() worker.Options {
	return b.jobs.WorkerOptions()
}

// Register attaches the training workflow and activity to a host-owned worker.
func (b *Business) Register(w worker.Registry) {
	if b.Enabled() {
		w.RegisterWorkflowWithOptions(Workflow, workflow.RegisterOptions{Name: WorkflowName})
		w.RegisterActivityWithOptions(b.RunStage, activity.RegisterOptions{Name: ActivityName})
		w.RegisterActivityWithOptions(b.ExecuteStage, activity.RegisterOptions{Name: SessionActivityName})
		b.jobs.RegisterActivity(w)
	}
}

func (b *Business) publishProgress(update tmprl.Progress) {
	b.progress.Publish(update.Execution, update)
}

// Subscribe receives local training progress.
func (b *Business) Subscribe(e tmprl.Execution) (<-chan tmprl.Progress, func()) {
	return b.progress.Subscribe(e)
}

func (b *Business) Start(ctx context.Context, input Request) (tmprl.Run[Request, Artifacts], error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Model = strings.TrimSpace(input.Model)
	input.Output = strings.TrimSpace(input.Output)
	input.Sources = slices.Clone(input.Sources)
	for i := range input.Sources {
		input.Sources[i] = strings.TrimSpace(input.Sources[i])
	}
	if err := validate(input); err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	if input.ID == "" {
		input.ID = "tuning-" + rand.Text()
	}
	stages := make([]tmprl.StageName, len(b.cfg.LegacySteps))
	for i, step := range b.cfg.LegacySteps {
		stages[i] = legacyStepID(step.Name())
	}
	plan := Input{Request: input, StageTimeout: b.cfg.StageTimeout, Stages: stages, JobTimeout: b.cfg.JobTimeout, QueueTimeout: b.cfg.QueueTimeout}
	state := queuedRun(plan)
	execution, err := tmprl.Start(ctx, b.client, client.StartWorkflowOptions{
		ID: input.ID, TaskQueue: b.cfg.TaskQueue,
		Memo:                                     map[string]any{runMemoKey: state},
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, workflowName, plan)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, fmt.Errorf("start tuning: %w", err)
	}
	b.cfg.Log(ctx, "training job submitted", "workflow_id", execution.WorkflowID, "run_id", execution.RunID, "task_queue", b.cfg.TaskQueue, "stages", stages)
	state.Execution = execution
	return state, nil
}

func validate(r Request) error {
	managed := r.DatasetID != "" || r.Preparation != (tmprl.Execution{}) || r.Prepare != nil
	switch {
	case len(r.ID) > 200 || r.Model == "" || (!managed && (r.Output == "" || len(r.Sources) == 0)):
		return fmt.Errorf("%w: model, output, sources and an ID of at most 200 bytes are required", ErrInvalidArgument)
	case r.Training.MaximumEpochs < 1 || r.Training.MaximumEpochs > 100 || r.Training.BatchSize < 1 || r.Training.BatchSize > 128 || r.Training.Seed < 0:
		return fmt.Errorf("%w: invalid epoch, batch size or seed bounds", ErrInvalidArgument)
	case r.Training.LearningRate <= 0 || r.Training.LearningRate > 1 || math.IsNaN(r.Training.LearningRate) || math.IsInf(r.Training.LearningRate, 0):
		return fmt.Errorf("%w: learning rate must be finite and in (0,1]", ErrInvalidArgument)
	}
	if slices.Contains(r.Sources, "") {
		return fmt.Errorf("%w: empty source", ErrInvalidArgument)
	}
	return nil
}

// Pause takes effect after the active step commits (legacy Start: stage).
// Resume continues that same
// run; it does not restart a stopped run or repeat completed stages.
func (b *Business) Pause(ctx context.Context, e tmprl.Execution) error {
	return ExecutionError(b.jobs.Pause(ctx, e))
}
func (b *Business) Resume(ctx context.Context, e tmprl.Execution) error {
	return ExecutionError(b.jobs.Resume(ctx, e))
}

// Stop requests cooperative cancellation of training. Wait/Get establish when
// the execution is closed.
func (b *Business) Stop(ctx context.Context, e tmprl.Execution) error {
	return ExecutionError(b.jobs.Stop(ctx, e))
}

func (b *Business) Wait(ctx context.Context, e tmprl.Execution) (tmprl.Run[Request, Artifacts], error) {
	result, err := b.jobs.Wait(ctx, e)
	return result, ExecutionError(err)
}

// Get reads durable lifecycle snapshots and enriches active stages with server
// heartbeat/retry diagnostics. It remains usable without a live worker. Older
// runs fall back to a bounded live query, then last-known history.
func (b *Business) Get(ctx context.Context, e tmprl.Execution) (tmprl.Run[Request, Artifacts], error) {
	result, err := b.jobs.Get(ctx, e)
	return result, ExecutionError(err)
}

// History pages through training executions, including active runs.
func (b *Business) History(ctx context.Context, pageSize int, next []byte) ([]tmprl.Execution, []byte, error) {
	runs, token, err := b.jobs.History(ctx, pageSize, next)
	if errors.Is(err, tmprl.ErrInvalidPageSize) {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return runs, token, err
}
