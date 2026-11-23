package training_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	datasetstore "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	job "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

type deletionService struct {
	workflowservice.WorkflowServiceClient
	fail bool
}

func (s *deletionService) DeleteWorkflowExecution(context.Context, *workflowservice.DeleteWorkflowExecutionRequest, ...grpc.CallOption) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	if s.fail {
		return nil, serviceerror.NewUnavailable("temporary deletion outage")
	}
	return &workflowservice.DeleteWorkflowExecutionResponse{}, nil
}

type deletionClient struct {
	expiredCatalogClient
	service *deletionService
}

func (c deletionClient) WorkflowService() workflowservice.WorkflowServiceClient { return c.service }

func TestRunDeletionSharedVersionsAndRecovery(t *testing.T) {
	db, _ := newCatalogDatabase(t)
	ctx := t.Context()
	root := t.TempDir()
	uploadsStore, err := uploads.New(uploads.Config{Root: filepath.Join(root, "uploads")})
	if err != nil {
		t.Fatal(err)
	}
	files, err := datasetstore.New(datasetstore.Config{Root: filepath.Join(root, "preparation-datasets")})
	if err != nil {
		t.Fatal(err)
	}
	service := &deletionService{}
	j, err := job.New(deletionClient{service: service}, job.Config{Uploads: uploadsStore, Datasets: files, MoxieDB: db, StorageRoot: root, Trainer: &trainer{}})
	if err != nil {
		t.Fatal(err)
	}
	datasetsService := datasets.New(datasets.Config{Files: files, Job: j, DB: db})
	d, err := datasetsService.Create(ctx, "Shared harbor")
	if err != nil {
		t.Fatal(err)
	}
	add := func(id, hash, prompt string, base int) {
		t.Helper()
		_, err := datasetsService.Add(ctx, d.ID, base, datasetstore.Addition{PublicationID: id, Source: uploads.File{ID: "source-" + id, Name: id + ".txt", SHA256: strings.Repeat(hash, 64)}, Records: []optimization.Record{{Prompt: prompt, Target: "Dawn.", Evidence: "The harbor opens at dawn.", SourceDocument: "source-" + id}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	add("prep-a", "a", "When does harbor A open?", 0)
	add("prep-b", "b", "When does harbor B open?", 1)
	seed := func(id, mode, status string, version int) tmprl.Execution {
		t.Helper()
		e := tmprl.Execution{WorkflowID: "workflow-" + id, RunID: id}
		if err := db.CreateJob(ctx, moxiedb.Job{JobID: id, Mode: mode, TargetDatasetID: sql.NullString{String: d.ID, Valid: true}, BaseDatasetVersion: sql.NullInt64{Int64: int64(version), Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if err := db.CreateAttempt(ctx, moxiedb.Attempt{AttemptID: id + "/1", JobID: id, AttemptNumber: 1, TemporalNamespace: "default", WorkflowID: e.WorkflowID, RunID: sql.NullString{String: id, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		o := moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: status, Valid: true}}
		if mode == "preparation" && status == "completed" {
			o.ResultDatasetID = sql.NullString{String: d.ID, Valid: true}
			o.ResultDatasetVersion = sql.NullInt64{Int64: int64(version), Valid: true}
		}
		if err := db.UpdateAttemptObservation(ctx, id+"/1", 0, o); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "training-jobs", id)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "adapter.bin"), []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
		return e
	}
	// The publication survived, but its preparation-result receipt did not.
	a := seed("prep-a", "preparation", "failed", 2)
	seed("prep-b", "preparation", "completed", 3)
	selectedTraining := seed("training-a", "training", "completed", 2)
	preservedTraining := seed("training-b", "training", "running", 3)
	seed("prep-current", "preparation", "running", 3)
	before, err := os.ReadFile(filepath.Join(files.Path(), d.ID, "3.json"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := j.PreviewDeletion(ctx, a)
	if err != nil || preview.DeletesDataset || len(preview.Blockers) != 0 || len(preview.Executions) != 2 || len(preview.ContinuingPreparation) != 1 {
		t.Fatalf("scope: %+v %v", preview, err)
	}
	if _, err := j.DeleteRun(ctx, a, "stale-revision"); !errors.Is(err, tmprl.ErrControlConflict) {
		t.Fatalf("stale confirmation: %v", err)
	}
	service.fail = true
	if _, err := j.DeleteRun(ctx, a, preview.Revision); err == nil {
		t.Fatal("outage reported as deletion success")
	}
	if pending, err := db.PendingDeletions(ctx); err != nil || len(pending) != 1 {
		t.Fatalf("missing recovery marker: %+v %v", pending, err)
	}
	service.fail = false
	// Fail after the files and SQL rows are removed, before the index receipt.
	index := filepath.Join(files.Path(), ".index.yaml")
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := j.RecoverDeletions(ctx); err == nil {
		t.Fatal("partial cleanup reported as success")
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := j.RecoverDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	for _, e := range []tmprl.Execution{a, selectedTraining} {
		if _, err := db.GetCatalogExecution(ctx, "default", e.WorkflowID, e.RunID); !errors.Is(err, moxiedb.ErrNotFound) {
			t.Fatalf("deleted run remains: %+v %v", e, err)
		}
		if _, err := os.Stat(filepath.Join(root, "training-jobs", e.RunID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted workspace remains: %+v %v", e, err)
		}
		if _, err := j.Get(ctx, e); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("deleted execution readable: %v", err)
		}
		if _, err := j.OpenDataset(ctx, e); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("deleted dataset downloadable: %v", err)
		}
		if _, err := j.OpenAdapter(ctx, e); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("deleted adapter downloadable: %v", err)
		}
		if _, err := j.Evaluation(ctx, e); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("deleted evaluation downloadable: %v", err)
		}
	}
	if _, err := db.GetCatalogExecution(ctx, "default", preservedTraining.WorkflowID, preservedTraining.RunID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "training-jobs", preservedTraining.RunID, "adapter.bin")); err != nil || string(data) != preservedTraining.RunID {
		t.Fatalf("other version's adapter changed: %s %v", data, err)
	}
	records, err := datasetsService.Records(ctx, d.ID, 3)
	if err != nil || len(records) != 2 || records[0].SourceDocument != "source-prep-a" || records[1].SourceDocument != "source-prep-b" {
		t.Fatalf("shared snapshot: %+v %v", records, err)
	}
	if after, err := os.ReadFile(filepath.Join(files.Path(), d.ID, "3.json")); err != nil || string(after) != string(before) {
		t.Fatalf("other version changed: %v", err)
	}
	if _, err := datasetsService.Get(ctx, d.ID, 2); !errors.Is(err, datasetstore.ErrDatasetNotFound) {
		t.Fatalf("deleted version available: %v", err)
	}
	if _, err := j.DeleteRun(ctx, a, preview.Revision); err != nil {
		t.Fatalf("confirmed retry: %v", err)
	}
	add("prep-current", "c", "When does harbor C open?", 3)
	latest, err := datasetsService.Get(ctx, d.ID, 0)
	if err != nil || latest.Version != 4 || latest.RecordCount != 3 {
		t.Fatalf("continued publication: %+v %v", latest, err)
	}
	// A training-only deletion must never delete its dataset version.
	trainingOnly := seed("training-only", "training", "completed", 4)
	p, err := j.PreviewDeletion(ctx, trainingOnly)
	if err != nil || p.DatasetID != "" || len(p.Snapshots) != 0 {
		t.Fatalf("training-only scope: %+v %v", p, err)
	}
	if _, err := j.DeleteRun(ctx, trainingOnly, p.Revision); err != nil {
		t.Fatal(err)
	}
	if records, err := datasetsService.Records(ctx, d.ID, 4); err != nil || len(records) != 3 {
		t.Fatalf("training-only deletion changed dataset: %+v %v", records, err)
	}
	// Active training on the selected version blocks deletion, never stops it.
	b := tmprl.Execution{WorkflowID: "workflow-prep-b", RunID: "prep-b"}
	p, err = j.PreviewDeletion(ctx, b)
	if err != nil || len(p.Blockers) == 0 {
		t.Fatalf("active dependency omitted: %+v %v", p, err)
	}
	if _, err := j.DeleteRun(ctx, b, p.Revision); !errors.Is(err, tmprl.ErrControlConflict) {
		t.Fatalf("active dependency was not blocked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "training-jobs", b.RunID)); err != nil {
		t.Fatal("blocked deletion removed artifacts", err)
	}
	if err := db.UpdateAttemptObservation(ctx, preservedTraining.RunID+"/1", 1, moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: "completed", Valid: true}}); err != nil {
		t.Fatal(err)
	}
	p, err = j.PreviewDeletion(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	// A newly attached training run invalidates an already displayed confirmation.
	seed("training-late", "training", "completed", 3)
	if _, err := j.DeleteRun(ctx, b, p.Revision); !errors.Is(err, tmprl.ErrControlConflict) {
		t.Fatalf("changed scope accepted: %v", err)
	}
	p, err = j.PreviewDeletion(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.DeleteRun(ctx, b, p.Revision); err != nil {
		t.Fatal(err)
	}
	if records, err := datasetsService.Records(ctx, d.ID, 4); err != nil || len(records) != 3 {
		t.Fatalf("older deletion changed newer records: %+v %v", records, err)
	}
	// Once the remaining preparation finishes, deleting its last saved version
	// removes the dataset and its empty creation scaffold from normal use.
	current := tmprl.Execution{WorkflowID: "workflow-prep-current", RunID: "prep-current"}
	if err := db.UpdateAttemptObservation(ctx, "prep-current/1", 1, moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: "completed", Valid: true}, ResultDatasetID: sql.NullString{String: d.ID, Valid: true}, ResultDatasetVersion: sql.NullInt64{Int64: 4, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	p, err = j.PreviewDeletion(ctx, current)
	if err != nil || !p.DeletesDataset || len(p.Snapshots) != 2 {
		t.Fatalf("last version scope: %+v %v", p, err)
	}
	if _, err := j.DeleteRun(ctx, current, p.Revision); err != nil {
		t.Fatal(err)
	}
	if list, err := datasetsService.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("last-version dataset still visible: %+v %v", list, err)
	}
	if _, err := files.Get(ctx, d.ID, 0); !errors.Is(err, datasetstore.ErrDatasetNotFound) {
		t.Fatalf("last-version files remain available: %v", err)
	}
}

type pausedTrainer struct {
	trainer
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *pausedTrainer) Train(ctx context.Context, input optimization.TrainingJob, publish func(tmprl.Progress)) (optimization.TrainingCandidate, error) {
	t.once.Do(func() { close(t.entered) })
	select {
	case <-ctx.Done():
		return optimization.TrainingCandidate{}, ctx.Err()
	case <-t.release:
	}
	if input.SafePoint == nil {
		return optimization.TrainingCandidate{}, errors.New("training did not receive its durable safe point")
	}
	path := filepath.Join(input.Workdir, "optimizer.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte("saved optimizer"), 0o600); err != nil {
			return optimization.TrainingCandidate{}, err
		}
	} else if err != nil {
		return optimization.TrainingCandidate{}, err
	}
	if err := input.SafePoint(ctx, "optimizer.json"); err != nil {
		return optimization.TrainingCandidate{}, err
	}
	return t.trainer.Train(ctx, input, publish)
}

func TestCatalogTrainingControlsRecoverLostReceipt(t *testing.T) {
	for _, action := range []string{"resume", "delete"} {
		t.Run(action, func(t *testing.T) { testCatalogTrainingControls(t, action) })
	}
}

func testCatalogTrainingControls(t *testing.T, action string) {
	db, _ := newCatalogDatabase(t)
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	files, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	name, data := source(t, false)
	upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, []byte("controlled model"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &lostReceiptClient{Client: env.Client}
	compute := &pausedTrainer{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: t.TempDir(),
		Models: map[string]string{"gemma": model}, Trainer: compute, Log: env.Log}
	workerJob, err := job.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	workers := workerJob.Workers()
	for _, w := range workers {
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
	}
	j, err := job.New(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	run, err := j.Start(ctx, job.Request{ID: "training-controls-" + env.TaskQueue, Model: "gemma", DatasetID: upload.ID})
	if err != nil || run.ControlStage != "training" || run.PauseInPlace {
		t.Fatalf("new training did not opt into controls: %+v %v", run, err)
	}
	select {
	case <-compute.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pause := tmprl.ControlRequest{RequestID: "pause", Action: "pause"}
	if _, err := j.PreparationControl(ctx, run.Execution, pause); !errors.Is(err, tmprl.ErrControlConflict) {
		t.Fatalf("preparation endpoint controlled training: %v", err)
	}
	c.review.Store(true)
	if _, err := j.TrainingControl(ctx, run.Execution, pause); !errors.Is(err, job.ErrUnavailable) {
		t.Fatalf("lost receipt was not exercised: %v", err)
	}
	close(compute.release)
	paused := awaitStatus(t, ctx, j, run.Execution, func(s job.State) bool { return s.Status == tmprl.StatusPaused && s.WorkerHost == "" })
	if paused.Step != "train" || paused.WorkerHost != "" || paused.ControlCheckpoint == "" || paused.ControlAcknowledgedRevision != 1 || compute.attempts.Load() != 0 {
		t.Fatalf("pause did not stop inside optimization: %+v", paused)
	}
	for _, w := range workers {
		w.Stop()
	}
	restarted, err := job.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range restarted.Workers() {
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
	}
	if err := restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if pending, err := db.PendingIntents(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("training receipt not reconciled: %+v %v", pending, err)
	}
	if receipt, err := restarted.TrainingControl(ctx, run.Execution, pause); err != nil || receipt.Revision != 1 {
		t.Fatalf("pause receipt could not be replayed: %+v %v", receipt, err)
	}
	if action == "delete" {
		preview, err := restarted.PreviewDeletion(ctx, run.Execution)
		if err != nil || len(preview.Blockers) != 0 {
			t.Fatalf("stopped run cannot be deleted: %+v %v", preview, err)
		}
		if _, err := restarted.DeleteRun(ctx, run.Execution, preview.Revision); err != nil {
			t.Fatal(err)
		}
		if _, err := restarted.TrainingControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "resume", Action: "resume", ExpectedRevision: 1}); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("deleted run accepted Resume: %v", err)
		}
		if _, err := os.Stat(filepath.Join(cfg.StorageRoot, "training-jobs", run.RunID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted checkpoint workspace remains: %v", err)
		}
		return
	}
	if _, err := restarted.TrainingControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "resume", Action: "resume", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	completed, err := j.Jobs().Wait(ctx, run.Execution)
	if err != nil || completed.Status != tmprl.StatusCompleted || !completed.Artifacts.Training.Accepted || compute.attempts.Load() != 1 {
		t.Fatalf("resume did not finish one training and evaluation: %+v %v", completed, err)
	}
}

type lostReceiptClient struct {
	client.Client
	submit atomic.Bool
	review atomic.Bool
	stop   atomic.Bool
	list   atomic.Bool
}

func (c *lostReceiptClient) ListWorkflow(ctx context.Context, request *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	if c.list.CompareAndSwap(true, false) {
		return nil, serviceerror.NewUnavailable("injected discovery outage")
	}
	return c.Client.ListWorkflow(ctx, request)
}

func (c *lostReceiptClient) CancelWorkflow(ctx context.Context, workflowID, runID string) error {
	err := c.Client.CancelWorkflow(ctx, workflowID, runID)
	if err == nil && c.stop.CompareAndSwap(true, false) {
		return serviceerror.NewUnavailable("injected lost cancellation receipt")
	}
	return err
}

func (c *lostReceiptClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow any, args ...any) (client.WorkflowRun, error) {
	run, err := c.Client.ExecuteWorkflow(ctx, options, workflow, args...)
	if err == nil && c.submit.CompareAndSwap(true, false) {
		return nil, serviceerror.NewUnavailable("injected lost submission receipt")
	}
	return run, err
}

func (c *lostReceiptClient) UpdateWorkflow(ctx context.Context, options client.UpdateWorkflowOptions) (client.WorkflowUpdateHandle, error) {
	handle, err := c.Client.UpdateWorkflow(ctx, options)
	if err == nil && c.review.CompareAndSwap(true, false) {
		return nil, serviceerror.NewUnavailable("injected lost review receipt")
	}
	return handle, err
}

type unavailableCatalogClient struct{ client.Client }

func (c unavailableCatalogClient) DescribeWorkflowExecution(context.Context, string, string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, serviceerror.NewUnavailable("injected outage")
}

type expiredCatalogClient struct{ client.Client }

func (c expiredCatalogClient) DescribeWorkflowExecution(context.Context, string, string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, serviceerror.NewNotFound("workflow history expired")
}

func TestCatalogHistoricalEvaluation(t *testing.T) {
	db, _ := newCatalogDatabase(t)
	ctx := t.Context()
	e := tmprl.Execution{WorkflowID: "historical-training", RunID: "historical-run"}
	if err := db.CreateJob(ctx, moxiedb.Job{JobID: "historical-job", Mode: "training"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAttempt(ctx, moxiedb.Attempt{
		AttemptID: "historical-attempt", JobID: "historical-job", AttemptNumber: 1,
		TemporalNamespace: "default", WorkflowID: e.WorkflowID,
		RunID: sql.NullString{String: e.RunID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	observation, err := json.Marshal(job.CatalogObservation{State: job.State{EvaluationAvailable: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateAttemptObservation(ctx, "historical-attempt", 0, moxiedb.AttemptObservation{
		ExecutionStatus: sql.NullString{String: "needs_improvement", Valid: true}, Observation: observation,
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	files, err := uploads.New(uploads.Config{Root: filepath.Join(root, "uploads")})
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "training-jobs", e.RunID, "steps", "training")
	if err := os.MkdirAll(reportPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reportPath, "evaluation.json"), []byte(`{"base_correct":1,"adapter_correct":2,"total":5,"accepted":false,"answers":[{"prompt":"question","expected":"answer","base_answer":"wrong","adapter_answer":"answer"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := job.Config{Uploads: files, StorageRoot: root, MoxieDB: db, Trainer: &trainer{}}
	j, err := job.New(expiredCatalogClient{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	report, err := j.Evaluation(ctx, e)
	if err != nil || report.BaseCorrect != 1 || report.AdapterCorrect != 2 || report.Total != 5 || report.Accepted || len(report.Answers) != 1 || report.Answers[0].Expected != "answer" {
		t.Fatalf("historical evaluation: %+v, %v", report, err)
	}
	// An existing report file must not bypass the catalog's exact identity.
	for _, other := range []tmprl.Execution{
		{WorkflowID: "another-workflow", RunID: e.RunID},
		{WorkflowID: e.WorkflowID, RunID: "another-run"},
	} {
		if _, err := j.Evaluation(ctx, other); !errors.Is(err, job.ErrNotFound) {
			t.Fatalf("unregistered execution %+v: %v", other, err)
		}
	}
	cfg.Namespace = "another-namespace"
	other, err := job.New(expiredCatalogClient{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Evaluation(ctx, e); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("unregistered namespace: %v", err)
	}
	// A file alone does not make an unrecorded evaluation available.
	if err := db.UpdateAttemptObservation(ctx, "historical-attempt", 1, moxiedb.AttemptObservation{
		Observation: json.RawMessage(`{"state":{"EvaluationAvailable":false}}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Evaluation(ctx, e); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("evaluation not recorded as available: %v", err)
	}
}

type delayedCatalogClient struct {
	client.Client
	once             atomic.Bool
	entered, release chan struct{}
}

func (c *delayedCatalogClient) DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	response, err := c.Client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err == nil && c.once.CompareAndSwap(false, true) {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return response, err
}

func TestCatalogPreparationControls(t *testing.T) {
	for _, failure := range []string{"temporal_receipt", "sql_receipt", "stop_before_resolution"} {
		t.Run(failure, func(t *testing.T) {
			db, setup := newCatalogDatabase(t)
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			files, err := uploads.New(uploads.Config{Root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			name, text := source(t, true)
			upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(text))
			if err != nil {
				t.Fatal(err)
			}
			c := &lostReceiptClient{Client: env.Client}
			cfg := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: t.TempDir(), Trainer: &trainer{}, Completer: completer{}, Log: env.Log}
			j, err := job.New(c, cfg)
			if err != nil {
				t.Fatal(err)
			}
			workerJob, err := job.New(env.Client, cfg)
			if err != nil {
				t.Fatal(err)
			}
			w := workerJob.Jobs().NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			run, err := j.Start(ctx, job.Request{ID: "controls-" + env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer, MaxQuestions: 5}})
			if err != nil {
				t.Fatal(err)
			}
			waiting := waitForCheckpoint(t, ctx, j, run.Execution, "preview-review")
			if failure == "sql_receipt" {
				_, err := setup.ExecContext(ctx, `CREATE SEQUENCE control_fault;
CREATE FUNCTION control_fault() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF nextval('control_fault')=1 THEN RAISE EXCEPTION 'injected settlement failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER control_fault BEFORE UPDATE ON job_intents FOR EACH ROW
WHEN (NEW.kind IN ('pause','resume') AND NEW.state='accepted') EXECUTE FUNCTION control_fault();`)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				c.review.Store(true)
			}
			pause := tmprl.ControlRequest{RequestID: "pause-1", Action: "pause"}
			if _, err := j.PreparationControl(ctx, run.Execution, pause); err == nil {
				t.Fatal("receipt-loss injection was not exercised")
			}
			pending, err := db.PendingIntents(ctx)
			if err != nil || len(pending) != 1 || pending[0].Kind != "pause" {
				t.Fatalf("lost receipt must retain intent: %+v %v", pending, err)
			}
			if failure == "stop_before_resolution" {
				if err := j.Stop(ctx, run.Execution); err != nil {
					t.Fatal(err)
				}
				if _, err := j.Jobs().Wait(ctx, run.Execution); err == nil {
					t.Fatal("Stop did not cancel the execution")
				}
			}
			restarted, err := job.New(env.Client, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			service := datasets.New(datasets.Config{Files: restarted.Datasets(), Job: restarted, DB: db})
			receipt, err := service.PreparationControl(ctx, run.Execution, pause)
			if err != nil || receipt.Revision != 1 || receipt.ControlRequest != pause {
				t.Fatalf("recovered receipt: %+v %v", receipt, err)
			}
			if failure == "stop_before_resolution" {
				if _, err := service.PreparationControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "continue-stopped", Action: "resume", ExpectedRevision: 1}); !errors.Is(err, tmprl.ErrControlConflict) {
					t.Fatalf("continued a canceled execution: %v", err)
				}
				if pending, err := db.PendingIntents(ctx); err != nil || len(pending) != 0 {
					t.Fatalf("Stop left unresolved receipt: %+v %v", pending, err)
				}
				return
			}
			for _, command := range []tmprl.ControlRequest{
				{RequestID: "resume-1", Action: "resume", ExpectedRevision: 1},
				{RequestID: "pause-2", Action: "pause", ExpectedRevision: 2},
				{RequestID: "resume-2", Action: "resume", ExpectedRevision: 3},
			} {
				if command.RequestID == "resume-1" {
					c.review.Store(true)
					if _, err := j.PreparationControl(ctx, run.Execution, command); !errors.Is(err, job.ErrUnavailable) {
						t.Fatalf("lost Continue receipt not exercised: %v", err)
					}
					if err := restarted.Reconcile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if r, err := service.PreparationControl(ctx, run.Execution, command); err != nil || r.Revision != command.ExpectedRevision+1 {
					t.Fatalf("cycle: %+v %v", r, err)
				}
			}
			for _, command := range []tmprl.ControlRequest{
				{RequestID: "pause-1", Action: "resume", ExpectedRevision: 1},
				{RequestID: "stale", Action: "pause", ExpectedRevision: 1},
			} {
				if _, err := service.PreparationControl(ctx, run.Execution, command); !errors.Is(err, tmprl.ErrControlConflict) {
					t.Fatalf("conflict accepted: %+v %v", command, err)
				}
			}
			if r, err := service.PreparationControl(ctx, run.Execution, pause); err != nil || r != receipt {
				t.Fatalf("old replay changed receipt: %+v %v", r, err)
			}
			state, err := service.GetRun(ctx, run.Execution)
			if err != nil || state.ControlRevision != 4 || state.ControlAction != "resume" || state.Checkpoint == nil || state.Checkpoint.ID != waiting.Checkpoint.ID || state.Status != tmprl.StatusWaiting {
				t.Fatalf("continue bypassed review or replay changed state: %+v %v", state, err)
			}
			if _, err := service.PreparationControl(ctx, tmprl.Execution{WorkflowID: run.WorkflowID, RunID: "wrong-run"}, pause); !errors.Is(err, job.ErrNotFound) {
				t.Fatalf("wrong execution accepted: %v", err)
			}
			if err := service.StopRun(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.Jobs().Wait(ctx, run.Execution); err == nil {
				t.Fatal("Stop did not cancel the execution")
			}
			if r, err := service.PreparationControl(ctx, run.Execution, pause); err != nil || r != receipt {
				t.Fatalf("Stop erased prior receipt: %+v %v", r, err)
			}
			if _, err := service.PreparationControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "after-stop", Action: "pause", ExpectedRevision: 4}); !errors.Is(err, tmprl.ErrControlConflict) {
				t.Fatalf("stopped execution accepted a new command: %v", err)
			}
			if pending, err := db.PendingIntents(ctx); err != nil || len(pending) != 0 {
				t.Fatalf("unresolved intents: %+v %v", pending, err)
			}
		})
	}
}

func TestCatalogPreparationSafePoint(t *testing.T) {
	db, _ := newCatalogDatabase(t)
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	files, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	name, text := source(t, true)
	upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	compute := &trainer{}
	config := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: t.TempDir(), Trainer: compute, Log: env.Log,
		Completer: preparationCompleter(func(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
			if strings.Contains(request.Messages[1].Content, "[EVIDENCE 1]") && calls.Add(1) == 2 {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return providers.Completion{}, ctx.Err()
				}
			}
			return (completer{}).Complete(ctx, request)
		}),
	}
	j, err := job.New(env.Client, config)
	if err != nil {
		t.Fatal(err)
	}
	w := j.Jobs().NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	service := datasets.New(datasets.Config{Files: j.Datasets(), Job: j, DB: db})
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(ctx, job.Request{ID: "safe-" + env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer, MaxQuestions: 12}})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForCheckpoint(t, ctx, j, run.Execution, "preview-review")
	payload, err := json.Marshal(preparation.Review{Records: waiting.Artifacts.Preview.Preview})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ContinuePreview(ctx, run.Execution, tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "accepted-review", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := service.PreparationControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "pause-in-generation", Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	for {
		state, err := service.GetRun(ctx, run.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == tmprl.StatusPaused {
			if state.Step != "generate" || state.ControlAcknowledgedRevision != 1 || !strings.HasPrefix(state.ControlCheckpoint, "preparation/generate/steps.json:") {
				t.Fatalf("catalog claimed pause without a saved generation checkpoint: %+v", state)
			}
			break
		}
		if state.Status.Terminal() {
			t.Fatalf("preparation closed instead of pausing: %+v", state)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	// Host shutdown while paused: save, stop the worker, and stay offline.
	if err := j.Jobs().Drain(ctx); err != nil {
		t.Fatal(err)
	}
	w.Stop()
	// A catalog-only reader must retain the confirmed checkpoint while the
	// worker/command facade is unavailable. It does not reconstruct the records.
	reader := datasets.New(datasets.Config{Files: j.Datasets(), DB: db, Namespace: env.Namespace})
	state, err := reader.GetRun(ctx, run.Execution)
	if err != nil || state.Status != tmprl.StatusPaused || state.ControlAcknowledgedRevision != 1 || state.ControlCheckpoint == "" {
		t.Fatalf("SQL lost safe-point observation: %+v, %v", state, err)
	}
	// Restarted process: the run stays paused until Continue.
	if j, err = job.New(env.Client, config); err != nil {
		t.Fatal(err)
	}
	w = j.Jobs().NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	service = datasets.New(datasets.Config{Files: j.Datasets(), Job: j, DB: db})
	if _, err := service.PreparationControl(ctx, run.Execution, tmprl.ControlRequest{RequestID: "continue-generation", Action: "resume", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	completed, err := j.Jobs().Wait(ctx, run.Execution)
	if err != nil || completed.Execution != run.Execution || completed.Artifacts.Preparation == nil || completed.Artifacts.Preparation.RecordCount != 12 || compute.attempts.Load() != 0 || len(completed.Decisions) != 1 || completed.Decisions[0].SubmissionID != "accepted-review" {
		t.Fatalf("Continue lost review/output or started training: %+v, %v", completed, err)
	}
}

func TestCatalogSubmissionRecovery(t *testing.T) {
	for _, failure := range []string{"temporal_receipt", "sql_receipt"} {
		t.Run(failure, func(t *testing.T) {
			db, setup := newCatalogDatabase(t)
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			files, err := uploads.New(uploads.Config{Root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			name, text := source(t, true)
			upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(text))
			if err != nil {
				t.Fatal(err)
			}
			c := &lostReceiptClient{Client: env.Client}
			c.submit.Store(failure == "temporal_receipt")
			if failure == "sql_receipt" {
				_, err := setup.ExecContext(ctx, `CREATE SEQUENCE link_fault;
CREATE FUNCTION link_fault() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF nextval('link_fault')=1 THEN RAISE EXCEPTION 'injected lost SQL receipt'; END IF; RETURN NEW; END $$;
CREATE TRIGGER link_fault BEFORE UPDATE ON job_attempts FOR EACH ROW
WHEN (OLD.run_id IS NULL AND NEW.run_id IS NOT NULL) EXECUTE FUNCTION link_fault();`)
				if err != nil {
					t.Fatal(err)
				}
			}
			cfg := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: t.TempDir(), Trainer: &trainer{}, Completer: completer{}, Log: env.Log}
			j, err := job.New(c, cfg)
			if err != nil {
				t.Fatal(err)
			}
			input := job.Request{ID: "catalog-" + env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer}}
			if _, err := j.Start(ctx, input); err == nil {
				t.Fatal("injected receipt loss was not exercised")
			}
			pending, err := db.PendingIntents(ctx)
			if err != nil || len(pending) != 1 || pending[0].Kind != "submit" {
				t.Fatalf("missing durable submit intent: %+v %v", pending, err)
			}
			a, err := db.GetAttempt(ctx, pending[0].AttemptID)
			if err != nil || a.RunID.Valid {
				t.Fatalf("receipt loss did not leave an unresolved execution link: %+v %v", a, err)
			}
			// Construct a fresh application object: recovery must use durable state,
			// not the original handler's return value or its process-local memory.
			restarted, err := job.New(env.Client, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			a, err = db.GetAttempt(ctx, a.AttemptID)
			if err != nil || !a.RunID.Valid {
				t.Fatalf("unresolved original execution: %+v %v", a, err)
			}
			replay, err := restarted.Start(ctx, input)
			if err != nil || replay.RunID != a.RunID.String {
				t.Fatalf("replay started another execution: %+v %v", replay, err)
			}
			input.Prepare.Model = "changed-model"
			if _, err := restarted.Start(ctx, input); !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("changed input replay accepted: %v", err)
			}
			pending, err = db.PendingIntents(ctx)
			if err != nil || len(pending) != 0 {
				t.Fatalf("unsettled submission: %+v %v", pending, err)
			}
			rows, err := db.ListCatalog(ctx, moxiedb.CatalogFilter{Namespace: env.Namespace})
			if err != nil || len(rows) != 1 || rows[0].Attempt.RunID.String != a.RunID.String {
				t.Fatalf("duplicate or missing logical job: %+v %v", rows, err)
			}
			c.stop.Store(true)
			if err := j.Stop(ctx, replay.Execution); !errors.Is(err, job.ErrUnavailable) {
				t.Fatalf("lost Stop receipt not exercised: %v", err)
			}
			pending, err = db.PendingIntents(ctx)
			if err != nil || len(pending) != 1 || pending[0].Kind != "stop" {
				t.Fatalf("missing uncertain Stop: %+v %v", pending, err)
			}
			if err := restarted.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			pending, err = db.PendingIntents(ctx)
			if err != nil || len(pending) != 0 {
				t.Fatalf("Stop not reconciled: %+v %v", pending, err)
			}
			if err := restarted.Stop(ctx, replay.Execution); err != nil {
				t.Fatalf("Stop replay failed: %v", err)
			}
		})
	}
}

func TestCatalogReviewAndObservationRecovery(t *testing.T) {
	db, _ := newCatalogDatabase(t)
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	files, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	name, text := source(t, true)
	upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	c := &lostReceiptClient{Client: env.Client}
	c.review.Store(true)
	compute := &trainer{}
	cfg := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: t.TempDir(), Trainer: compute, Completer: completer{}, Log: env.Log}
	j, err := job.New(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(datasets.Config{Files: j.Datasets(), Job: j, DB: db})
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	workerJob, err := job.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	worker := workerJob.Jobs().NewWorker()
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Stop)
	run, err := j.Start(ctx, job.Request{ID: "review-" + env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer, MaxQuestions: 5}})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForCheckpoint(t, ctx, j, run.Execution, "preview-review")
	// The foundation polling helper deliberately bypasses the application's
	// catalog. Observe through the public application API before the outage.
	if _, err := j.Get(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	entry, err := db.GetCatalogExecution(ctx, env.Namespace, run.WorkflowID, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entry.Attempt.Observation), waiting.Artifacts.Preview.Preview[0].Target) || strings.Contains(string(entry.Attempt.Observation), upload.ID) {
		t.Fatal("SQL observation contains source/preview text")
	}
	outage, err := job.New(unavailableCatalogClient{env.Client}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := outage.Reconcile(ctx); !errors.Is(err, job.ErrUnavailable) {
		t.Fatalf("outage was hidden: %v", err)
	}
	stale, err := db.GetCatalogExecution(ctx, env.Namespace, run.WorkflowID, run.RunID)
	if err != nil || !stale.Attempt.StateStale || stale.Attempt.ExecutionStatus.String != "waiting" || !stale.Attempt.ObservedAt.Time.Equal(entry.Attempt.ObservedAt.Time) {
		t.Fatalf("outage fabricated state/freshness: %+v %v", stale, err)
	}
	delayed := &delayedCatalogClient{Client: env.Client, entered: make(chan struct{}), release: make(chan struct{})}
	oldReader, err := job.New(delayed, cfg)
	if err != nil {
		t.Fatal(err)
	}
	oldResult := make(chan error, 1)
	go func() { _, err := oldReader.Get(ctx, run.Execution); oldResult <- err }()
	select {
	case <-delayed.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	payload, err := json.Marshal(preparation.Review{Records: waiting.Artifacts.Preview.Preview})
	if err != nil {
		t.Fatal(err)
	}
	decision := tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "review-one", Payload: payload}
	if _, err := j.ContinuePreview(ctx, run.Execution, decision); !errors.Is(err, job.ErrUnavailable) {
		t.Fatalf("lost review receipt not exercised: %v", err)
	}
	finished, err := j.Jobs().Wait(ctx, run.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != tmprl.StatusCompleted || compute.attempts.Load() != 0 {
		t.Fatal("preparation unexpectedly authorized training")
	}
	restarted, err := job.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Get(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	close(delayed.release)
	if err := <-oldResult; err != nil {
		t.Fatal(err)
	}
	entry, err = db.GetCatalogExecution(ctx, env.Namespace, run.WorkflowID, run.RunID)
	if err != nil || entry.Attempt.ExecutionStatus.String != "completed" || entry.Attempt.StateStale {
		t.Fatalf("older observation regressed completion: %+v %v", entry, err)
	}
	pending, err := db.PendingIntents(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("accepted review receipt not reconciled: %+v %v", pending, err)
	}
	if _, err := restarted.ContinuePreview(ctx, run.Execution, decision); err != nil {
		t.Fatalf("accepted review replay failed: %v", err)
	}
	decision.SubmissionID = "different-review"
	if _, err := restarted.ContinuePreview(ctx, run.Execution, decision); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("conflicting review accepted: %v", err)
	}
}

func TestCatalogDiscoveryRecovery(t *testing.T) {
	db, _ := newCatalogDatabase(t)
	env := newTestEnvironment(t)
	// History discovery is namespace-wide, unlike a task-queue-scoped worker.
	// Keep this test independent of other runs and their saved-output catalogs.
	namespace := "discovery-" + env.TaskQueue
	if _, err := env.Client.WorkflowService().RegisterNamespace(t.Context(), &workflowservice.RegisterNamespaceRequest{Namespace: namespace, WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEMPORAL_NAMESPACE", namespace)
	env = newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	files, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	name, text := source(t, true)
	upload, err := files.Put(ctx, uploads.Input{Name: name}, strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	cfg := job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, StorageRoot: t.TempDir(), Trainer: &trainer{}, Completer: completer{}, Log: env.Log}
	legacy, err := job.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	run, err := legacy.Start(ctx, job.Request{ID: "legacy-" + env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{DatasetID: upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer}})
	if err != nil {
		t.Fatal(err)
	}
	// Wait for Temporal's eventually consistent visibility before injecting the
	// startup outage, so the regression is discovery retry, not indexing timing.
	for {
		runs, _, err := legacy.List(ctx, tmprl.ListOptions{PageSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, existing := range runs {
			found = found || existing.RunID == run.RunID
		}
		if found {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	c := &lostReceiptClient{Client: env.Client}
	c.list.Store(true)
	cfg.MoxieDB = db
	restarted, err := job.New(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); restarted.RunCatalog(ctx) }()
	defer func() { cancel(); <-done }()
	for {
		entry, err := db.GetCatalogExecution(ctx, env.Namespace, run.WorkflowID, run.RunID)
		if err == nil {
			if c.list.Load() || entry.Attempt.ExecutionStatus.String != "queued" {
				t.Fatalf("discovery did not exercise/recover the outage: %+v", entry)
			}
			break
		}
		if !errors.Is(err, moxiedb.ErrNotFound) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("legacy history not rediscovered after outage", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}
