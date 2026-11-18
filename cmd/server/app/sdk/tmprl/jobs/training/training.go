// Package training defines the document-preparation and model-training job.
// Foundation owns execution; this package supplies training inputs and steps.
package training

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/preview"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/verify"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

//go:embed job.yaml
var jobYAML []byte

const storageDirectory = "training-jobs"

// Request selects either document preparation or an imported prepared dataset.
type Request struct {
	ID              string                          `json:"id"`
	PreparationOnly bool                            `json:"preparation_only,omitzero"`
	Model           string                          `json:"model"`
	Training        optimization.Training           `json:"training"`
	DatasetID       string                          `json:"dataset_id,omitzero"`
	Prepare         *preparation.PreparationRequest `json:"prepare,omitzero"`
	SavedDataset    *SavedDataset                   `json:"saved_dataset,omitzero"`
}

// SavedDataset selects a saved dataset version as training input. Training reads
// the version's records from the dataset store; nothing is uploaded.
type SavedDataset struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// Artifacts retains the outputs owned by each stage of the job.
type Artifacts struct {
	Preview     *preparation.PreparationResult `json:"preview,omitzero"`
	Preparation *preparation.PreparationResult `json:"preparation,omitzero"`
	Training    optimization.Artifacts         `json:"training"`
}

// State combines an execution observation with training output readiness and ETA.
// Catalog persistence uses a sanitized projection; execution history and stage
// artifacts retain their existing formats.
type State struct {
	tmprl.Run[Request, Artifacts]
	EvaluationAvailable bool
	AdapterAvailable    bool
	RemainingSeconds    int64
	FailureReason       string
	Issues              []RunIssue `json:"issues,omitempty"`
}

// RunIssue contains only a stable diagnostic code and counts, never error text.
type RunIssue struct {
	Code     string `json:"code"`
	Count    int    `json:"count"`
	Required int    `json:"required,omitempty"`
}

// Describe interprets an execution observation without querying or changing it.
func Describe(run tmprl.Run[Request, Artifacts]) State {
	state := State{
		Run:                 run,
		EvaluationAvailable: run.Artifacts.Training.Report != "",
		AdapterAvailable: run.Status == tmprl.StatusCompleted && run.Artifacts.Training.Accepted &&
			run.Artifacts.Training.Report != "" && run.Artifacts.Training.Adapter != "",
		FailureReason: FailureReason(run.FailureTypes),
	}
	if run.Progress != nil && run.Status == tmprl.StatusRunning && !run.StateStale && run.Progress.Stage == run.Stage && run.Progress.Step == run.Step {
		state.RemainingSeconds = int64(run.Progress.Remaining().Round(time.Second) / time.Second)
	}
	return state
}

// Config supplies worker-local training resources, not execution policies.
// Models maps approved identifiers to GGUF paths. Trainer defaults to GoMLX.
// ModelPool supplies KMS inference for the default trainer and completer.
type Config struct {
	TaskQueue   string
	Namespace   string
	Uploads     *uploads.Store
	Datasets    *datasetstore.Store
	MoxieDB     *moxiedb.DB
	StorageRoot string
	Models      map[string]string
	Trainer     optimization.Trainer
	ModelPool   *pool.Pool
	Extractor   preparation.DocumentExtractor
	Completer   preparation.ModelCompleter
	Log         applog.Logger
}

// Job validates training submissions and connects stage resources to the runner.
type Job struct {
	cfg    Config
	client client.Client
	jobs   *tmprl.Jobs[Request, Artifacts]
}

