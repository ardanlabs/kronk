package tmprl

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// ErrInvalidArgument identifies invalid job definitions, registration, or filters.
var ErrInvalidArgument = errors.New("invalid temporal job argument")

// StepContext contains worker-local resources; only Request crosses Temporal.
// Handlers must honor cancellation and deduplicate external effects by Request.Key.
type StepContext[R, A any] struct {
	Request   StepRequest[R, A]
	Workspace *os.Root
	Publish   func(Progress)
	// SafePoint is non-nil only for opted-in checkpointed steps. Call after
	// committing progress and before starting another unit. An error must be
	// returned unchanged; it may suspend this activity without completing it.
	SafePoint func(context.Context, string) error
}

// StepHandler executes application computation without scheduling Temporal work.
type StepHandler[R, A any] interface {
	Execute(context.Context, StepContext[R, A]) (StepResult[A], error)
}

// StepFunc adapts a function to StepHandler.
type StepFunc[R, A any] func(context.Context, StepContext[R, A]) (StepResult[A], error)

// Execute calls the registered function.
func (f StepFunc[R, A]) Execute(ctx context.Context, input StepContext[R, A]) (StepResult[A], error) {
	return f(ctx, input)
}

// Config supplies application identity, storage and capabilities, not execution
// order. Order and execution policies come from the submitted job definition.
type Config[R, A any] struct {
	Names     Names
	TaskQueue string
	// Queue routes a request to a task queue; nil or empty uses TaskQueue. Each
	// queue's worker holds one job at a time, so jobs on different queues run
	// side by side.
	Queue            func(R) string
	StorageRoot      string
	StorageDirectory string
	Steps            map[string]StepHandler[R, A]
	// Outcomes declares additional successful terminal outcomes (for example,
	// a review job may return "rejected"). Empty status continues the next step.
	Outcomes []Status
	Log      applog.Logger
	Progress *ProgressBroker[Execution, Progress]
	// ResultActivities and ArtifactActivities support reading older histories
	// whose results use StepResult[A] and bare A respectively. New jobs only
	// schedule Names.Activity. Existing registrations remain application-owned.
	ResultActivities   []string
	ArtifactActivities []string
}

// Jobs owns reusable job execution and management. It borrows the client and
// storage; the host owns their lifetime and starts/stops returned workers.
type Jobs[R, A any] struct {
	client client.Client
	cfg    Config[R, A]
	// drain owns host shutdown for this process's workers. Once draining
	// starts, opted-in steps suspend at their next durable safe point.
	drain struct {
		sync.Mutex
		started bool
		active  int
		idle    chan struct{}
	}
}

// New constructs job infrastructure without contacting Temporal or loading models.
func New[R, A any](c client.Client, cfg Config[R, A]) (*Jobs[R, A], error) {
	if c == nil || strings.TrimSpace(cfg.TaskQueue) == "" || len(cfg.TaskQueue) > 200 {
		return nil, fmt.Errorf("%w: client and a valid task queue are required", ErrInvalidArgument)
	}
	for _, name := range []string{cfg.Names.Workflow, cfg.Names.Activity, cfg.Names.Memo, cfg.Names.Query, cfg.Names.Control} {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "'\\") {
			return nil, fmt.Errorf("%w: nonempty protocol names without quotes or backslashes are required", ErrInvalidArgument)
		}
	}
	if cfg.StorageRoot == "" || !ValidName(cfg.StorageDirectory) {
		return nil, fmt.Errorf("%w: storage root and a safe storage directory are required", ErrInvalidArgument)
	}
	for name, handler := range cfg.Steps {
		stage, step, ok := strings.Cut(name, "/")
		if !ok || !ValidName(stage) || !ValidName(step) || handler == nil {
			return nil, fmt.Errorf("%w: invalid step registration %q", ErrInvalidArgument, name)
		}
	}
	for _, outcome := range cfg.Outcomes {
		if !ValidName(string(outcome)) || !outcome.Terminal() || outcome == StatusFailed || outcome == StatusStopped || outcome == StatusCompleted {
			return nil, fmt.Errorf("%w: invalid application outcome %q", ErrInvalidArgument, outcome)
		}
	}
	if cfg.Log == nil {
		cfg.Log = applog.FmtLogger
	}
	if cfg.Progress == nil {
		cfg.Progress = new(ProgressBroker[Execution, Progress])
	}
	cfg.Steps = maps.Clone(cfg.Steps)
	cfg.Outcomes = slices.Clone(cfg.Outcomes)
	cfg.ResultActivities = slices.Clone(cfg.ResultActivities)
	cfg.ArtifactActivities = slices.Clone(cfg.ArtifactActivities)
	j := Jobs[R, A]{client: c, cfg: cfg}
	return &j, nil
}

