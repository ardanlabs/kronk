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
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
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
	Preview     *preparation.PreparationResult `json:"preview,omitzero"`
	Preparation *preparation.PreparationResult `json:"preparation,omitzero"`
	Training    optimization.Artifacts         `json:"training"`
}

// Config supplies worker-local training resources, not execution policies.
// Models maps approved identifiers to GGUF paths. Trainer defaults to GoMLX.
// ModelPool supplies KMS inference for the default trainer and completer.
type Config struct {
	TaskQueue   string
	Uploads     *uploads.Store
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
	cfg  Config
	jobs *tmprl.Jobs[Request, Artifacts]
}

// New constructs the job without contacting Temporal or loading a model.
// The host owns the client, providers, storage, and worker lifetimes.
func New(c client.Client, cfg Config) (*Job, error) {
	if cfg.Uploads == nil || cfg.StorageRoot == "" || len(cfg.Models) == 0 {
		return nil, fmt.Errorf("%w: uploads, execution storage root, and approved models are required", tmprl.ErrInvalidArgument)
	}
	root, err := filepath.Abs(cfg.StorageRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving execution storage: %w", err)
	}
	cfg.StorageRoot = root
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
	j := Job{cfg: cfg}
	jobs, err := tmprl.New(c, tmprl.Config[Request, Artifacts]{
		Names:     tmprl.Names{Workflow: "moxie.training.Job", Activity: "moxie.training.Step", Memo: "moxie.training.job", Query: "training-state", Control: "training-control"},
		TaskQueue: cfg.TaskQueue, StorageRoot: cfg.StorageRoot, StorageDirectory: storageDirectory,
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

// Jobs exposes foundation's worker, status, history, progress, and control APIs.
// Submit training requests through Start so training validation is applied.
func (j *Job) Jobs() *tmprl.Jobs[Request, Artifacts] { return j.jobs }

// Start submits the embedded plan after validating and copying training inputs.
// Prepared JSONL skips preparation. Documents wait for Preview review and for
// explicit authorization to enter Training, using one reserved worker.
func (j *Job) Start(ctx context.Context, input Request) (tmprl.Run[Request, Artifacts], error) {
	input, err := j.validate(ctx, input)
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
	return j.jobs.Submit(ctx, input.ID, tmprl.JobInput[Request]{Job: &plan, Request: input})
}

// ContinuePreview accepts edits to the saved examples and releases Preview.
// The receipt acknowledges accepted input, not completion of the full dataset.
func (j *Job) ContinuePreview(ctx context.Context, e tmprl.Execution, decision tmprl.Decision) (tmprl.Decision, error) {
	if err := decision.Validate(); err != nil {
		return tmprl.Decision{}, err
	}
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return tmprl.Decision{}, err
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
	return j.jobs.Decide(ctx, e, decision)
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
		return tmprl.Decision{}, err
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
	return j.jobs.Decide(ctx, e, decision)
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

// =============================================================================

func (j *Job) validate(ctx context.Context, r Request) (Request, error) {
	r.ID, r.Model, r.DatasetID = strings.TrimSpace(r.ID), strings.TrimSpace(r.Model), strings.TrimSpace(r.DatasetID)
	if len(r.ID) > 200 || j.cfg.Models[r.Model] == "" {
		return Request{}, fmt.Errorf("%w: approved model and ID of at most 200 bytes are required", tmprl.ErrInvalidArgument)
	}
	if (r.Prepare == nil) == (r.DatasetID == "") {
		return Request{}, fmt.Errorf("%w: select exactly one document preparation or prepared dataset", tmprl.ErrInvalidArgument)
	}
	if r.Training == (optimization.Training{}) {
		r.Training = optimization.Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407}
	}
	if r.Training.LoRARank == 0 {
		r.Training.LoRARank = 8
	}
	if r.Training.LoRAAlpha == 0 {
		r.Training.LoRAAlpha = 8
	}
	t := r.Training
	if t.MaximumEpochs < 1 || t.MaximumEpochs > 100 || t.BatchSize < 1 || t.BatchSize > 128 || t.Seed < 0 || t.LoRARank < 1 || t.LoRAAlpha <= 0 || math.IsNaN(t.LoRAAlpha) || math.IsInf(t.LoRAAlpha, 0) || t.LearningRate <= 0 || t.LearningRate > 1 || math.IsNaN(t.LearningRate) || math.IsInf(t.LearningRate, 0) {
		return Request{}, fmt.Errorf("%w: invalid training hyperparameters", tmprl.ErrInvalidArgument)
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
	} else if _, err := j.records(ctx, r, Artifacts{}, ""); err != nil {
		return Request{}, err
	}
	return r, nil
}

func (j *Job) openArtifact(ctx context.Context, e tmprl.Execution, adapter bool) (*os.File, error) {
	run, err := j.jobs.Get(ctx, e)
	if err != nil {
		return nil, err
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
		"preparation/generate": j.prepare(generate.Step{}),
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
		output, err := step.Execute(ctx, preparation.StepContext{
			Preparation: &preparation.PreparationContext{
				Input:  preparation.PreparationInput{Request: request, Kind: preparation.PreparationFull},
				Source: source, Extractor: j.cfg.Extractor, Completer: j.cfg.Completer,
				OpenSource: func(ctx context.Context) (io.ReadCloser, error) {
					return j.cfg.Uploads.Open(ctx, source.ID)
				},
			},
			Workspace: input.Workspace, Artifacts: preparation.Artifacts{Preview: r.Artifacts.Preview, Preparation: r.Artifacts.Preparation},
			Review: review, ReviewID: reviewID, Publish: input.Publish,
		})
		artifacts := r.Artifacts
		artifacts.Preview = output.Artifacts.Preview
		artifacts.Preparation = output.Artifacts.Preparation
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

func invalidInput(message string) error {
	return temporal.NewNonRetryableApplicationError(message, "invalid-training", nil)
}

func stepError(err error) error {
	if errors.Is(err, tmprl.ErrInvalidArgument) || errors.Is(err, uploads.ErrInvalidArgument) || errors.Is(err, uploads.ErrNotFound) {
		return temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	return err
}