// New constructs the job without contacting Temporal or loading a model.
// The host owns the client, providers, storage, and worker lifetimes.
func New(c client.Client, cfg Config) (*Job, error) {
	if cfg.Uploads == nil || cfg.StorageRoot == "" {
		return nil, fmt.Errorf("%w: uploads and execution storage root are required", tmprl.ErrInvalidArgument)
	}
	root, err := filepath.Abs(cfg.StorageRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving execution storage: %w", err)
	}
	cfg.StorageRoot = root
	if cfg.Datasets == nil {
		cfg.Datasets, err = datasetstore.New(datasetstore.Config{Root: filepath.Join(root, "preparation-datasets")})
		if err != nil {
			return nil, err
		}
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "moxie-training"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
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
		cfg.Trainer, err = optimization.NewGoMLXTrainer(cfg.ModelPool)
		if err != nil {
			return nil, err
		}
	}
	if cfg.Completer == nil && cfg.ModelPool != nil {
		cfg.Completer, err = providers.NewKMSCompleter(cfg.ModelPool)
		if err != nil {
			return nil, err
		}
	}
	if cfg.Log == nil {
		cfg.Log = applog.FmtLogger
	}
	j := Job{cfg: cfg, client: c}
	jobs, err := tmprl.New(c, tmprl.Config[Request, Artifacts]{
		Names:     tmprl.Names{Workflow: "moxie.training.Job", Activity: "moxie.training.Step", Memo: "moxie.training.job", Query: "training-state", Control: "training-control"},
		TaskQueue: cfg.TaskQueue, Queue: j.queue, StorageRoot: cfg.StorageRoot, StorageDirectory: storageDirectory,
		Steps: j.steps(), Outcomes: []tmprl.Status{optimization.StatusNeedsImprovement}, Log: cfg.Log,
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
	j.jobs = jobs
	return &j, nil
}

// Jobs exposes foundation's worker and infrastructure APIs to the host.
// Application callers use Start, Get, List, review methods, and Stop instead.
func (j *Job) Jobs() *tmprl.Jobs[Request, Artifacts] { return j.jobs }

// queue gives runs that train their own queue, so one preparation and one
// training run side by side. Preparation runs use TaskQueue.
func (j *Job) queue(r Request) string {
	if r.PreparationOnly {
		return j.cfg.TaskQueue
	}
	return j.cfg.TaskQueue + "-train"
}

// Workers creates one worker per queue: preparation and training. The host
// starts and stops them.
func (j *Job) Workers() []worker.Worker {
	return []worker.Worker{j.jobs.NewWorker(), j.jobs.NewQueueWorker(j.queue(Request{}))}
}

// Datasets exposes the configured reusable asset store for host integration.
// HTTP dependencies are supplied explicitly by the host, not discovered here.
func (j *Job) Datasets() *datasetstore.Store { return j.cfg.Datasets }

var (
	ErrNotFound      = errors.New("training resource not found")
	ErrAlreadyExists = errors.New("training job already exists")
	ErrUnavailable   = errors.New("training service is unavailable")
)

// executionError provides application error identities while preserving the cause.
func executionError(err error) error {
	if _, ok := errors.AsType[*serviceerror.NotFound](err); ok {
		return errors.Join(ErrNotFound, err)
	}
	if _, ok := errors.AsType[*serviceerror.WorkflowExecutionAlreadyStarted](err); ok {
		return errors.Join(ErrAlreadyExists, err)
	}
	if _, ok := errors.AsType[*serviceerror.Unavailable](err); ok {
		return errors.Join(ErrUnavailable, err)
	}
	if _, ok := errors.AsType[*serviceerror.DeadlineExceeded](err); ok {
		return errors.Join(context.DeadlineExceeded, err)
	}
	return err
}

// Get reads execution state and interprets training-specific output readiness.
func (j *Job) Get(ctx context.Context, e tmprl.Execution) (State, error) {
	var attempt moxiedb.Attempt
	if j.cfg.MoxieDB != nil {
		var err error
		attempt, err = j.cfg.MoxieDB.GetAttempt(ctx, jobIdentity(j.cfg.Namespace, e.WorkflowID)+"/1")
		if err != nil && !errors.Is(err, moxiedb.ErrNotFound) {
			return State{}, err
		}
	}
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return State{}, executionError(err)
	}
	if j.cfg.MoxieDB != nil {
		if attempt.AttemptID == "" {
			attempt, err = j.registerAttempt(ctx, run)
		} else {
			err = j.cfg.MoxieDB.LinkAttemptRun(ctx, attempt.AttemptID, run.RunID)
		}
		if err == nil {
			err = j.observe(ctx, attempt, run)
		}
		if err != nil {
			return State{}, err
		}
	}
	state := Describe(run)
	state.Issues = j.runIssues(run)
	return state, nil
}

// List preserves runner ordering and pagination and interprets each observation.
func (j *Job) List(ctx context.Context, options tmprl.ListOptions) ([]State, []byte, error) {
	runs, next, err := j.jobs.List(ctx, options)
	if err != nil {
		return nil, nil, executionError(err)
	}
	states := make([]State, len(runs))
	for i, run := range runs {
		states[i] = Describe(run)
	}
	return states, next, nil
}

// Stop requests cooperative cancellation without deleting committed outputs.
// Acceptance is not terminal completion; Get or List establishes that outcome.
func (j *Job) Stop(ctx context.Context, e tmprl.Execution) error {
	if j.cfg.MoxieDB == nil {
		return executionError(j.jobs.Stop(ctx, e))
	}
	intent, err := j.beginControl(ctx, e, "stop", tmprl.Decision{})
	if err != nil || intent.State == "accepted" {
		return err
	}
	return j.settleControl(ctx, intent, executionError(j.jobs.Stop(ctx, e)))
}

// Start submits the embedded plan after validating and copying training inputs.
// Prepared JSONL skips preparation. Documents wait for Preview review;
// preparation-only runs finish after Verify, while combined runs wait for
// explicit authorization to enter Training, using one reserved worker.
func (j *Job) Start(ctx context.Context, input Request) (tmprl.Run[Request, Artifacts], error) {
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		input.ID = "training-" + rand.Text()
	}
	if j.cfg.MoxieDB != nil {
		return j.catalogSubmit(ctx, input)
	}
	input, plan, err := j.submission(ctx, input)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	run, err := j.jobs.Submit(ctx, input.ID, tmprl.JobInput[Request]{Job: &plan, Request: input})
	return run, executionError(err)
}