// ValidatePlan checks policies and ensures every submitted step can run locally.
func (j *Jobs[R, A]) ValidatePlan(job Job) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("%w: job definition: %s", ErrInvalidArgument, err)
	}
	for _, stage := range job.Stages {
		for _, step := range stage.Steps {
			if j.cfg.Steps[stage.Name+"/"+step.Name] == nil {
				return fmt.Errorf("%w: unregistered step %s/%s", ErrInvalidArgument, stage.Name, step.Name)
			}
		}
	}
	return nil
}

// Submit starts an exact, non-reusable workflow ID with a durable queued snapshot.
// The application validates/normalizes Request and chooses ID before this call.
func (j *Jobs[R, A]) Submit(ctx context.Context, id string, input JobInput[R]) (Run[R, A], error) {
	if strings.TrimSpace(id) == "" || len(id) > 200 || input.Job == nil {
		return Run[R, A]{}, fmt.Errorf("%w: ID and job definition are required", ErrInvalidArgument)
	}
	if err := j.ValidatePlan(*input.Job); err != nil {
		return Run[R, A]{}, err
	}
	if input.ControlStage != "" && (len(input.Job.Stages) != 1 || input.Job.Stages[0].Name != input.ControlStage) {
		return Run[R, A]{}, fmt.Errorf("%w: ordered controls require a single matching stage", ErrInvalidArgument)
	}
	queue := j.cfg.TaskQueue
	if j.cfg.Queue != nil {
		queue = cmp.Or(j.cfg.Queue(input.Request), queue)
	}
	state := QueuedJob[R, A](input)
	e, err := Start(ctx, j.client, client.StartWorkflowOptions{
		ID: id, TaskQueue: queue, Memo: map[string]any{j.cfg.Names.Memo: state},
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, j.cfg.Names.Workflow, input)
	if err != nil {
		return Run[R, A]{}, fmt.Errorf("starting job: %w", err)
	}
	state.Execution = e
	j.cfg.Log(ctx, "job submitted", "job", input.Job.Name, "workflow_id", e.WorkflowID, "run_id", e.RunID, "task_queue", queue, "stages", len(input.Job.Stages))
	return state, nil
}

// Workflow executes the submitted definition. Applications with historical
// workflow payloads may register their own dispatcher and call RunJob instead.
func (j *Jobs[R, A]) Workflow(ctx workflow.Context, input JobInput[R]) (Run[R, A], error) {
	return RunJob[R, A](ctx, input, j.cfg.Names)
}

// Drain prepares this process for shutdown before its workers stop. Opted-in
// steps suspend at their next durable safe point without completing their step
// or consuming a worker-recovery attempt; other steps are unaffected. Drain
// returns when no opted-in step is running here, or ctx's error. The host then
// stops its workers; an unfinished unit repeats after restart.
func (j *Jobs[R, A]) Drain(ctx context.Context) error {
	j.drain.Lock()
	j.drain.started = true
	if j.drain.active == 0 {
		j.drain.Unlock()
		return nil
	}
	if j.drain.idle == nil {
		j.drain.idle = make(chan struct{})
	}
	idle := j.drain.idle
	j.drain.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// beginSafePointStep tracks a running opted-in step until Drain may return.
func (j *Jobs[R, A]) beginSafePointStep() (done func()) {
	j.drain.Lock()
	defer j.drain.Unlock()
	j.drain.active++
	return func() {
		j.drain.Lock()
		defer j.drain.Unlock()
		j.drain.active--
		if j.drain.active == 0 && j.drain.idle != nil {
			close(j.drain.idle)
			j.drain.idle = nil
		}
	}
}

func (j *Jobs[R, A]) draining() bool {
	j.drain.Lock()
	defer j.drain.Unlock()
	return j.drain.started
}

// WorkerOptions reserves one worker for one complete job, including pauses.
func (j *Jobs[R, A]) WorkerOptions() worker.Options {
	return worker.Options{
		WorkerStopTimeout: 5 * time.Second, EnableSessionWorker: true,
		MaxConcurrentSessionExecutionSize: 1, MaxConcurrentActivityExecutionSize: 1,
	}
}

// Register attaches the shared workflow and step activity to a host-owned worker.
func (j *Jobs[R, A]) Register(w worker.Registry) {
	w.RegisterWorkflowWithOptions(j.Workflow, workflow.RegisterOptions{Name: j.cfg.Names.Workflow})
	j.RegisterActivity(w)
}

// RegisterActivity supports applications that retain a historical workflow dispatcher.
func (j *Jobs[R, A]) RegisterActivity(w worker.Registry) {
	w.RegisterActivityWithOptions(j.ExecuteStep, activity.RegisterOptions{Name: j.cfg.Names.Activity})
}

// NewWorker creates a worker for TaskQueue; its Start/Run and Stop calls belong to the host.
func (j *Jobs[R, A]) NewWorker() worker.Worker {
	return j.NewQueueWorker(j.cfg.TaskQueue)
}

// NewQueueWorker creates a worker for a queue that Config.Queue returns.
func (j *Jobs[R, A]) NewQueueWorker(queue string) worker.Worker {
	return NewWorker(j.client, queue, j.WorkerOptions(), j.Register)
}

// Subscribe receives local progress; use Get for durable cross-worker state.
func (j *Jobs[R, A]) Subscribe(e Execution) (<-chan Progress, func()) {
	return j.cfg.Progress.Subscribe(e)
}

// Pause takes effect after the active step commits, retaining the reserved worker.
func (j *Jobs[R, A]) Pause(ctx context.Context, e Execution) error {
	return j.control(ctx, e, "pause")
}

// Resume continues the same run without repeating committed steps.
func (j *Jobs[R, A]) Resume(ctx context.Context, e Execution) error {
	return j.control(ctx, e, "resume")
}

// Control uses a stable Temporal update identity. The server retains its result
// across worker loss and workflow closure; request-ID reuse is checked against
// the original receipt rather than silently accepting a different command.
func (j *Jobs[R, A]) Control(ctx context.Context, e Execution, request ControlRequest) (ControlReceipt, error) {
	if err := request.Validate(); err != nil {
		return ControlReceipt{}, err
	}
	run, err := j.Get(ctx, e)
	if err != nil {
		return ControlReceipt{}, err
	}
	if run.ControlStage == "" {
		return ControlReceipt{}, ErrControlConflict
	}
	id := "ordered-control/" + request.RequestID
	handle, err := j.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID: e.WorkflowID, RunID: e.RunID, UpdateID: id,
		UpdateName: j.cfg.Names.Control + ".ordered", Args: []any{request},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		// A closed execution cannot accept a new update. Resolve only a stored
		// receipt, never submit a new execution or substitute the current run.
		if _, closed := errors.AsType[*serviceerror.NotFound](err); !closed {
			return ControlReceipt{}, err
		}
		handle = j.client.GetWorkflowUpdateHandle(client.GetWorkflowUpdateHandleOptions{WorkflowID: e.WorkflowID, RunID: e.RunID, UpdateID: id})
	}
	var receipt ControlReceipt
	if err := handle.Get(ctx, &receipt); err != nil {
		if _, missing := errors.AsType[*serviceerror.NotFound](err); missing && run.Status.Terminal() {
			return ControlReceipt{}, ErrControlConflict
		}
		if failure, ok := errors.AsType[*temporal.ApplicationError](err); ok && failure.Type() == "control-conflict" {
			return ControlReceipt{}, ErrControlConflict
		}
		return ControlReceipt{}, err
	}
	if receipt.ControlRequest != request || receipt.Revision != request.ExpectedRevision+1 {
		return ControlReceipt{}, ErrControlConflict
	}
	return receipt, nil
}

// Validate checks the decision envelope, not application-specific approval rules.
func (d Decision) Validate() error {
	if strings.TrimSpace(d.CheckpointID) == "" || len(d.CheckpointID) > 512 || strings.TrimSpace(d.SubmissionID) == "" || len(d.SubmissionID) > 200 {
		return fmt.Errorf("%w: checkpoint ID (1–512 bytes) and submission ID (1–200 bytes) are required", ErrInvalidArgument)
	}
	if len(d.Payload) > 64<<10 || len(d.Payload) > 0 && !json.Valid(d.Payload) {
		return fmt.Errorf("%w: decision payload must be valid JSON of at most 64 KiB", ErrInvalidArgument)
	}
	return nil
}

// Decide returns a durable acceptance receipt, not completion of subsequent work.
// A caller timeout is ambiguous: retry the same submission or read Get.
func (j *Jobs[R, A]) Decide(ctx context.Context, e Execution, decision Decision) (Decision, error) {
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	if err := ValidateWorkflow(ctx, j.client, e, j.cfg.Names.Workflow); err != nil {
		return Decision{}, err
	}
	handle, err := j.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID: e.WorkflowID, RunID: e.RunID, UpdateName: j.cfg.Names.Control + ".decide",
		Args: []any{decision}, WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return Decision{}, err
	}
	var receipt Decision
	if err := handle.Get(ctx, &receipt); err != nil {
		return Decision{}, err
	}
	return receipt, nil
}