func (j *Job) submission(ctx context.Context, input Request) (Request, tmprl.Job, error) {
	input, err := j.validate(ctx, input)
	if err != nil {
		return Request{}, tmprl.Job{}, err
	}
	plan, err := tmprl.ParseJob(jobYAML)
	if err != nil {
		return Request{}, tmprl.Job{}, err
	}
	if input.Prepare == nil {
		plan.Stages = slices.DeleteFunc(plan.Stages, func(stage tmprl.Stage) bool { return stage.Name == "preparation" })
	}
	if input.PreparationOnly {
		plan.Stages = slices.DeleteFunc(plan.Stages, func(stage tmprl.Stage) bool { return stage.Name == "training" })
		plan.Stages[0].WaitAfter = ""
	}
	return input, plan, nil
}

// ContinuePreview accepts edits to the saved examples and releases Preview.
// The receipt acknowledges accepted input, not completion of the full dataset.
func (j *Job) ContinuePreview(ctx context.Context, e tmprl.Execution, decision tmprl.Decision) (tmprl.Decision, error) {
	if err := decision.Validate(); err != nil {
		return tmprl.Decision{}, err
	}
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return tmprl.Decision{}, executionError(err)
	}
	if run.Request.Prepare == nil || decision.CheckpointID != run.RunID+"/preparation/preview/preview-review" {
		return tmprl.Decision{}, fmt.Errorf("%w: decision must identify this run's Preview", tmprl.ErrInvalidArgument)
	}
	review, err := preview.ValidateReview(*run.Request.Prepare, run.Artifacts.Preview, decision.Payload)
	if err != nil {
		return tmprl.Decision{}, err
	}
	decision.Payload, err = json.Marshal(review)
	if err != nil {
		return tmprl.Decision{}, err
	}
	for _, accepted := range run.Decisions {
		if accepted.CheckpointID == decision.CheckpointID {
			if accepted.SubmissionID == decision.SubmissionID && bytes.Equal(accepted.Payload, decision.Payload) {
				return accepted, nil
			}
			return tmprl.Decision{}, fmt.Errorf("%w: Preview review conflicts with accepted input", tmprl.ErrInvalidArgument)
		}
	}
	if run.Status.Terminal() || run.Checkpoint == nil || run.Checkpoint.ID != decision.CheckpointID || run.Checkpoint.Stage != "preparation" || run.Checkpoint.Step != "preview" {
		return tmprl.Decision{}, fmt.Errorf("%w: Preview is not waiting for review", tmprl.ErrInvalidArgument)
	}
	return j.decide(ctx, e, "preview_review", decision)
}

// StartTraining authorizes the Training stage only after document preparation
// completes with a training-ready dataset. Decision has no application payload.
func (j *Job) StartTraining(ctx context.Context, e tmprl.Execution, decision tmprl.Decision) (tmprl.Decision, error) {
	if err := decision.Validate(); err != nil {
		return tmprl.Decision{}, err
	}
	if len(decision.Payload) != 0 {
		return tmprl.Decision{}, fmt.Errorf("%w: StartTraining does not accept a payload", tmprl.ErrInvalidArgument)
	}
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return tmprl.Decision{}, executionError(err)
	}
	if run.Request.Prepare == nil || decision.CheckpointID != run.RunID+"/preparation//start-training" {
		return tmprl.Decision{}, fmt.Errorf("%w: decision must identify this run's completed Preparation", tmprl.ErrInvalidArgument)
	}
	for _, accepted := range run.Decisions {
		if accepted.CheckpointID == decision.CheckpointID {
			if accepted.SubmissionID == decision.SubmissionID && bytes.Equal(accepted.Payload, decision.Payload) {
				return accepted, nil
			}
			return tmprl.Decision{}, fmt.Errorf("%w: Training authorization conflicts with accepted input", tmprl.ErrInvalidArgument)
		}
	}
	if run.Status.Terminal() || run.Checkpoint == nil || run.Checkpoint.ID != decision.CheckpointID || run.Checkpoint.Stage != "preparation" || run.Checkpoint.Step != "" || !slices.ContainsFunc(run.Stages, func(stage tmprl.StageState) bool {
		return stage.Name == "preparation" && stage.Status == tmprl.StatusCompleted
	}) {
		return tmprl.Decision{}, fmt.Errorf("%w: Preparation has not completed", tmprl.ErrInvalidArgument)
	}
	if _, err := j.records(ctx, run.Request, run.Artifacts, run.RunID); err != nil {
		return tmprl.Decision{}, err
	}
	return j.decide(ctx, e, "authorize_training", decision)
}

// Evaluation reads the persisted report, including a rejected candidate's report.
func (j *Job) Evaluation(ctx context.Context, e tmprl.Execution) (evaluate.EvaluationReport, error) {
	file, err := j.openArtifact(ctx, e, false)
	if err != nil {
		return evaluate.EvaluationReport{}, err
	}
	defer file.Close()
	var report evaluate.EvaluationReport
	err = json.NewDecoder(tmprl.ContextReader{Context: ctx, Reader: file}).Decode(&report)
	return report, err
}

// OpenAdapter streams an accepted, published adapter. The caller closes it.
func (j *Job) OpenAdapter(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	file, err := j.openArtifact(ctx, e, true)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// OpenDataset streams a completed preparation dataset, even if it is too small
// for training. The caller closes it. Artifact paths never select the file.
func (j *Job) OpenDataset(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return nil, executionError(err)
	}
	if run.Request.Prepare == nil || run.Artifacts.Preparation == nil || run.Artifacts.Preparation.RecordsPath == "" || run.Artifacts.Preparation.DatasetID != run.Request.Prepare.DatasetID {
		return nil, fmt.Errorf("%w: prepared dataset is not available", tmprl.ErrInvalidArgument)
	}
	if result := run.Artifacts.Preparation; result.SavedDatasetID != "" {
		if _, err := j.dataset(ctx, result.SavedDatasetID, result.SavedVersion); err != nil {
			return nil, err
		}
		records, err := j.cfg.Datasets.Records(ctx, result.SavedDatasetID, result.SavedVersion)
		if err != nil {
			return nil, err
		}
		var data bytes.Buffer
		for _, record := range records {
			if err := json.NewEncoder(&data).Encode(record); err != nil {
				return nil, err
			}
		}
		return io.NopCloser(&data), nil
	}
	root, err := os.OpenRoot(j.cfg.StorageRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join(storageDirectory, run.RunID, "steps", "preparation", "dataset.jsonl"))
}

// =============================================================================

func (j *Job) validate(ctx context.Context, r Request) (Request, error) {
	r.ID, r.Model, r.DatasetID = strings.TrimSpace(r.ID), strings.TrimSpace(r.Model), strings.TrimSpace(r.DatasetID)
	if len(r.ID) > 200 || !r.PreparationOnly && j.cfg.Models[r.Model] == "" {
		return Request{}, fmt.Errorf("%w: approved model and ID of at most 200 bytes are required", tmprl.ErrInvalidArgument)
	}
	inputs := 0
	for _, selected := range []bool{r.Prepare != nil, r.DatasetID != "", r.SavedDataset != nil} {
		if selected {
			inputs++
		}
	}
	if inputs != 1 {
		return Request{}, fmt.Errorf("%w: select exactly one document preparation, prepared dataset, or saved dataset version", tmprl.ErrInvalidArgument)
	}
	if r.PreparationOnly && r.Prepare == nil {
		return Request{}, fmt.Errorf("%w: preparation-only requires a source document", tmprl.ErrInvalidArgument)
	}
	if r.Training == (optimization.Training{}) {
		r.Training = optimization.Training{MaximumEpochs: 10, BatchSize: 2, LearningRate: 0.0002, Seed: 3407}
	}
	if r.Training.LoRARank == 0 {
		r.Training.LoRARank = 8
	}
	if r.Training.LoRAAlpha == 0 {
		r.Training.LoRAAlpha = 8
	}
	t := r.Training
	if !r.PreparationOnly && (t.MaximumEpochs < 1 || t.MaximumEpochs > 100 || t.BatchSize < 1 || t.BatchSize > 128 || t.Seed < 0 || t.LoRARank < 1 || t.LoRAAlpha <= 0 || math.IsNaN(t.LoRAAlpha) || math.IsInf(t.LoRAAlpha, 0) || t.LearningRate <= 0 || t.LearningRate > 1 || math.IsNaN(t.LearningRate) || math.IsInf(t.LearningRate, 0)) {
		return Request{}, fmt.Errorf("%w: invalid training hyperparameters", tmprl.ErrInvalidArgument)
	}
	if saved := r.SavedDataset; saved != nil {
		saved.ID = strings.TrimSpace(saved.ID)
		if !datasetstore.ValidID(saved.ID) || saved.Version < 1 {
			return Request{}, fmt.Errorf("%w: saved dataset requires a dataset ID and a positive version", tmprl.ErrInvalidArgument)
		}
		dataset, err := j.dataset(ctx, saved.ID, saved.Version)
		if err != nil {
			return Request{}, err
		}
		if !dataset.TrainingReady {
			return Request{}, fmt.Errorf("%w: dataset version %d is not ready for training", tmprl.ErrInvalidArgument, saved.Version)
		}
	}
	if r.Prepare != nil {
		if len(r.Prepare.ReviewedRecords) != 0 {
			return Request{}, fmt.Errorf("%w: submit reviewed examples through ContinuePreview after Preview", tmprl.ErrInvalidArgument)
		}
		if j.cfg.Completer == nil {
			return Request{}, fmt.Errorf("%w: document preparation requires a completer", tmprl.ErrInvalidArgument)
		}
		prepared, err := preparation.ValidateRequest(ctx, j.cfg.Uploads, *r.Prepare)
		if err != nil {
			return Request{}, err
		}
		r.Prepare = &prepared
		if prepared.TargetDatasetID != "" {
			if !r.PreparationOnly {
				return Request{}, fmt.Errorf("%w: saving to a reusable dataset requires a preparation-only run", tmprl.ErrInvalidArgument)
			}
			dataset, err := j.dataset(ctx, prepared.TargetDatasetID, 0)
			if err != nil {
				return Request{}, err
			}
			if dataset.Version != prepared.BaseVersion {
				return Request{}, datasetstore.ErrDatasetConflict
			}
			// Reject a document the dataset already holds before any model work.
			source, err := j.cfg.Uploads.Get(ctx, prepared.DatasetID)
			if err != nil {
				return Request{}, err
			}
			info, err := j.cfg.Datasets.Inspect(ctx, dataset.ID, dataset.Version)
			if err != nil {
				return Request{}, err
			}
			if info.HasDocument(source.SHA256, prepared.PageStart, prepared.PageEnd) {
				return Request{}, datasetstore.ErrDocumentExists
			}
		}
	} else if _, err := j.records(ctx, r, Artifacts{}, ""); err != nil {
		return Request{}, err
	}
	return r, nil
}