func (j *Jobs[R, A]) control(ctx context.Context, e Execution, command string) error {
	err := Signal(ctx, j.client, e, j.cfg.Names.Control, command, j.cfg.Names.Workflow)
	j.cfg.Log(ctx, "job control submitted", "workflow_id", e.WorkflowID, "run_id", e.RunID, "command", command, "succeeded", err == nil)
	return err
}

// Stop cooperatively cancels active work; Wait/Get establish terminal state.
func (j *Jobs[R, A]) Stop(ctx context.Context, e Execution) error {
	err := Stop(ctx, j.client, e, j.cfg.Names.Workflow)
	j.cfg.Log(ctx, "job cancellation submitted", "workflow_id", e.WorkflowID, "run_id", e.RunID, "succeeded", err == nil)
	return err
}

// Wait returns the workflow result or its Temporal failure, without a live query.
func (j *Jobs[R, A]) Wait(ctx context.Context, e Execution) (Run[R, A], error) {
	var state Run[R, A]
	err := Wait(ctx, j.client, e, &state, j.cfg.Names.Workflow)
	return state, err
}

// History pages execution identities without querying workers.
func (j *Jobs[R, A]) History(ctx context.Context, pageSize int, next []byte) ([]Execution, []byte, error) {
	return History(ctx, j.client, j.cfg.Names.Workflow, pageSize, next)
}

func (j *Jobs[R, A]) openWorkspace(runID, stage string) (*os.Root, error) {
	root, err := os.OpenRoot(j.cfg.StorageRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	path := filepath.Join(j.cfg.StorageDirectory, runID, "steps", stage)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return nil, err
	}
	return root.OpenRoot(path)
}