func (j *Job) openArtifact(ctx context.Context, e tmprl.Execution, adapter bool) (*os.File, error) {
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return nil, executionError(err)
	}
	name := "evaluation.json"
	if run.Artifacts.Training.Report == "" || adapter && (run.Status != tmprl.StatusCompleted || !run.Artifacts.Training.Accepted || run.Artifacts.Training.Adapter == "") {
		return nil, fmt.Errorf("%w: training artifact is not available", tmprl.ErrInvalidArgument)
	}
	if adapter {
		name = "adapter.gguf"
	}
	root, err := os.OpenRoot(j.cfg.StorageRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join(storageDirectory, run.RunID, "steps", "training", name))
}

// records reads bounded, confined training input and verifies its provenance.
func (j *Job) records(ctx context.Context, request Request, artifacts Artifacts, runID string) ([]optimization.Record, error) {
	var file io.ReadCloser
	var err error
	limit := int64(100 << 20)
	if saved := request.SavedDataset; saved != nil {
		if _, err := j.dataset(ctx, saved.ID, saved.Version); err != nil {
			return nil, err
		}
		records, err := j.cfg.Datasets.Records(ctx, saved.ID, saved.Version)
		if err != nil {
			return nil, err
		}
		if err := optimization.TrainingReadiness(records); err != nil {
			return nil, fmt.Errorf("%w: saved dataset: %w", tmprl.ErrInvalidArgument, err)
		}
		return records, nil
	}
	if request.Prepare == nil {
		source, sourceErr := j.cfg.Uploads.Get(ctx, request.DatasetID)
		if sourceErr != nil {
			return nil, sourceErr
		}
		if source.Format != ".jsonl" {
			return nil, fmt.Errorf("%w: training requires prepared JSONL", tmprl.ErrInvalidArgument)
		}
		file, err = j.cfg.Uploads.Open(ctx, source.ID)
		limit = j.cfg.Uploads.MaximumBytes()
	} else {
		if artifacts.Preparation == nil || !artifacts.Preparation.TrainingReady || artifacts.Preparation.DatasetID != request.Prepare.DatasetID {
			return nil, fmt.Errorf("%w: preparation is not training-ready", tmprl.ErrInvalidArgument)
		}
		root, rootErr := os.OpenRoot(j.cfg.StorageRoot)
		if rootErr != nil {
			return nil, rootErr
		}
		defer root.Close()
		file, err = root.Open(filepath.Join(storageDirectory, runID, "steps", "preparation", "dataset.jsonl"))
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(tmprl.ContextReader{Context: ctx, Reader: file}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: training input exceeds size limit", tmprl.ErrInvalidArgument)
	}
	records, err := optimization.ReadRecords(ctx, bytes.NewReader(data))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: prepared dataset: %w", tmprl.ErrInvalidArgument, err)
	}
	if request.Prepare != nil && len(records) != artifacts.Preparation.RecordCount {
		return nil, fmt.Errorf("%w: prepared record count changed", tmprl.ErrInvalidArgument)
	}
	if err := optimization.TrainingReadiness(records); err != nil {
		return nil, fmt.Errorf("%w: prepared dataset: %w", tmprl.ErrInvalidArgument, err)
	}
	return records, nil
}

// trainingContext resolves immutable inputs and borrows foundation's workspace.
// It does not own activity identity, heartbeats, retries, or completed-step commits.
func (j *Job) trainingContext(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (optimization.StepContext, error) {
	r := input.Request
	records, err := j.records(ctx, r.Request, r.Artifacts, r.RunID)
	if err != nil {
		return optimization.StepContext{}, err
	}
	modelPath := j.cfg.Models[r.Request.Model]
	file, err := os.Open(modelPath)
	if err != nil {
		return optimization.StepContext{}, err
	}
	metadata, statErr := file.Stat()
	if statErr != nil || !metadata.Mode().IsRegular() {
		return optimization.StepContext{}, errors.Join(statErr, invalidInput("model must be a regular GGUF file"), file.Close())
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, tmprl.ContextReader{Context: ctx, Reader: file})
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return optimization.StepContext{}, err
	}
	identity, err := json.Marshal(struct {
		Request Request
		Records []optimization.Record
		Model   string
	}{r.Request, records, hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		return optimization.StepContext{}, err
	}
	fingerprint := sha256.Sum256(identity)
	key := hex.EncodeToString(fingerprint[:])
	j.cfg.Log(ctx, "training inputs resolved", "workflow_id", r.WorkflowID, "run_id", r.RunID, "step", r.Step, "model_id", r.Request.Model, "records", len(records), "fingerprint", key)
	var state split.State
	data, err := input.Workspace.ReadFile("dataset.json")
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return optimization.StepContext{}, err
		}
		if state.Fingerprint != key {
			return optimization.StepContext{}, invalidInput("training input changed after splitting")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return optimization.StepContext{}, err
	}
	scratch, err := os.MkdirTemp("", "moxie-training-")
	if err != nil {
		return optimization.StepContext{}, err
	}
	return optimization.StepContext{
		Artifacts: r.Artifacts.Training, Workspace: input.Workspace, Trainer: j.cfg.Trainer,
		Job:     optimization.TrainingJob{ModelID: r.Request.Model, Model: modelPath, Training: r.Request.Training, Dataset: state.Dataset, Workdir: scratch},
		Records: records, Fingerprint: key, Publish: input.Publish,
	}, nil
}

// steps registers capabilities. Only job.yaml specifies their order and policies.
func (j *Job) steps() map[string]tmprl.StepHandler[Request, Artifacts] {
	return map[string]tmprl.StepHandler[Request, Artifacts]{
		"preparation/extract":  j.prepare(extract.Step{}),
		"preparation/preview":  j.prepare(preview.Step{}),
		"preparation/generate": j.prepare(generate.Step{Verify: verify.Batch}),
		"preparation/verify":   j.prepare(verify.Step{}),
		"training/split":       j.optimize(split.Step{}),
		"training/train":       j.optimize(train.Step{}),
		"training/evaluate":    j.optimize(evaluate.Step{}),
		"training/publish":     j.optimize(publish.Step{}),
	}
}

func (j *Job) prepare(step preparation.StepHandler) tmprl.StepHandler[Request, Artifacts] {
	return tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
		r := input.Request
		if r.Request.Prepare == nil {
			return tmprl.StepResult[Artifacts]{}, invalidInput("document preparation input is required")
		}
		source, err := j.cfg.Uploads.Get(ctx, r.Request.Prepare.DatasetID)
		if err != nil {
			return tmprl.StepResult[Artifacts]{}, stepError(err)
		}
		request := *r.Request.Prepare
		if step.Name() == "preview" {
			request.PreviewRecords = cmp.Or(request.PreviewRecords, 3)
			request.ReviewedRecords = nil
		}
		var review *preparation.Review
		var reviewID string
		if r.Artifacts.Preview != nil && (step.Name() == "generate" || step.Name() == "verify") {
			for _, decision := range r.Decisions {
				if decision.CheckpointID == r.RunID+"/preparation/preview/preview-review" {
					accepted, err := preview.ValidateReview(request, r.Artifacts.Preview, decision.Payload)
					if err != nil {
						return tmprl.StepResult[Artifacts]{}, stepError(err)
					}
					review, reviewID = &accepted, decision.SubmissionID
				}
			}
			if review == nil {
				return tmprl.StepResult[Artifacts]{}, invalidInput("Preview review must be accepted before full preparation")
			}
		}
		var datasetRecords []optimization.Record
		if step.Name() == "generate" && request.TargetDatasetID != "" && request.BaseVersion > 0 {
			if datasetRecords, err = j.cfg.Datasets.Records(ctx, request.TargetDatasetID, request.BaseVersion); err != nil {
				return tmprl.StepResult[Artifacts]{}, stepError(err)
			}
		}
		output, err := step.Execute(ctx, preparation.StepContext{
			Preparation: &preparation.PreparationContext{
				Input:  preparation.PreparationInput{Request: request, Kind: preparation.PreparationFull},
				Source: source, Extractor: j.cfg.Extractor, Completer: j.cfg.Completer, DatasetRecords: datasetRecords,
				OpenSource: func(ctx context.Context) (io.ReadCloser, error) {
					return j.cfg.Uploads.Open(ctx, source.ID)
				},
			},
			Workspace: input.Workspace, Artifacts: preparation.Artifacts{Preview: r.Artifacts.Preview, Preparation: r.Artifacts.Preparation},
			Review: review, ReviewID: reviewID, Publish: input.Publish, SafePoint: input.SafePoint,
		})
		artifacts := r.Artifacts
		artifacts.Preview = output.Artifacts.Preview
		artifacts.Preparation = output.Artifacts.Preparation
		if err == nil && step.Name() == "verify" && request.TargetDatasetID != "" && artifacts.Preparation != nil {
			if input.SafePoint != nil {
				if err := input.SafePoint(ctx, "result.json"); err != nil {
					return tmprl.StepResult[Artifacts]{}, err
				}
			}
			file, openErr := input.Workspace.Open("dataset.jsonl")
			if openErr != nil {
				return tmprl.StepResult[Artifacts]{}, openErr
			}
			records, readErr := optimization.ReadRecords(ctx, file)
			file.Close()
			if readErr != nil {
				return tmprl.StepResult[Artifacts]{}, stepError(readErr)
			}
			saved, saveErr := j.saveDataset(ctx, request.TargetDatasetID, request.BaseVersion, datasetstore.Addition{
				PublicationID: r.RunID, Source: source, PageStart: request.PageStart, PageEnd: request.PageEnd, Records: records,
			})
			if saveErr != nil {
				return tmprl.StepResult[Artifacts]{}, stepError(saveErr)
			}
			artifacts.Preparation.SavedDatasetID, artifacts.Preparation.SavedVersion = saved.ID, saved.Version
			artifacts.Preparation.SavedRecordCount, artifacts.Preparation.SavedTrainingReady = saved.RecordCount, saved.TrainingReady
			// The store skips questions the dataset already has.
			if i := slices.IndexFunc(saved.Documents, func(d datasetstore.Document) bool { return d.ID == r.RunID }); i >= 0 {
				artifacts.Preparation.SavedSkippedRecords = len(records) - saved.Documents[i].RecordCount
			}
		}
		return tmprl.StepResult[Artifacts]{Artifacts: artifacts, Status: output.Status}, stepError(err)
	})
}

func (j *Job) optimize(step optimization.StepHandler) tmprl.StepHandler[Request, Artifacts] {
	return tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
		resources, err := j.trainingContext(ctx, input)
		if err != nil {
			return tmprl.StepResult[Artifacts]{}, stepError(err)
		}
		defer os.RemoveAll(resources.Job.Workdir)
		output, err := step.Execute(ctx, resources)
		artifacts := input.Request.Artifacts
		artifacts.Training = output.Artifacts
		return tmprl.StepResult[Artifacts]{Artifacts: artifacts, Status: output.Status}, stepError(err)
	})
}

// =============================================================================

// dataset resolves SQL-published versions before opening immutable files.
func (j *Job) dataset(ctx context.Context, id string, version int) (datasetstore.Dataset, error) {
	if j.cfg.MoxieDB == nil {
		return j.cfg.Datasets.Get(ctx, id, version)
	}
	v, err := j.cfg.MoxieDB.GetVersion(ctx, id, version)
	if errors.Is(err, moxiedb.ErrNotFound) {
		return datasetstore.Dataset{}, datasetstore.ErrDatasetNotFound
	}
	if err != nil {
		return datasetstore.Dataset{}, err
	}
	info, err := j.cfg.Datasets.Lookup(ctx, id, v.Version)
	if err != nil {
		return datasetstore.Dataset{}, err
	}
	path, err := filepath.Rel(j.cfg.StorageRoot, filepath.Join(j.cfg.Datasets.Path(), info.Path))
	if err != nil || !filepath.IsLocal(path) || filepath.ToSlash(path) != v.SnapshotPath || info.SHA256 != v.SnapshotSHA256 || info.Size != v.SnapshotBytes {
		return datasetstore.Dataset{}, fmt.Errorf("%w: saved dataset integrity mismatch", datasetstore.ErrDatasetConflict)
	}
	return info.Dataset, nil
}

// saveDataset preserves verified Add idempotency while persisting publication
// intent before touching files. Jobs call SQL directly, not datasets.Service.
func (j *Job) saveDataset(ctx context.Context, id string, base int, addition datasetstore.Addition) (datasetstore.Dataset, error) {
	if j.cfg.MoxieDB == nil {
		return j.cfg.Datasets.Add(ctx, id, base, addition)
	}
	data, err := json.Marshal(addition)
	if err != nil {
		return datasetstore.Dataset{}, err
	}
	digest := sha256.Sum256(data)
	operation := "add/" + addition.PublicationID
	var saved datasetstore.Dataset
	err = j.cfg.Datasets.WithLock(ctx, func(ctx context.Context) error {
		previous, err := j.cfg.MoxieDB.GetPublication(ctx, operation)
		switch {
		case err == nil && (previous.InputDigest != hex.EncodeToString(digest[:]) || previous.DatasetID != id || previous.BaseVersion != base || previous.Kind != "add"):
			return datasetstore.ErrDatasetConflict
		case err == nil && previous.State == "committed":
			saved, err = j.dataset(ctx, id, previous.TargetVersion)
			return err
		case err != nil && !errors.Is(err, moxiedb.ErrNotFound):
			return err
		}
		latest, err := j.cfg.Datasets.Inspect(ctx, id, 0)
		if err != nil {
			return err
		}
		if latest.Operation != operation {
			published, err := j.dataset(ctx, id, 0)
			if err != nil {
				return err
			}
			if published.Version != latest.Dataset.Version {
				return fmt.Errorf("%w: another dataset publication requires reconciliation", datasetstore.ErrDatasetConflict)
			}
		}
		pending, err := j.cfg.MoxieDB.PendingPublications(ctx)
		if err != nil {
			return err
		}
		for _, publication := range pending {
			if publication.DatasetID == id && publication.ID != operation {
				return fmt.Errorf("%w: another dataset publication requires reconciliation", datasetstore.ErrDatasetConflict)
			}
		}
		if err := j.cfg.MoxieDB.BeginPublication(ctx, moxiedb.Publication{ID: operation, DatasetID: id, Kind: "add", BaseVersion: base, InputDigest: hex.EncodeToString(digest[:])}); err != nil {
			return err
		}
		saved, err = j.cfg.Datasets.Add(ctx, id, base, addition)
		if err != nil {
			if errors.Is(err, tmprl.ErrInvalidArgument) || errors.Is(err, datasetstore.ErrDatasetConflict) || errors.Is(err, datasetstore.ErrDatasetNotFound) {
				return errors.Join(err, j.cfg.MoxieDB.MarkPublication(ctx, operation, "conflict", "invalid_or_stale_input"))
			}
			return err
		}
		info, err := j.cfg.Datasets.Inspect(ctx, saved.ID, saved.Version)
		if err != nil {
			return err
		}
		dataset, version, documents, err := j.toDBSnapshot(info, operation)
		if err != nil {
			return err
		}
		if err := j.cfg.MoxieDB.CommitVersion(ctx, dataset, version, documents); err != nil {
			return err
		}
		if err := j.cfg.Datasets.BuildIndex(ctx); err != nil {
			return fmt.Errorf("dataset %s version %d is saved; index refresh failed: %w", saved.ID, saved.Version, err)
		}
		return nil
	})
	return saved, err
}

func (j *Job) toDBSnapshot(info datasetstore.SnapshotInfo, operation string) (moxiedb.Dataset, moxiedb.Version, []moxiedb.Document, error) {
	d := info.Dataset
	path, err := filepath.Rel(j.cfg.StorageRoot, filepath.Join(j.cfg.Datasets.Path(), info.Path))
	if err != nil || !filepath.IsLocal(path) {
		return moxiedb.Dataset{}, moxiedb.Version{}, nil, fmt.Errorf("dataset snapshot outside Moxie root")
	}
	dataset := moxiedb.Dataset{ID: d.ID, Name: d.Name, CurrentVersion: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	version := moxiedb.Version{DatasetID: d.ID, Version: d.Version, SnapshotPath: filepath.ToSlash(path), SnapshotSHA256: info.SHA256, SnapshotBytes: info.Size, RecordCount: int64(d.RecordCount), TrainingReady: d.TrainingReady, CreatedAt: d.UpdatedAt, PublicationID: operation}
	documents := make([]moxiedb.Document, 0, len(info.Documents))
	for i, c := range info.Documents {
		documents = append(documents, moxiedb.Document{ID: c.Document.ID, Name: c.Document.Name, SourceSHA256: c.SHA256, Position: i, PageStart: c.Document.PageStart, PageEnd: c.Document.PageEnd, RecordCount: int64(c.Document.RecordCount)})
	}
	return dataset, version, documents, nil
}

func invalidInput(message string) error {
	return temporal.NewNonRetryableApplicationError(message, "invalid-training", nil)
}

// Failure types the application reports to users; any other failure is shown
// generically because its message may include private source text.
const (
	failureDocumentExists = "dataset-document-exists"
	failureQuestionsExist = "dataset-questions-exist"
	failureDatasetChanged = "dataset-changed"
	failureNoExamples     = "preparation-no-examples"

	// Types raised by the preparation steps and request validation.
	failureNoSourceUnits     = "preparation-no-source-units"
	failureRequestBudget     = "preparation-request-budget-exhausted"
	failureNoVerified        = "preparation-no-verified-examples"
	failureCheckpointVersion = "preparation-checkpoint-version"
	failureInvalidRequest    = "invalid-training"

	// Types raised when the document extractor fails.
	failureExtractorUnavailable = "preparation-extractor-unavailable"
	failurePageRange            = "preparation-page-range"
	failureExtractionFailed     = "preparation-extraction-failed"
)

// FailureReason returns the first known application failure type of a run.
func FailureReason(types []string) string {
	for _, t := range types {
		switch t {
		case failureDocumentExists, failureQuestionsExist, failureDatasetChanged, failureNoExamples,
			failureNoSourceUnits, failureRequestBudget, failureNoVerified, failureCheckpointVersion, failureInvalidRequest,
			failureExtractorUnavailable, failurePageRange, failureExtractionFailed, "training-records-insufficient":
			return t
		}
	}
	return ""
}

func stepError(err error) error {
	switch {
	case errors.Is(err, datasetstore.ErrDocumentExists):
		return temporal.NewNonRetryableApplicationError(err.Error(), failureDocumentExists, err)
	case errors.Is(err, datasetstore.ErrQuestionsExist):
		return temporal.NewNonRetryableApplicationError(err.Error(), failureQuestionsExist, err)
	case errors.Is(err, datasetstore.ErrDatasetConflict), errors.Is(err, datasetstore.ErrDatasetNotFound):
		return temporal.NewNonRetryableApplicationError(err.Error(), failureDatasetChanged, err)
	case errors.Is(err, providers.ErrExtractorUnavailable):
		return temporal.NewApplicationError(err.Error(), failureExtractorUnavailable, err)
	case errors.Is(err, providers.ErrPageRangePastEnd):
		return temporal.NewNonRetryableApplicationError(err.Error(), failurePageRange, err)
	case errors.Is(err, providers.ErrExtractionFailed):
		return temporal.NewNonRetryableApplicationError(err.Error(), failureExtractionFailed, err)
	}
	if errors.Is(err, tmprl.ErrInvalidArgument) || errors.Is(err, uploads.ErrInvalidArgument) || errors.Is(err, uploads.ErrNotFound) || errors.Is(err, preparation.ErrInvalidArgument) || errors.Is(err, datasetstore.ErrDatasetConflict) || errors.Is(err, datasetstore.ErrDatasetNotFound) {
		return temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	return err
}
